package app

import (
	"context"
	"fmt"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// RejectGate fails a gated task with an operator-supplied reason, leaving the
// video parked until it is retried.
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
	t, err := FindOpenGate(ctx, tasks, videoID, gate)
	if err != nil {
		return entity.Task{}, err
	}
	if err := rejecter.Reject(ctx, t.ID, reason); err != nil {
		return entity.Task{}, err
	}
	t.State = entity.TaskStateFailed
	t.Error = reason
	return t, nil
}
