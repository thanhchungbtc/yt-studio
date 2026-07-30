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

// StartVideo enqueues a video by scheduling its blueprint, or resumes one whose
// DAG already exists.
//
// Only the blueprint is scheduled on the first pass. Everything below it is one
// branch per chapter, and the chapter count is not known yet: it is the
// blueprint's output rather than its input, fixed when the operator accepts the
// outline. The rest of the DAG is spliced on then, by ExpandVideoGraph.
//
// A video that already has tasks takes the resume path instead. Submitting a
// fresh head graph over a DAG that has already expanded would leave the loop
// believing the video is one node while the database holds hundreds, so what
// "start it again" can mean there is to requeue whatever it stopped on —
// cancelled tasks after a cancel, failed ones after an exhausted retry. A video
// that stopped on nothing is left exactly as it is, so the call stays
// idempotent.
//
// The graph is handed back to the loop before the requeue because the loop may
// not be holding it: a video whose tasks are all terminal is not among the open
// graphs reloaded at startup, so after a restart a cancelled video is one the
// scheduler has never heard of. Resume admits it only if it is unknown, which
// makes this a no-op in the ordinary case.
//
//nolint:revive // the parameter list is the dependency list
func StartVideo(
	ctx context.Context,
	videos repository.VideoReader,
	tasks repository.TaskReader,
	submitter GraphSubmitter,
	resumer GraphResumer,
	requeuer VideoRequeuer,
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
		return v, resumeVideo(ctx, tasks, resumer, requeuer, v)
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

// resumeVideo re-admits a video's persisted DAG and requeues what it stopped on.
func resumeVideo(
	ctx context.Context,
	tasks repository.TaskReader,
	resumer GraphResumer,
	requeuer VideoRequeuer,
	v entity.Video,
) error {
	persisted, err := tasks.GraphByVideo(ctx, v.ID)
	if err != nil {
		return err
	}
	g, err := scheduler.GraphFromPersisted(persisted)
	if err != nil {
		return fmt.Errorf("%w: rebuild %s: %w", ErrValidation, v.Ref, err)
	}
	if err := resumer.Resume(ctx, []*scheduler.Graph{g}); err != nil {
		return fmt.Errorf("resume %s: %w", v.Ref, err)
	}
	if _, err := requeuer.Requeue(ctx, v.ID); err != nil {
		return fmt.Errorf("requeue %s: %w", v.Ref, err)
	}
	return nil
}
