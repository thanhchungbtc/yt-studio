package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
	"github.com/tbui/yt-studio/domain/scheduler"
)

// StartVideoOptions are the scheduler-shaped inputs, all sourced from settings
// rows by the caller.
//
// The upload gate is not among them: it is carried by the metadata task, which
// is part of the tail, so it is read when the graph expands rather than when
// the video is enqueued. See ExpandOptions.
type StartVideoOptions struct {
	MaxAttempts   int
	BlueprintGate bool
}

// StartVideo enqueues a video by scheduling its blueprint, and hands the
// resulting graph to the scheduler.
//
// Only the blueprint is scheduled. Everything below it is one branch per
// chapter, and the chapter count is not known yet: it is the blueprint's output
// rather than its input, fixed when the operator accepts the outline. The rest
// of the DAG is spliced on then, by ExpandVideoGraph.
//
// It is idempotent: a video that already has tasks is already enqueued, so
// starting it again schedules nothing new.
//
// That check reads the task table rather than relying on the scheduler holding
// the video in memory. A video whose tasks are all cancelled is not resumed at
// startup, so after a restart the loop knows nothing about it — and submitting
// a fresh head graph over a DAG that has already expanded would leave the loop
// believing the video is one node while the database holds hundreds.
func StartVideo(
	ctx context.Context,
	videos repository.VideoReader,
	tasks repository.TaskReader,
	submitter GraphSubmitter,
	now time.Time,
	opts StartVideoOptions,
	key string,
) (entity.Video, error) {
	v, err := GetVideo(ctx, videos, key)
	if err != nil {
		return entity.Video{}, err
	}
	if v.State == entity.VideoStateCompleted {
		return entity.Video{}, fmt.Errorf("%w: %s has already completed", ErrConflict, v.Ref)
	}
	counts, err := tasks.CountTasksByVideo(ctx, v.ID)
	if err != nil {
		return entity.Video{}, err
	}
	if counts.Total > 0 {
		return v, nil
	}

	g, err := scheduler.BuildHeadGraph(scheduler.HeadSpec{
		VideoID:       v.ID,
		MaxAttempts:   opts.MaxAttempts,
		BlueprintGate: opts.BlueprintGate,
		Now:           now,
	})
	if err != nil {
		return entity.Video{}, fmt.Errorf("%w: %w", ErrValidation, err)
	}
	if err := submitter.Submit(ctx, g); err != nil {
		return entity.Video{}, fmt.Errorf("submit %s: %w", v.Ref, err)
	}
	return v, nil
}
