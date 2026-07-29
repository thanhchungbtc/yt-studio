package http

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// TasksOutput is a task list response.
type TasksOutput struct {
	Body struct {
		Tasks []TaskDTO `json:"tasks"`
	}
}

// TaskIDInput identifies one task.
type TaskIDInput struct {
	ID string `path:"id" doc:"Task id"`
}

// RecentTasksInput bounds the console's live table.
type RecentTasksInput struct {
	Limit int `query:"limit" default:"200" minimum:"1" maximum:"2000"`
}

func getVideoTasks(videos repository.VideoReader, tasks repository.TaskReader) func(context.Context, *VideoKeyInput) (*TasksOutput, error) {
	return func(ctx context.Context, in *VideoKeyInput) (*TasksOutput, error) {
		v, err := app.GetVideo(ctx, videos, in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		rows, err := app.ListTasksByVideo(ctx, tasks, v.ID)
		if err != nil {
			return nil, mapError(err)
		}
		return tasksOutput(rows), nil
	}
}

func getRecentTasks(tasks repository.TaskReader) func(context.Context, *RecentTasksInput) (*TasksOutput, error) {
	return func(ctx context.Context, in *RecentTasksInput) (*TasksOutput, error) {
		rows, err := app.ListRecentTasks(ctx, tasks, in.Limit)
		if err != nil {
			return nil, mapError(err)
		}
		return tasksOutput(rows), nil
	}
}

func postTaskRetry(
	tasks repository.TaskReader,
	retrier app.TaskRetrier,
	prompts app.PromptCacheInvalidator,
) func(context.Context, *TaskIDInput) (*TaskOutput, error) {
	return func(ctx context.Context, in *TaskIDInput) (*TaskOutput, error) {
		t, err := app.RetryTask(ctx, tasks, retrier, prompts, entity.TaskID(in.ID))
		if err != nil {
			return nil, mapError(err)
		}
		return &TaskOutput{Body: taskFrom(t)}, nil
	}
}

// RerunInput asks to re-run tasks that have already succeeded.
type RerunInput struct {
	Key  string `path:"key" doc:"Video ref or id"`
	Body struct {
		TaskIDs []string `json:"taskIds" required:"true" minItems:"1" doc:"Tasks to run again"`
		// omitempty, or huma makes it required: a re-run without the flag is the
		// ordinary case and must not be a validation error.
		DryRun bool `json:"dryRun,omitempty" doc:"Report the blast radius without changing anything"`
	}
}

// RerunOutput is the plan: what will run, and what will be flagged stale.
type RerunOutput struct {
	Body struct {
		DryRun bool      `json:"dryRun"`
		Rerun  []TaskDTO `json:"rerun" doc:"Tasks that will run again"`
		Stale  []TaskDTO `json:"stale" doc:"Tasks that will be flagged stale rather than run"`
	}
}

// StaleActionInput names stale tasks to run or accept. An empty list means all
// of them, which is the ordinary case.
type StaleActionInput struct {
	Key  string `path:"key" doc:"Video ref or id"`
	Body struct {
		TaskIDs []string `json:"taskIds,omitempty" doc:"Stale tasks to act on; empty means every stale task"`
	}
}

// StaleActionOutput reports how many tasks the action touched.
type StaleActionOutput struct {
	Body struct {
		Count int `json:"count"`
	}
}

func postRerun(
	videos repository.VideoReader,
	tasks repository.TaskReader,
	rerunner app.TaskRerunner,
	prompts app.PromptCacheInvalidator,
) func(context.Context, *RerunInput) (*RerunOutput, error) {
	return func(ctx context.Context, in *RerunInput) (*RerunOutput, error) {
		v, err := app.GetVideo(ctx, videos, in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		plan, err := app.RerunTasks(ctx, tasks, rerunner, prompts, v.ID,
			taskIDs(in.Body.TaskIDs), in.Body.DryRun)
		if err != nil {
			return nil, mapError(err)
		}
		out := &RerunOutput{}
		out.Body.DryRun = in.Body.DryRun
		out.Body.Rerun = taskDTOs(plan.Rerun)
		out.Body.Stale = taskDTOs(plan.Stale)
		return out, nil
	}
}

func postRunStale(
	videos repository.VideoReader,
	runner app.StaleRunner,
	prompts app.PromptCacheInvalidator,
) func(context.Context, *StaleActionInput) (*StaleActionOutput, error) {
	return func(ctx context.Context, in *StaleActionInput) (*StaleActionOutput, error) {
		v, err := app.GetVideo(ctx, videos, in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		n, err := app.RunStaleTasks(ctx, runner, prompts, v.ID, taskIDs(in.Body.TaskIDs))
		if err != nil {
			return nil, mapError(err)
		}
		out := &StaleActionOutput{}
		out.Body.Count = n
		return out, nil
	}
}

func postAcceptStale(
	videos repository.VideoReader,
	accepter app.StaleAccepter,
) func(context.Context, *StaleActionInput) (*StaleActionOutput, error) {
	return func(ctx context.Context, in *StaleActionInput) (*StaleActionOutput, error) {
		v, err := app.GetVideo(ctx, videos, in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		n, err := app.AcceptStaleTasks(ctx, accepter, v.ID, taskIDs(in.Body.TaskIDs))
		if err != nil {
			return nil, mapError(err)
		}
		out := &StaleActionOutput{}
		out.Body.Count = n
		return out, nil
	}
}

func taskIDs(ids []string) []entity.TaskID {
	if len(ids) == 0 {
		return nil
	}
	out := make([]entity.TaskID, 0, len(ids))
	for _, id := range ids {
		out = append(out, entity.TaskID(id))
	}
	return out
}

func taskDTOs(rows []entity.Task) []TaskDTO {
	out := make([]TaskDTO, 0, len(rows))
	for _, t := range rows {
		out = append(out, taskFrom(t))
	}
	return out
}

func tasksOutput(rows []entity.Task) *TasksOutput {
	out := &TasksOutput{}
	out.Body.Tasks = make([]TaskDTO, 0, len(rows))
	for _, t := range rows {
		out.Body.Tasks = append(out.Body.Tasks, taskFrom(t))
	}
	return out
}

func registerTaskRoutes(
	api huma.API,
	videos repository.VideoReader,
	tasks repository.TaskReader,
	retrier app.TaskRetrier,
	prompts app.PromptCacheInvalidator,
	rerunner app.TaskRerunner,
	staleRunner app.StaleRunner,
	staleAccepter app.StaleAccepter,
) {
	huma.Register(api, huma.Operation{
		OperationID: "listVideoTasks", Method: "GET", Path: "/api/videos/{key}/tasks",
		Summary: "List a video's whole DAG", Tags: []string{"tasks"},
	}, getVideoTasks(videos, tasks))

	huma.Register(api, huma.Operation{
		OperationID: "listRecentTasks", Method: "GET", Path: "/api/tasks",
		Summary: "List recently updated tasks", Tags: []string{"tasks"},
	}, getRecentTasks(tasks))

	huma.Register(api, huma.Operation{
		OperationID: "retryTask", Method: "POST", Path: "/api/tasks/{id}/retry",
		Summary: "Retry a failed task and everything downstream",
		Description: "For a task that failed. Everything below it is blocked rather than " +
			"done, so it cascades and runs immediately. To redo a task that succeeded, " +
			"use the re-run endpoint instead.",
		Tags: []string{"tasks"},
	}, postTaskRetry(tasks, retrier, prompts))

	huma.Register(api, huma.Operation{
		OperationID: "rerunTasks", Method: "POST", Path: "/api/videos/{key}/rerun",
		Summary: "Re-run succeeded tasks, flagging their downstream stale",
		Description: "Re-runs the named tasks and marks everything downstream of them " +
			"stale instead of re-running it, so artifacts that may already have been " +
			"reviewed are not discarded without a decision. Send dryRun to see what " +
			"would be affected without changing anything.",
		Tags: []string{"tasks"},
	}, postRerun(videos, tasks, rerunner, prompts))

	huma.Register(api, huma.Operation{
		OperationID: "runStaleTasks", Method: "POST", Path: "/api/videos/{key}/stale/run",
		Summary: "Re-run stale tasks", Tags: []string{"tasks"},
	}, postRunStale(videos, staleRunner, prompts))

	huma.Register(api, huma.Operation{
		OperationID: "acceptStaleTasks", Method: "POST", Path: "/api/videos/{key}/stale/accept",
		Summary: "Keep stale artifacts as they are",
		Description: "Clears the stale flag without re-running anything. Staleness records " +
			"that an input changed, not that the output is wrong; an operator who has " +
			"checked the artifact can keep it.",
		Tags: []string{"tasks"},
	}, postAcceptStale(videos, staleAccepter))
}
