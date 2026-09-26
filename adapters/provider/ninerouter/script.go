package ninerouter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// scriptPrompt is what the script templates render against: the whole outline,
// the one chapter being written, and the budget it has to hit.
type scriptPrompt struct {
	Blueprint provider.BlueprintOutline
	Chapter   provider.BlueprintChapter
	// TargetWords is the resolved budget, separate from Chapter's own figure
	// because that may be zero and a prompt asking for zero words is worse.
	TargetWords int
}

// newScriptPrompt finds the chapter in the outline and resolves its budget. It
// is looked up rather than passed alongside, so the assignment and the outline
// entry are the same object by construction.
func newScriptPrompt(req provider.ScriptRequest) (scriptPrompt, error) {
	ch, ok := req.Blueprint.Chapter(req.Ordinal)
	if !ok {
		return scriptPrompt{}, fmt.Errorf(
			"chapter %d is not in the outline of %s", req.Ordinal, req.VideoID)
	}
	target := req.TargetWords
	if target <= 0 {
		target = ch.EstimatedWords
	}
	if target <= 0 {
		target = entity.DefaultWordsPerChapter
	}
	return scriptPrompt{Blueprint: req.Blueprint, Chapter: ch, TargetWords: target}, nil
}

// Script writes one chapter's narration. The completion is the narration —
// prose, not JSON — so no parse error catches a model that prefaced its answer,
// which is why the system prompt spends a section on it.
//
// Narration offered in the request is taken as written, chapter by chapter. A
// chapter the document does not cover is written by the model as usual, so a
// half-finished set of scripts is a video the two of you wrote together rather
// than a request that fails.
func (c *Client) Script(ctx context.Context, req provider.ScriptRequest) (provider.Script, error) {
	text, err := c.scriptText(ctx, req)
	if err != nil {
		return provider.Script{}, err
	}

	assetID, err := c.putText(ctx, entity.AssetKindScript, text)
	if err != nil {
		return provider.Script{}, err
	}
	return provider.Script{
		Text:      text,
		WordCount: len(strings.Fields(text)),
		AssetID:   assetID,
	}, nil
}

// preparedScripts is the prepared document as this path reads it.
//
// Stripped to one field on purpose: the outline is already in req.Blueprint,
// projected from the chapter rows, so declaring it again here would be a second
// copy to disagree. Everything else in the document is ignored.
type preparedScripts struct {
	Chapters []struct {
		Script string `json:"script"`
	} `json:"chapters"`
}

// scriptText answers where one chapter's narration comes from: the prepared
// document if it covers this chapter, and the model otherwise.
//
// Only the source differs. What comes back is stored, counted and addressed
// identically either way, so nothing downstream can tell which chapters were
// written here and which were handed in.
func (c *Client) scriptText(ctx context.Context, req provider.ScriptRequest) (string, error) {
	if raw := req.Options.PreparedScripts; len(raw) > 0 {
		var doc preparedScripts
		// Malformed is an error rather than a fall-through. The bytes are the
		// operator's, they will not parse differently next time, and quietly
		// writing a chapter they meant to supply is the one outcome nobody could
		// detect.
		if err := json.Unmarshal(raw, &doc); err != nil {
			return "", fmt.Errorf("prepared scripts are not JSON: %w (%s)", err, snippet(string(raw)))
		}
		// Matched by position rather than by the document's own order field.
		// normalise renumbers chapters from their position, so chapters[i] is the
		// chapter that becomes ordinal i+1 whatever it called itself — and its
		// script sits inside it, so the two cannot come apart.
		if i := req.Ordinal - 1; i >= 0 && i < len(doc.Chapters) {
			// Blank counts as absent: an empty narration would give the TTS task
			// nothing and fail three stages below, pointing at the wrong one.
			if text := strings.TrimSpace(doc.Chapters[i].Script); text != "" {
				return text, nil
			}
		}
	}

	prompt, err := newScriptPrompt(req)
	if err != nil {
		return "", err
	}
	system, err := render(scriptSystemPrompt, prompt)
	if err != nil {
		return "", err
	}
	user, err := render(scriptUserPrompt, prompt)
	if err != nil {
		return "", err
	}
	return c.chat(ctx,
		call{Video: req.VideoID, Label: fmt.Sprintf("script-ch%d", req.Ordinal), Kind: KindScript}, system, user)
}
