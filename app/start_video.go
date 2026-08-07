package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
	"github.com/tbui/yt-studio/domain/scheduler"
)

// StartVideoOptions are the settings-sourced scheduler inputs. The upload gate
// is not among them: it rides on a task in the tail, so it is read at expansion
// instead. See ExpandOptions.
type StartVideoOptions struct {
	MaxAttempts   int
	BlueprintGate bool
}

// StartVideo enqueues a video by scheduling its blueprint, or resumes one whose
// DAG already exists. Only the blueprint goes on the first pass; ExpandVideoGraph
// splices the rest on once the outline is accepted.
//
// A video that already has tasks takes the resume path, because submitting a
// fresh head graph over an expanded DAG would tell the loop the video is one
// node while the database holds hundreds. Starting it again means requeueing
// whatever it stopped on; a video that stopped on nothing is left alone.
//
// The graph is re-admitted before the requeue because the loop may not hold it:
// a video whose tasks are all terminal is not reloaded at startup. Resume
// ignores a graph it already has, so that is a no-op in the ordinary case.
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
