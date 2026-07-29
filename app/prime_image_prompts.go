package app

import (
	"context"
	"fmt"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// PrimeImagePrompts forces the video's whole prompt batch to be produced once.
//
// This task exists because the LLM pool is capped at 2: without it, the N
// per-chapter prompt tasks would trickle through that cap and there would never
// be a batch to coalesce. It occupies a real LLM slot; the per-chapter tasks
// then fan out as cheap cache reads on a separate high-concurrency pool (§4).
func PrimeImagePrompts(
	ctx context.Context,
	t entity.Task,
	llm provider.LLMProvider,
) entity.TaskOutcome {
	prompts, err := llm.ImagePrompts(ctx, t.VideoID)
	if err != nil {
		return classify(fmt.Errorf("prime image prompts: %w", err))
	}
	if len(prompts) == 0 {
		return entity.Failed{
			Err:       fmt.Errorf("%w: prompt batch is empty", ErrValidation),
			Retryable: true,
		}
	}
	return entity.Success{}
}
