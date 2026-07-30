package app

import (
	"context"
	"fmt"
	"sort"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// ResolveImagePrompts takes one chapter's slice of the coalesced batch.
//
// The DAG keeps N clean per-chapter prompt tasks — individually retryable and
// uniform with every other chapter task — while the provider serves them all
// from one production behind singleflight.
func ResolveImagePrompts(
	ctx context.Context,
	t entity.Task,
	llm provider.LLMProvider,
	chapters repository.ChapterReader,
	fields repository.ChapterFieldWriter,
) entity.TaskOutcome {
	if t.ChapterID == nil {
		return entity.Failed{Err: fmt.Errorf("%w: prompt task has no chapter", ErrValidation), Retryable: false}
	}
	chapter, err := chapters.ChapterByID(ctx, *t.ChapterID)
	if err != nil {
		return classify(err)
	}
	batch, err := llm.ImagePrompts(ctx, t.VideoID)
	if err != nil {
		return classify(fmt.Errorf("resolve image prompts: %w", err))
	}

	mine := make([]provider.ImagePrompt, 0, 4)
	for _, p := range batch {
		if p.Ordinal == chapter.Ordinal {
			mine = append(mine, p)
		}
	}
	if len(mine) == 0 {
		return entity.Failed{
			Err:       fmt.Errorf("%w: batch has no prompts for chapter %d", ErrValidation, chapter.Ordinal),
			Retryable: true,
		}
	}
	sort.Slice(mine, func(i, j int) bool { return mine[i].Index < mine[j].Index })

	prompts := make([]string, 0, len(mine))
	for _, p := range mine {
		prompts = append(prompts, p.Prompt)
	}
	if err := fields.SetChapterPrompts(ctx, chapter.ID, prompts); err != nil {
		return classify(err)
	}
	return entity.Success{}
}
