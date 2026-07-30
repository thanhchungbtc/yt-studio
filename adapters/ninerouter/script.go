package ninerouter

import (
	"context"
	"regexp"
	"strconv"
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

// briefWords finds the word budget the blueprint wrote into this chapter's
// brief, in the direction footer it ends with — "exploration · curious · deep ·
// ~420 words".
//
// The budget is per-chapter and deliberately uneven: a deep chapter carries
// roughly twice a short one. It reaches us inside the brief text because
// ScriptRequest carries a summary and not a plan, so it is read back out here
// rather than flattened to the channel average.
var briefWords = regexp.MustCompile(`~\s*(\d+)\s+words`)

func newScriptPrompt(req provider.ScriptRequest) scriptPrompt {
	target := wordsPerChapter(req.Style)
	if match := briefWords.FindStringSubmatch(req.ChapterSummary); match != nil {
		if n, err := strconv.Atoi(match[1]); err == nil && n > 0 {
			target = n
		}
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
