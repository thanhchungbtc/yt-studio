package ninerouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// narrationRate is the channel's reading speed, which is the number the whole
// budget is derived from at both ends: words planned here, duration reported
// after the script is written.
func narrationRate(style entity.StyleConfig) int {
	if style.WordsPerMinute > 0 {
		return style.WordsPerMinute
	}
	return entity.DefaultWordsPerMinute
}

// wordsPerChapter is the channel's per-chapter target, falling back to the
// domain default so a budget handed to a model is never zero.
func wordsPerChapter(style entity.StyleConfig) int {
	if style.WordsPerChapter > 0 {
		return style.WordsPerChapter
	}
	return entity.DefaultWordsPerChapter
}

// blueprintPrompt is what the templates render against: the request, plus the
// two figures the model needs and Go has to compute because text/template
// cannot do arithmetic.
type blueprintPrompt struct {
	provider.BlueprintRequest
	WordsPerMinute  int
	TotalWords      int
	DurationMinutes int
}

// newBlueprintPrompt resolves the video's spoken-word budget.
//
// A target duration is the honest input — the channel wants three hours and
// does not much mind how many chapters that takes — so when one is set the
// budget comes from it. Without one the length is whatever the requested
// chapters come to at the channel's usual size, which is how videos planned
// before durations existed keep the shape they were planned with.
func newBlueprintPrompt(req provider.BlueprintRequest) blueprintPrompt {
	rate := narrationRate(req.Style)
	total := req.TargetDurationMinutes * rate
	if total <= 0 {
		total = req.ChapterCount * wordsPerChapter(req.Style)
	}
	return blueprintPrompt{
		BlueprintRequest: req,
		WordsPerMinute:   rate,
		TotalWords:       total,
		DurationMinutes:  total / rate,
	}
}

// blueprintDoc is the outline as the model returns it and as the store keeps
// it, in full.
//
// It is much richer than provider.Blueprint, deliberately. The brief a script
// writer needs is a structured thing, and the port carries a title and a
// sentence — so the whole document is what lands in the asset store, and only
// the collapse below crosses the boundary. Nothing is lost, and widening the
// port later is a change to the collapse rather than to the prompt.
type blueprintDoc struct {
	Video    entity.VideoID `json:"video"`
	Ref      entity.Ref     `json:"ref"`
	Title    string         `json:"title"`
	Summary  string         `json:"summary"`
	Chapters []chapterDoc   `json:"chapters"`
}

type chapterDoc struct {
	Order int    `json:"order"`
	Title string `json:"title"`
	// Mechanism is the idea the chapter teaches, stripped of its title. It is
	// the model's own dedup working shown on the page: two chapters sharing a
	// mechanism are the duplicate the outline was supposed to remove.
	Mechanism      string   `json:"mechanism"`
	CoreConcept    string   `json:"core_concept"`
	KeyPoints      []string `json:"key_points"`
	DoNotRepeats   []string `json:"do_not_repeats"`
	ForwardHook    string   `json:"forward_hook"`
	Tone           string   `json:"tone"`
	Pacing         string   `json:"pacing"`
	Role           string   `json:"role"`
	EstimatedWords int      `json:"estimated_words"`
}

// Blueprint plans a video's chapters.
//
// There is almost no validation here on purpose. What the model returns is
// trusted; the layers that already have an opinion enforce it — the tolerance
// band in the use case owns the chapter count, and entity.NewChapter owns what
// makes a chapter well formed. Checking any of it a second time here would only
// mean two places to change.
func (c *Client) Blueprint(ctx context.Context, req provider.BlueprintRequest) (provider.Blueprint, error) {
	prompt := newBlueprintPrompt(req)
	system, err := render(blueprintSystemPrompt, prompt)
	if err != nil {
		return provider.Blueprint{}, err
	}
	user, err := render(blueprintUserPrompt, prompt)
	if err != nil {
		return provider.Blueprint{}, err
	}

	content, err := c.chat(ctx, system, user)
	if err != nil {
		return provider.Blueprint{}, err
	}

	// No fence stripping and no seeking for the outermost brace. The output
	// contract is the last thing the system prompt says, and a response that
	// ignores it is a bad roll rather than a shape to unwrap.
	var doc blueprintDoc
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return provider.Blueprint{}, fmt.Errorf("blueprint response is not JSON: %w (%s)",
			err, snippet(content))
	}
	doc.normalise(req)

	assetID, err := c.putJSON(ctx, entity.AssetKindBlueprint, doc)
	if err != nil {
		return provider.Blueprint{}, err
	}

	chapters := make([]provider.BlueprintChapter, 0, len(doc.Chapters))
	for _, ch := range doc.Chapters {
		chapters = append(chapters, provider.BlueprintChapter{
			Ordinal:        ch.Order,
			Title:          ch.Title,
			Summary:        ch.brief(),
			EstimatedWords: ch.EstimatedWords,
		})
	}
	return provider.Blueprint{
		BlueprintOutline: provider.BlueprintOutline{
			Title:    doc.Title,
			Summary:  doc.Summary,
			Chapters: chapters,
		},
		AssetID: assetID,
	}, nil
}

