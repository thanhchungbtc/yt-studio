package app

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// RerunPlan is what a re-run is about to do: the tasks that will run again, and
// the tasks that will be flagged stale rather than run.
type RerunPlan struct {
	Rerun []entity.Task
	Stale []entity.Task
}

// RerunTasks re-runs tasks that have already succeeded. Unlike RetryTask it does
// not cascade: the dependents hold artifacts an operator may have reviewed, so
// they are flagged stale and left for a decision.
//
// With dryRun nothing changes and the plan describes what would happen.
func RerunTasks(
	ctx context.Context,
	tasks repository.TaskReader,
	rerunner TaskRerunner,
	prompts PromptCacheInvalidator,
	videoID entity.VideoID,
	ids []entity.TaskID,
	dryRun bool,
) (RerunPlan, error) {
	if len(ids) == 0 {
		return RerunPlan{}, Invalid("tasks", "name at least one task to re-run")
	}

	seeds := make([]entity.Task, 0, len(ids))
	for _, id := range ids {
		t, err := tasks.TaskByID(ctx, id)
		if err != nil {
			return RerunPlan{}, err
		}
		if t.VideoID != videoID {
			return RerunPlan{}, Invalid("tasks", "task "+string(id)+" belongs to another video")
		}
		seeds = append(seeds, t)
	}

	// Re-priming the batch is the point of re-running either prompt task.
	if prompts != nil && !dryRun {
		for _, t := range seeds {
			if t.Kind == entity.TaskKindPrimeImagePrompts || t.Kind == entity.TaskKindImagePrompts {
				prompts.Forget(videoID)
				break
			}
		}
	}

	affected, err := rerunner.Rerun(ctx, videoID, ids, dryRun)
	if err != nil {
		return RerunPlan{}, err
	}
	stale, err := loadTasks(ctx, tasks, affected)
	if err != nil {
		return RerunPlan{}, err
	}
	if !dryRun {
		// The scheduler batches its writes, so the rows just read still show the
		// pre-run flag.
		for i := range stale {
			stale[i].Stale = true
		}
	}
	return RerunPlan{Rerun: seeds, Stale: stale}, nil
}

// RunStaleTasks re-runs flagged tasks. A nil id list means all of them.
func RunStaleTasks(
	ctx context.Context,
	runner StaleRunner,
	prompts PromptCacheInvalidator,
	videoID entity.VideoID,
	ids []entity.TaskID,
) (int, error) {
	// The stale set may include the per-chapter prompt reads, and replaying the
	// cached batch would reproduce exactly what is being re-run.
	if prompts != nil {
		prompts.Forget(videoID)
	}
	return runner.RunStale(ctx, videoID, ids)
}

// AcceptStaleTasks clears the flag without re-running anything. Staleness is
// pessimistic — it records that an input moved, not that the output is wrong —
// so an operator who has checked the artifact can keep it.
func AcceptStaleTasks(
	ctx context.Context,
	accepter StaleAccepter,
	videoID entity.VideoID,
	ids []entity.TaskID,
) (int, error) {
	return accepter.AcceptStale(ctx, videoID, ids)
}

// loadTasks resolves ids to rows, skipping any that have since disappeared.
func loadTasks(
	ctx context.Context,
	tasks repository.TaskReader,
	ids []entity.TaskID,
) ([]entity.Task, error) {
	out := make([]entity.Task, 0, len(ids))
	for _, id := range ids {
		t, err := tasks.TaskByID(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}
