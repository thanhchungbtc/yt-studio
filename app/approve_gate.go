package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// FindOpenGate returns the task a video is currently parked on, if any.
//
// A gate is a row update: there is no in-memory flow to suspend, so the open
// gate is simply the one task in awaiting_approval.
func FindOpenGate(
	ctx context.Context,
	tasks repository.TaskReader,
	videoID entity.VideoID,
	gate entity.GateKind,
) (entity.Task, error) {
	rows, err := tasks.ListTasksByVideo(ctx, videoID)
	if err != nil {
		return entity.Task{}, err
	}
	for _, t := range rows {
		if t.State != entity.TaskStateAwaitingApproval {
			continue
		}
		if gate != entity.GateNone && t.Gate != gate {
			continue
		}
		return t, nil
	}
	return entity.Task{}, fmt.Errorf("%w: video %s is not awaiting approval", ErrConflict, videoID)
}

// ApproveGate releases a gated task's successors. Waits may last days, so the
// state lives in the task table and the daemon may have restarted since the
// gate opened.
//
// Approving a blueprint does one thing more: it builds the rest of the video's
// DAG. Until this moment a video is a single blueprint node, because the number
// of chapter branches it needs is the number of chapters the operator is
// approving right now.
//
//nolint:revive // the parameter list is the dependency list
func ApproveGate(
	ctx context.Context,
	tasks repository.TaskReader,
	videos repository.VideoReader,
	chapters repository.ChapterReader,
	expander GraphExpander,
	approver GateApprover,
	now time.Time,
	opts ExpandOptions,
	videoID entity.VideoID,
	gate entity.GateKind,
) (entity.Task, error) {
	if !gate.Valid() {
		return entity.Task{}, Invalid("gate", fmt.Sprintf("must be one of %v", entity.AllGateKinds))
	}
	t, err := FindOpenGate(ctx, tasks, videoID, gate)
	if err != nil {
		return entity.Task{}, err
	}
	if t.Kind == entity.TaskKindBlueprint {
		// Expansion precedes approval, not the other way round: releasing the
		// blueprint's dependents before they exist would leave a video whose whole
		// DAG had succeeded after one task.
		if err := ExpandVideoGraph(ctx, videos, chapters, expander, now, opts, videoID); err != nil {
			return entity.Task{}, err
		}
	}
	if err := approver.Approve(ctx, t.ID); err != nil {
		return entity.Task{}, err
	}
	t.State = entity.TaskStateSucceeded
	return t, nil
}
