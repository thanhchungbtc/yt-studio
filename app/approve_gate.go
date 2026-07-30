package app

import (
	"context"
	"fmt"

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
func ApproveGate(
	ctx context.Context,
	tasks repository.TaskReader,
	approver GateApprover,
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
	if err := approver.Approve(ctx, t.ID); err != nil {
		return entity.Task{}, err
	}
	t.State = entity.TaskStateSucceeded
	return t, nil
}
