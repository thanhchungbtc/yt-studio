package ninerouter

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// VideoContext is what a slide-prompt generation needs to know about a video.
// The port hands this method an id and nothing else, and a provider may not
// read the database, so the caller supplies a lookup between the two.
type VideoContext struct {
	provider.BlueprintOutline
	SlidesPerChapter int
}

// ContextLookup resolves a video id into the plan its slides illustrate.
type ContextLookup func(ctx context.Context, videoID entity.VideoID) (VideoContext, error)

// slidePromptsDoc is the shape the model returns, and the type the prompt's
// schema is generated from.
type slidePromptsDoc struct {
	//nolint:lll // one field, one line: each tag is this field's line in the prompt
	Prompts []slidePromptDoc `json:"prompts" doc:"one entry per requested image, in the order the assets were listed"`
}

//nolint:lll // one field, one line: each tag is this field's line in the prompt
type slidePromptDoc struct {
	Chapter int    `json:"chapter" doc:"the 1-based chapter ordinal this image belongs to"`
	Index   int    `json:"index" doc:"the 0-based position of this image within its chapter"`
	Prompt  string `json:"prompt" doc:"a complete self-contained prompt of 2 to 4 sentences, ending with the exact style tag"`
}

// slideAsset is one image the model is asked to write a prompt for.
type slideAsset struct {
	Chapter int
	Index   int
	Title   string
	Summary string
	// Role is the image's job, in the prompt's vocabulary, derived from its
	// position within the chapter rather than asked for.
	Role string
	// Position is where the chapter sits on the video's arc, empty when the
	// video is too short for that to mean much.
	Position string
}

// slidePromptsPrompt is what the templates render against.
type slidePromptsPrompt struct {
	Blueprint            provider.BlueprintOutline
	Assets               []slideAsset
	Total                int
	ExpectedOutputSchema string
}

// SlidePrompts returns every chapter's slide prompts from one generation: the
// prompt carries the whole outline, so chapter 31's slides can answer chapter
// 12's instead of repeating them.
//
// The DAG still holds N retryable per-chapter tasks. Singleflight collapses the
// callers that overlap and the cache serves the ones that come after, since the
// pool is capped well below the number of callers.
func (c *Client) SlidePrompts(ctx context.Context, videoID entity.VideoID) ([]provider.SlidePrompt, error) {
	if cached, ok := c.cached(videoID); ok {
		return cached, nil
	}
	if c.lookup == nil {
		return nil, fmt.Errorf("%w: no video context lookup is wired", ErrUnavailable)
	}

	v, err, _ := c.inflight.Do(string(videoID), func() (any, error) {
		// A caller that queued behind the production must not trigger another.
		if cached, ok := c.cached(videoID); ok {
			return cached, nil
		}
		prompts, err := c.generateSlidePrompts(ctx, videoID)
		if err != nil {
			return nil, err
		}
		c.cacheMu.Lock()
		c.cache[videoID] = prompts
		c.cacheMu.Unlock()
		return prompts, nil
	})
	if err != nil {
		return nil, err
	}
	prompts, ok := v.([]provider.SlidePrompt)
	if !ok {
		// Unreachable: the closure above returns only this type.
		return nil, fmt.Errorf("9router: slide prompt cache holds %T", v)
	}
	return prompts, nil
}

// Forget drops a video's batch so a retry regenerates rather than replays it.
func (c *Client) Forget(videoID entity.VideoID) {
	c.cacheMu.Lock()
	delete(c.cache, videoID)
	c.cacheMu.Unlock()
}

func (c *Client) cached(videoID entity.VideoID) ([]provider.SlidePrompt, bool) {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()
	prompts, ok := c.cache[videoID]
	return prompts, ok
}

// generateSlidePrompts is the real production, run once per video.
func (c *Client) generateSlidePrompts(ctx context.Context, videoID entity.VideoID) ([]provider.SlidePrompt, error) {
	vc, err := c.lookup(ctx, videoID)
	if err != nil {
		return nil, fmt.Errorf("resolve video context: %w", err)
	}
	prompt, err := newSlidePromptsPrompt(vc)
	if err != nil {
		return nil, err
	}
	// No chapters means the graph was expanded from an empty outline, which no
	// number of attempts fixes.
	if prompt.Total == 0 {
		return nil, fmt.Errorf("%w: %s has no chapters to illustrate", ErrUnavailable, videoID)
	}

	system, err := render(slidePromptsSystemPrompt, prompt)
	if err != nil {
		return nil, err
	}
	user, err := render(slidePromptsUserPrompt, prompt)
	if err != nil {
		return nil, err
	}

	content, err := c.chat(ctx, call{Video: videoID, Label: "image-prompts"}, system, user)
	if err != nil {
		return nil, err
	}
	var doc slidePromptsDoc
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return nil, fmt.Errorf("slide prompt response is not JSON: %w (%s)", err, snippet(content))
	}

	out := make([]provider.SlidePrompt, 0, len(doc.Prompts))
	for _, p := range doc.Prompts {
		out = append(out, provider.SlidePrompt{Ordinal: p.Chapter, Index: p.Index, Prompt: p.Prompt})
	}
	// A short batch is a bad roll: the per-chapter tasks address this by
	// (ordinal, index), and a missing pair has no prompt to draw from.
	if len(out) < prompt.Total {
		return nil, fmt.Errorf("9router returned %d slide prompts, want %d", len(out), prompt.Total)
	}
	return out, nil
}

func newSlidePromptsPrompt(vc VideoContext) (slidePromptsPrompt, error) {
	perChapter := vc.SlidesPerChapter
	if perChapter <= 0 {
		perChapter = 1
	}
	assets := make([]slideAsset, 0, len(vc.Chapters)*perChapter)
	for _, ch := range vc.Chapters {
		for i := range perChapter {
			assets = append(assets, slideAsset{
				Chapter:  ch.Ordinal,
				Index:    i,
				Title:    ch.Title,
				Summary:  ch.Summary,
				Role:     slideRole(i),
				Position: macroPosition(ch.Ordinal, len(vc.Chapters)),
			})
		}
	}
	schema, err := jsonSchemaOf(slidePromptsDoc{})
	if err != nil {
		return slidePromptsPrompt{}, err
	}
	return slidePromptsPrompt{
		Blueprint:            vc.BlueprintOutline,
		Assets:               assets,
		Total:                len(assets),
		ExpectedOutputSchema: schema,
	}, nil
}

// slideRole names an image's job from its position in the chapter, in the
// vocabulary the system prompt defines.
func slideRole(index int) string {
	switch index {
	case 0:
		return "ESTABLISHING"
	case 1:
		return "CONCEPT"
	default:
		return "RESONANCE"
	}
}

// macroPosition places a chapter on the video's arc, in the vocabulary the
// system prompt defines. A video too short for an arc gets none rather than a
// spurious one.
func macroPosition(ordinal, chapters int) string {
	if chapters < 5 {
		return ""
	}
	switch {
	case ordinal == 1:
		return "OPENING"
	case ordinal == chapters:
		return "LANDING"
	case ordinal <= chapters/3:
		return "BUILD"
	case ordinal <= 2*chapters/3:
		return "DARK_MIDDLE"
	default:
		return "RECONTEXTUALIZE"
	}
}
