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

// blueprintPrompt is what the templates render against: the request, plus the
// two figures the model needs and Go has to compute because text/template
// cannot do arithmetic.
type blueprintPrompt struct {
	provider.BlueprintRequest
	WordsPerMinute  int
	TotalWords      int
	DurationMinutes int
	// ExpectedOutputSchema is generated from blueprintDoc, so the shape the
	// prompt asks for is the shape the parse expects, always.
	ExpectedOutputSchema string
}

// newBlueprintPrompt resolves the video's spoken-word budget. A target duration
// is the honest input, so it wins when set; without one the length is whatever
// the requested chapters come to at the default size.
func newBlueprintPrompt(req provider.BlueprintRequest) (blueprintPrompt, error) {
	rate := entity.DefaultWordsPerMinute
	total := req.TargetDurationMinutes * rate
	if total <= 0 {
		total = req.ChapterCount * entity.DefaultWordsPerChapter
	}
	schema, err := jsonSchemaOf(blueprintDoc{})
	if err != nil {
		return blueprintPrompt{}, err
	}
	return blueprintPrompt{
		BlueprintRequest:     req,
		WordsPerMinute:       rate,
		TotalWords:           total,
		DurationMinutes:      total / rate,
		ExpectedOutputSchema: schema,
	}, nil
}

// blueprintDoc is the outline in full, as the model returns it and the store
// keeps it. It is richer than provider.Blueprint on purpose: the whole document
// lands in the asset store and only the collapse below crosses the port, so
// widening the port later is a change to the collapse rather than the prompt.
// The doc tags are the prompt's field descriptions.
type blueprintDoc struct {
	Title    string       `json:"title" doc:"the video's working title, kept as given"`
	Summary  string       `json:"summary" doc:"two or three sentences describing the whole video"`
	Chapters []chapterDoc `json:"chapters" doc:"every chapter, in order, numbered from 1"`
}

//nolint:lll // one field, one line: each tag is this field's line in the prompt
type chapterDoc struct {
	Order          int      `json:"order" doc:"1-based position in the outline"`
	Title          string   `json:"title" doc:"short phrase, letters and spaces only, 3-10 words"`
	CoreConcept    string   `json:"core_concept" doc:"one clear paragraph explaining precisely what the chapter teaches"`
	KeyPoints      []string `json:"key_points" doc:"beats in order; the first is the opening directive, written as 'Open: <style> — <description>'"`
	DoNotRepeats   []string `json:"do_not_repeats" doc:"concepts an earlier chapter already taught, each as '<concept> — introduced in <title> (ch N) — <instruction>'; empty when the chapter opens fresh ground"`
	ForwardHook    string   `json:"forward_hook" doc:"a speakable sentence handing an unresolved question to the next chapter, or an empty string"`
	Tone           string   `json:"tone" doc:"light | curious | unsettling | dark"`
	Pacing         string   `json:"pacing" doc:"short | medium | deep"`
	Role           string   `json:"role" doc:"hook | exploration | contrast | deep_dive"`
	EstimatedWords int      `json:"estimated_words" doc:"this chapter's share of the video's spoken-word budget"`
}

// storedBlueprint is the outline plus the identity the caller stamps on it.
// Kept apart so the schema handed to the model describes only its own output.
type storedBlueprint struct {
	Video entity.VideoID `json:"video"`
	Ref   entity.Ref     `json:"ref"`
	blueprintDoc
}

// Blueprint plans a video's chapters. There is almost no validation here on
// purpose: the use case's tolerance band owns the chapter count and
// entity.NewChapter owns what makes a chapter well formed.
func (c *Client) Blueprint(ctx context.Context, req provider.BlueprintRequest) (provider.Blueprint, error) {
	prompt, err := newBlueprintPrompt(req)
	if err != nil {
		return provider.Blueprint{}, err
	}
	system, err := render(blueprintSystemPrompt, prompt)
	if err != nil {
		return provider.Blueprint{}, err
	}
	user, err := render(blueprintUserPrompt, prompt)
	if err != nil {
		return provider.Blueprint{}, err
	}

	content, err := c.chat(ctx, call{Video: req.VideoID, Label: "blueprint"}, system, user)
	if err != nil {
		return provider.Blueprint{}, err
	}

	// No fence stripping and no seeking for the outermost brace: the contract is
	// the last thing the prompt says, and ignoring it is a bad roll.
	var doc blueprintDoc
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return provider.Blueprint{}, fmt.Errorf("blueprint response is not JSON: %w (%s)",
			err, snippet(content))
	}
	doc.normalise(req)

	assetID, err := c.putJSON(ctx, entity.AssetKindBlueprint, storedBlueprint{
		Video: req.VideoID, Ref: req.VideoRef, blueprintDoc: doc,
	})
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

// normalise renumbers the chapters from their position and falls back to the
// requested title. Order is reassigned rather than trusted because the DAG and
// the chapter table are addressed by it; everything else the model said stands.
func (d *blueprintDoc) normalise(req provider.BlueprintRequest) {
	if strings.TrimSpace(d.Title) == "" {
		d.Title = req.Title
	}
	for i := range d.Chapters {
		d.Chapters[i].Order = i + 1
	}
}

// brief renders one chapter's plan as the text the script writer receives. The
// port carries only a Summary, so the whole brief is folded into that field —
// a block of direction rather than a sentence, until the port grows fields.
func (c chapterDoc) brief() string {
	var b strings.Builder
	if concept := strings.TrimSpace(c.CoreConcept); concept != "" {
		b.WriteString(concept)
	}

	if points := trimmed(c.KeyPoints); len(points) > 0 {
		section(&b)
		// The first beat is the opening directive and reads as a heading.
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

// meta is the one-line direction footer: feel, depth, length.
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

// trimmed drops blank entries, so a padded list is not a brief full of stray
// bullets.
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

// putJSON stores a document and returns its content address. Indented so the
// asset viewer shows something legible, and stable so identical outlines share
// one address.
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
