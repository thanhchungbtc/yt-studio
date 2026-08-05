package app

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// RetryTask resets one task and everything downstream of it.
func RetryTask(
	ctx context.Context,
	tasks repository.TaskReader,
	retrier TaskRetrier,
	prompts PromptCacheInvalidator,
	id entity.TaskID,
) (entity.Task, error) {
	t, err := tasks.TaskByID(ctx, id)
	if err != nil {
		return entity.Task{}, err
	}
	// Re-priming the batch is the point of retrying either prompt task.
	if prompts != nil && (t.Kind == entity.TaskKindPrimeSlidePrompts || t.Kind == entity.TaskKindSlidePrompts) {
		prompts.Forget(t.VideoID)
	}
	if err := retrier.RetryTask(ctx, id); err != nil {
		return entity.Task{}, err
	}
	t.State = entity.TaskStateBlocked
	t.Attempt = 0
	t.Error = ""
	return t, nil
}