// normalise stamps the facts that are the caller's rather than the model's, and
// renumbers the chapters from their position.
//
// Order is reassigned rather than trusted because it is the key the DAG and the
// chapter table are addressed by; everything else the model said stands.
func (d *blueprintDoc) normalise(req provider.BlueprintRequest) {
	d.Video, d.Ref = req.VideoID, req.VideoRef
	if strings.TrimSpace(d.Title) == "" {
		d.Title = req.Title
	}
	for i := range d.Chapters {
		d.Chapters[i].Order = i + 1
	}
}

// brief renders one chapter's plan as the text the script writer receives.
//
// provider.BlueprintChapter carries a Summary, and a summary is not what a
// writer needs — so the whole brief is folded into that field rather than
// dropped. It reads as a block of direction rather than a sentence, which is
// deliberate until the port grows fields of its own.
func (c chapterDoc) brief() string {
	var b strings.Builder
	if concept := strings.TrimSpace(c.CoreConcept); concept != "" {
		b.WriteString(concept)
	}

	if points := trimmed(c.KeyPoints); len(points) > 0 {
		section(&b)
		// The first beat is the opening directive and reads as a heading; the rest
		// are beats and read as a tight list beneath it.
		b.WriteString(points[0])
		for _, point := range points[1:] {
			b.WriteString("\n- " + point)
		}
	}

	if repeats := trimmed(c.DoNotRepeats); len(repeats) > 0 {
		section(&b)
		b.WriteString("Do not re-explain:")
		for _, entry := range repeats {
			b.WriteString("\n- " + entry)
		}
	}
	if hook := strings.TrimSpace(c.ForwardHook); hook != "" {
		section(&b)
		b.WriteString("Hand-off: " + hook)
	}
	if meta := c.meta(); meta != "" {
		section(&b)
		b.WriteString(meta)
	}
	return b.String()
}

// meta is the one-line direction footer: how this chapter should feel, how deep
// it should go, and how long it should run.
func (c chapterDoc) meta() string {
	parts := make([]string, 0, 4)
	for _, field := range []string{c.Role, c.Tone, c.Pacing} {
		if field = strings.TrimSpace(field); field != "" {
			parts = append(parts, field)
		}
	}
	if c.EstimatedWords > 0 {
		parts = append(parts, fmt.Sprintf("~%d words", c.EstimatedWords))
	}
	return strings.Join(parts, " · ")
}

// trimmed drops blank entries and trims what is left, so a model that padded a
// list with empty strings does not produce a brief full of stray bullets.
func trimmed(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// section starts a new block, blank-line separated, unless nothing has been
// written yet.
func section(b *strings.Builder) {
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
}

// putText stores a plain-text artifact and returns its content address.
func (c *Client) putText(ctx context.Context, kind entity.AssetKind, text string) (entity.AssetID, error) {
	stored, err := c.store.Put(ctx, kind, strings.NewReader(text))
	if err != nil {
		return "", fmt.Errorf("store %s: %w", kind, err)
	}
	return stored.ID, nil
}

// putJSON stores a document and returns its content address.
//
// The encoding is indented so the asset viewer shows something legible, and
// stable so two identical outlines share one address.
func (c *Client) putJSON(ctx context.Context, kind entity.AssetKind, v any) (entity.AssetID, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return "", fmt.Errorf("encode %s: %w", kind, err)
	}
	stored, err := c.store.Put(ctx, kind, &buf)
	if err != nil {
		return "", fmt.Errorf("store %s: %w", kind, err)
	}
	return stored.ID, nil
}
