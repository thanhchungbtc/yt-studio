package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/internal/domain/entity"
	"github.com/tbui/yt-studio/internal/domain/repository"
)

// FindOpenGates returns the tasks a video is parked on, in task order. A gate
// is a row update, so an open gate is simply a task in awaiting_approval.
//
// A slice rather than one task because the script gate rides on every chapter's
// script at once: the whole stage is one question, and one answer settles it.
// The blueprint and upload gates ride on a single node each and so return one,
// which is why nothing above here had to learn a new shape.
//
// Two different gates can never be open together — the blueprint's tail is not
// spliced until it is approved, and the upload gate sits downstream of every
// script — so the returned set is always one gate's.
func FindOpenGates(
	ctx context.Context,
	tasks repository.TaskReader,
	videoID entity.VideoID,
	gate entity.GateKind,
) ([]entity.Task, error) {
	rows, err := tasks.ListTasksByVideo(ctx, videoID)
	if err != nil {
		return nil, err
	}
	open := make([]entity.Task, 0, 1)
	for _, t := range rows {
		if t.State != entity.TaskStateAwaitingApproval {
			continue
		}
		if gate != entity.GateNone && t.Gate != gate {
			continue
		}
		open = append(open, t)
	}
	if len(open) == 0 {
		return nil, fmt.Errorf("%w: video %s is not awaiting approval", ErrConflict, videoID)
	}
	return open, nil
}

// ApproveGate releases the gated tasks' successors and reports the first of
// them. Waits may last days, so the state lives in the task table and the
// server may have restarted since.
//
// Approving a blueprint also builds the rest of the DAG: until now the video is
// one node, because its chapter branches are the chapters being approved.
//
// The script gate parks one task per chapter, so this releases a set. It does
// so one command at a time rather than atomically, which is worth what it
// saves: a call that dies halfway leaves the rest parked and still open, and
// pressing Approve again finds exactly those — the ones already released are no
// longer awaiting_approval, so a second answer is not a second release.
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
	open, err := FindOpenGates(ctx, tasks, videoID, gate)
	if err != nil {
		return entity.Task{}, err
	}
	if open[0].Kind == entity.TaskKindBlueprint {
		// Expansion precedes approval: releasing dependents before they exist
		// would leave a video whose whole DAG had succeeded after one task.
		if err := ExpandVideoGraph(ctx, videos, chapters, expander, now, opts, videoID); err != nil {
			return entity.Task{}, err
		}
	}
	for _, t := range open {
		if err := approver.Approve(ctx, t.ID); err != nil {
			return entity.Task{}, err
		}
	}
	t := open[0]
	t.State = entity.TaskStateSucceeded
	return t, nil
}
