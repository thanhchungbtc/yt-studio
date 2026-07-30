package ninerouter

import (
	"context"
	"strings"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// scriptPrompt is what the script templates render against: the request, plus
// the word budget resolved from the brief or the channel.
type scriptPrompt struct {
	provider.ScriptRequest
	TargetWords int
}

// newScriptPrompt resolves the chapter's word budget: the one the blueprint
// assigned if it assigned one, and the channel's per-chapter average otherwise.
//
// The budget is deliberately uneven across a video — a deep chapter carries
// roughly twice a short one — so falling back to the average flattens pacing
// the outline was built around. It arrives as a field rather than as prose
// inside the brief, which is what makes that fallback rare and visible.
func newScriptPrompt(req provider.ScriptRequest) scriptPrompt {
	target := req.TargetWords
	if target <= 0 {
		target = wordsPerChapter(req.Style)
	}
	return scriptPrompt{ScriptRequest: req, TargetWords: target}
}

// Script writes one chapter's narration.
//
// Unlike the blueprint this returns prose rather than JSON, so there is nothing
// to parse: the completion is the narration. That also means there is no parse
// error to catch a model that prefaced its answer, which is why the system
// prompt spends a section on it — every character here is read aloud.
func (c *Client) Script(ctx context.Context, req provider.ScriptRequest) (provider.Script, error) {
	prompt := newScriptPrompt(req)
	system, err := render(scriptSystemPrompt, prompt)
	if err != nil {
		return provider.Script{}, err
	}
	user, err := render(scriptUserPrompt, prompt)
	if err != nil {
		return provider.Script{}, err
	}

	text, err := c.chat(ctx, system, user)
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
