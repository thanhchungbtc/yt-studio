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
		Summary: "Re-run a task and everything downstream", Tags: []string{"tasks"},
	}, postTaskRetry(tasks, retrier, prompts))
}
