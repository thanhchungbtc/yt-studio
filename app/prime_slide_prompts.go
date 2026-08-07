package app

import (
	"context"
	"fmt"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// PrimeSlidePrompts produces the video's whole prompt batch once, on a real LLM
// slot. Without it the per-chapter tasks would trickle through the LLM cap and
// there would never be a batch to coalesce; with it they are cache reads on a
// separate high-concurrency pool.
func PrimeSlidePrompts(
	ctx context.Context,
	t entity.Task,
	llm provider.LLM,
) entity.TaskOutcome {
	prompts, err := llm.SlidePrompts(ctx, t.VideoID)
	if err != nil {
		return classify(fmt.Errorf("prime slide prompts: %w", err))
	}
	if len(prompts) == 0 {
		return entity.Failed{
			Err:       fmt.Errorf("%w: prompt batch is empty", ErrValidation),
			Retryable: true,
		}
	}
	return entity.Success{}
}
