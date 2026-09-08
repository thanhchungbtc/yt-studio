package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
	"github.com/tbui/yt-studio/domain/scheduler"
)

// readmitting runs a scheduler command, and if the scheduler has forgotten the
// video, loads its DAG back from the database and runs the command again.
//
// A graph lives in the loop's memory, and the loop is only ever handed the ones
// with open tasks: `ResumeScheduler` calls `ListOpenGraphs` at startup, so a
// video that finished before the last restart is not there. Nothing dropped it
// on purpose — `Forget` runs only when a channel is deleted — it simply was
// never loaded back.
//
// Which makes "unknown video" a fact about the process's memory rather than
// about the video, and the wrong thing to report to somebody who has just
// edited a prompt on a finished video and wants the one tile redrawn. Being
// finished is not a reason a task cannot run again.
//
// The command runs first and the graph is loaded only when it has to be, so the
// usual path — a video still running, or one finished since the last restart —
// costs nothing at all.
func readmitting[T any](
	ctx context.Context,
	tasks repository.TaskReader,
	resumer GraphResumer,
	videoID entity.VideoID,
	run func() (T, error),
) (T, error) {
	out, err := run()
	if err == nil || !errors.Is(err, scheduler.ErrUnknownVideo) {
		return out, err
	}
	// Nil rather than a missing dependency: the callers below reach this from
	// screens that were built before there was anything to re-admit, and an
	// unwired one should behave as it did rather than panic.
	if resumer == nil {
		return out, err
	}

	persisted, err := tasks.GraphByVideo(ctx, videoID)
	if err != nil {
		return out, fmt.Errorf("reload graph of %s: %w", videoID, err)
	}
	g, err := scheduler.GraphFromPersisted(persisted)
	if err != nil {
		return out, fmt.Errorf("rebuild graph of %s: %w", videoID, err)
	}
	if err := resumer.Resume(ctx, []*scheduler.Graph{g}); err != nil {
		return out, fmt.Errorf("re-admit %s: %w", videoID, err)
	}
	// Resume is a no-op for a video the loop already holds, so this cannot loop:
	// either the graph is there now, or the second failure is a real one.
	return run()
}
