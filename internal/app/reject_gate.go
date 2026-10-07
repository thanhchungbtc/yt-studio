package app

import (
	"context"
	"fmt"

	"github.com/tbui/yt-studio/internal/domain/entity"
	"github.com/tbui/yt-studio/internal/domain/repository"
)

// RejectGate fails the gated tasks with an operator-supplied reason, leaving
// the video parked until they are retried, and reports the first of them.
//
// It rejects the whole set the gate holds, symmetrically with ApproveGate: the
// script gate asks about a stage, so sending it back sends the stage back. The
// screen offers no Reject on that gate for exactly that reason — one press
// would discard every chapter's narration, and the per-chapter Regenerate in
// the pipeline table is the surgical instrument.
func RejectGate(
	ctx context.Context,
	tasks repository.TaskReader,
	rejecter GateRejecter,
	videoID entity.VideoID,
	gate entity.GateKind,
	reason string,
) (entity.Task, error) {
	if !gate.Valid() {
		return entity.Task{}, Invalid("gate", fmt.Sprintf("must be one of %v", entity.AllGateKinds))
	}
	open, err := FindOpenGates(ctx, tasks, videoID, gate)
	if err != nil {
		return entity.Task{}, err
	}
	for _, t := range open {
		if err := rejecter.Reject(ctx, t.ID, reason); err != nil {
			return entity.Task{}, err
		}
	}
	t := open[0]
	t.State = entity.TaskStateFailed
	t.Error = reason
	return t, nil
}
