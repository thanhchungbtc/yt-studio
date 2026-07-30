package ninerouter

import (
	"context"
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
	// TargetWords is the resolved budget. It is separate from Chapter's own
	// figure because that one may be zero, and a prompt that asks for zero
	// words is worse than one that falls back to the channel average.
	TargetWords int
	Style       entity.StyleConfig
}

// newScriptPrompt finds the chapter in the outline and resolves its budget.
//
// The chapter is looked up rather than passed alongside, so the assignment and
// the outline entry it points at are the same object by construction.
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
		target = wordsPerChapter(req.Style)
	}
	return scriptPrompt{
		Blueprint:   req.Blueprint,
		Chapter:     ch,
		TargetWords: target,
		Style:       req.Style,
	}, nil
}

// Script writes one chapter's narration.
//
// Unlike the blueprint this returns prose rather than JSON, so there is nothing
// to parse: the completion is the narration. That also means there is no parse
// error to catch a model that prefaced its answer, which is why the system
// prompt spends a section on it — every character here is read aloud.
func (c *Client) Script(ctx context.Context, req provider.ScriptRequest) (provider.Script, error) {
	prompt, err := newScriptPrompt(req)
	if err != nil {
		return provider.Script{}, err
	}
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
