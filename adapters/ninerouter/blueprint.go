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

// blueprintDoc is the outline as the model returns it and as the store keeps
// it. The two are the same shape on purpose: what an operator reads in the
// asset viewer is what the pipeline was built from.
type blueprintDoc struct {
	Video    entity.VideoID `json:"video"`
	Ref      entity.Ref     `json:"ref"`
	Title    string         `json:"title"`
	Summary  string         `json:"summary"`
	Chapters []chapterDoc   `json:"chapters"`
}

type chapterDoc struct {
	// Ordinal is filled in here rather than asked for. Position in the array is
	// the answer, and a model numbering its own chapters gets it wrong often
	// enough to be a nuisance.
	Ordinal int    `json:"ordinal"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

// Blueprint plans a video's chapters.
//
// The chapter count is deliberately not checked here. The request carries a
// target, and how far an outline may land from it is a policy the use case
// owns; enforcing it twice would mean two places to change and an adapter that
// rejects an outline before the policy gets a say.
func (c *Client) Blueprint(ctx context.Context, req provider.BlueprintRequest) (provider.Blueprint, error) {
	system, err := render(blueprintSystemPrompt, req)
	if err != nil {
		return provider.Blueprint{}, err
	}
	user, err := render(blueprintUserPrompt, req)
	if err != nil {
		return provider.Blueprint{}, err
	}

	content, err := c.chat(ctx, system, user)
	if err != nil {
		return provider.Blueprint{}, err
	}

	var doc blueprintDoc
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return provider.Blueprint{}, fmt.Errorf("blueprint response is not JSON: %w (%s)",
			err, snippet(content))
	}
	if err := doc.normalise(req); err != nil {
		return provider.Blueprint{}, err
	}

	assetID, err := c.putJSON(ctx, entity.AssetKindBlueprint, doc)
	if err != nil {
		return provider.Blueprint{}, err
	}

	chapters := make([]provider.BlueprintChapter, 0, len(doc.Chapters))
	for _, ch := range doc.Chapters {
		chapters = append(chapters, provider.BlueprintChapter{
			Ordinal: ch.Ordinal,
			Title:   ch.Title,
			Summary: ch.Summary,
		})
	}
	return provider.Blueprint{
		Title:    doc.Title,
		Summary:  doc.Summary,
		Chapters: chapters,
		AssetID:  assetID,
	}, nil
}

// normalise tidies what the model returned and rejects what cannot be used.
//
// It stamps the identity fields rather than trusting them: they are the
// caller's facts, not the model's to get wrong.
func (d *blueprintDoc) normalise(req provider.BlueprintRequest) error {
	d.Video, d.Ref = req.VideoID, req.VideoRef
	d.Title = strings.TrimSpace(d.Title)
	if d.Title == "" {
		d.Title = req.Title
	}
	d.Summary = strings.TrimSpace(d.Summary)

	if len(d.Chapters) == 0 {
		return fmt.Errorf("blueprint for %s has no chapters", req.VideoRef)
	}
	for i := range d.Chapters {
		ch := &d.Chapters[i]
		ch.Ordinal = i + 1
		ch.Title = strings.TrimSpace(ch.Title)
		ch.Summary = strings.TrimSpace(ch.Summary)
		if ch.Title == "" {
			return fmt.Errorf("blueprint for %s has an untitled chapter at position %d", req.VideoRef, i+1)
		}
	}
	return nil
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
