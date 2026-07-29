package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/tbui/yt-studio/domain/repository"
	"github.com/tbui/yt-studio/domain/scheduler"
)

// ResumeScheduler rebuilds every open video's DAG from the database and hands
// it back to the loop.
//
// This is what makes a crash 45 minutes into a run resume rather than restart
// (§2, principle 4): the task table is the state, edges are persisted, and a
// task caught mid-flight is simply re-run because every step is idempotent.
func ResumeScheduler(
	ctx context.Context,
	tasks repository.TaskReader,
	resumer GraphResumer,
	log *slog.Logger,
) (int, error) {
	persisted, err := tasks.ListOpenGraphs(ctx)
	if err != nil {
		return 0, fmt.Errorf("load open graphs: %w", err)
	}
	graphs := make([]*scheduler.Graph, 0, len(persisted))
	for _, vg := range persisted {
		g, err := scheduler.GraphFromPersisted(vg)
		if err != nil {
			// One corrupt graph must not stop the daemon from serving the rest.
			log.Error("skipping unresumable video",
				slog.String("video_id", vg.VideoID.String()),
				slog.String("error", err.Error()))
			continue
		}
		graphs = append(graphs, g)
	}
	if len(graphs) == 0 {
		return 0, nil
	}
	if err := resumer.Resume(ctx, graphs); err != nil {
		return 0, fmt.Errorf("resume graphs: %w", err)
	}
	return len(graphs), nil
}
