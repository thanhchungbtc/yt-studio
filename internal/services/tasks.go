package services

import (
	"context"

	"github.com/tbui/yt-studio/internal/app"
	"github.com/tbui/yt-studio/internal/domain/entity"
)

// TaskService reads a video's DAG and runs its tasks again.
type TaskService struct{ b *Backend }

// NewTaskService creates a TaskService.
func NewTaskService(b *Backend) *TaskService { return &TaskService{b: b} }

// RerunPlan is what a re-run ran again and what it flagged stale.
type RerunPlan struct {
	DryRun bool      `json:"dryRun"`
	Rerun  []TaskDTO `json:"rerun"`
	Stale  []TaskDTO `json:"stale"`
}

// List returns a video's whole DAG.
func (s *TaskService) List(ctx context.Context, video string) ([]TaskDTO, error) {
	v, err := s.b.video(ctx, video)
	if err != nil {
		return nil, err
	}
	rows, err := app.ListTasksByVideo(ctx, s.b.Store, v.ID)
	if err != nil {
		return nil, err
	}
	return taskDTOs(rows), nil
}

// Rerun runs succeeded tasks again, flagging downstream tasks stale.
func (s *TaskService) Rerun(ctx context.Context, video string, ids []string) (RerunPlan, error) {
	if len(ids) == 0 {
		return RerunPlan{}, app.Invalid("taskIds", "must name at least one task")
	}
	v, err := s.b.video(ctx, video)
	if err != nil {
		return RerunPlan{}, err
	}
	plan, err := app.RerunTasks(ctx, s.b.Store, s.b.Scheduler, s.b.Prompts, v.ID, taskIDs(ids), false)
	if err != nil {
		return RerunPlan{}, err
	}
	return RerunPlan{Rerun: taskDTOs(plan.Rerun), Stale: taskDTOs(plan.Stale)}, nil
}

// Retry runs a failed task and everything under it again.
func (s *TaskService) Retry(ctx context.Context, id string) (TaskDTO, error) {
	t, err := app.RetryTask(ctx, s.b.Store, s.b.Scheduler, s.b.Prompts, entity.TaskID(id))
	if err != nil {
		return TaskDTO{}, err
	}
	return taskFrom(t), nil
}

// AcceptStale clears stale flags (no ids: all) and returns how many.
func (s *TaskService) AcceptStale(ctx context.Context, video string, ids []string) (int, error) {
	v, err := s.b.video(ctx, video)
	if err != nil {
		return 0, err
	}
	return app.AcceptStaleTasks(ctx, s.b.Scheduler, v.ID, taskIDs(ids))
}
