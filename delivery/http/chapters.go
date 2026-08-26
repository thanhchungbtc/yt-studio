package http

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// ChaptersOutput is the chapter list response.
type ChaptersOutput struct {
	Body struct {
		Chapters []ChapterDTO `json:"chapters"`
	}
}

// ChapterOutput is a single chapter response.
type ChapterOutput struct {
	Body ChapterDTO
}

// UpdatePlanInput is an operator's edit to the blueprint's plan for one chapter.
// All three fields are sent every time: they are edited as one row, and a PUT
// that took whichever of them changed would need a rule for telling an omitted
// summary from a cleared one.
type UpdatePlanInput struct {
	ID   string `path:"id" doc:"Chapter id"`
	Body struct {
		Title          string `json:"title" required:"true" minLength:"1" doc:"What the chapter covers"`
		Summary        string `json:"summary" doc:"The chapter's brief, as the script is written from it"`
		EstimatedWords int    `json:"estimatedWords" minimum:"0" doc:"Spoken-word budget; 0 leaves it unset"`
	}
}

// UpdateScriptInput is an operator's inline script edit.
type UpdateScriptInput struct {
	ID   string `path:"id" doc:"Chapter id"`
	Body struct {
		Script string `json:"script" required:"true" minLength:"1"`
	}
}

// RegenerateSlideInput is an edited prompt and the instruction to draw with it.
// One request, because there is no way to save a prompt without generating.
type RegenerateSlideInput struct {
	ID    string `path:"id" doc:"Chapter id"`
	Index int    `path:"index" minimum:"0" doc:"0-based slide index within the chapter"`
	Body  struct {
		Prompt string `json:"prompt" required:"true" minLength:"1"`
	}
}

// RetryChapterInput identifies a chapter to re-run.
type RetryChapterInput struct {
	Key     string `path:"key" doc:"Video ref or id"`
	Ordinal int    `path:"ordinal" minimum:"1" doc:"1-based chapter ordinal"`
}

func getChapters(videos repository.VideoReader, chapters repository.ChapterReader) func(context.Context, *VideoKeyInput) (*ChaptersOutput, error) {
	return func(ctx context.Context, in *VideoKeyInput) (*ChaptersOutput, error) {
		v, err := app.GetVideo(ctx, videos, in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		rows, err := app.ListChapters(ctx, chapters, v.ID)
		if err != nil {
			return nil, mapError(err)
		}
		out := &ChaptersOutput{}
		out.Body.Chapters = make([]ChapterDTO, 0, len(rows))
		for _, c := range rows {
			out.Body.Chapters = append(out.Body.Chapters, chapterFrom(c))
		}
		return out, nil
	}
}

func putChapterPlan(
	chapters repository.ChapterReader,
	fields repository.ChapterFieldWriter,
	notifier app.ChapterNotifier,
) func(context.Context, *UpdatePlanInput) (*ChapterOutput, error) {
	return func(ctx context.Context, in *UpdatePlanInput) (*ChapterOutput, error) {
		c, err := app.UpdateChapterPlan(ctx, chapters, fields, notifier,
			entity.ChapterID(in.ID), app.ChapterPlan{
				Title:          in.Body.Title,
				Summary:        in.Body.Summary,
				EstimatedWords: in.Body.EstimatedWords,
			})
		if err != nil {
			return nil, mapError(err)
		}
		return &ChapterOutput{Body: chapterFrom(c)}, nil
	}
}

func putChapterScript(
	chapters repository.ChapterReader,
	fields repository.ChapterFieldWriter,
	notifier app.ChapterNotifier,
	marker app.StaleMarker,
) func(context.Context, *UpdateScriptInput) (*ChapterOutput, error) {
	return func(ctx context.Context, in *UpdateScriptInput) (*ChapterOutput, error) {
		c, err := app.UpdateChapterScript(ctx, chapters, fields, notifier, marker,
			entity.ChapterID(in.ID), in.Body.Script)
		if err != nil {
			return nil, mapError(err)
		}
		return &ChapterOutput{Body: chapterFrom(c)}, nil
	}
}

func postRegenerateSlide(
	chapters repository.ChapterReader,
	fields repository.ChapterFieldWriter,
	rerunner app.TaskRerunner,
	notifier app.ChapterNotifier,
) func(context.Context, *RegenerateSlideInput) (*ChapterOutput, error) {
	return func(ctx context.Context, in *RegenerateSlideInput) (*ChapterOutput, error) {
		c, err := app.RegenerateChapterSlide(ctx, chapters, fields, rerunner, notifier,
			entity.ChapterID(in.ID), in.Index, in.Body.Prompt)
		if err != nil {
			return nil, mapError(err)
		}
		return &ChapterOutput{Body: chapterFrom(c)}, nil
	}
}

func postChapterRetry(
	videos repository.VideoReader,
	retrier app.ChapterRetrier,
	prompts app.PromptCacheInvalidator,
) func(context.Context, *RetryChapterInput) (*struct{}, error) {
	return func(ctx context.Context, in *RetryChapterInput) (*struct{}, error) {
		v, err := app.GetVideo(ctx, videos, in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		if err := app.RetryChapter(ctx, retrier, prompts, v.ID, in.Ordinal); err != nil {
			return nil, mapError(err)
		}
		return nil, nil //nolint:nilnil // huma models 204 as a nil body
	}
}

func registerChapterRoutes(
	api huma.API,
	videos repository.VideoReader,
	chapters repository.ChapterReader,
	fields repository.ChapterFieldWriter,
	notifier app.ChapterNotifier,
	retrier app.ChapterRetrier,
	prompts app.PromptCacheInvalidator,
	marker app.StaleMarker,
	rerunner app.TaskRerunner,
) {
	huma.Register(api, huma.Operation{
		OperationID: "listChapters", Method: "GET", Path: "/api/videos/{key}/chapters",
		Summary: "List a video's chapters", Tags: []string{"chapters"},
	}, getChapters(videos, chapters))

	huma.Register(api, huma.Operation{
		OperationID: "updateChapterPlan", Method: "PUT", Path: "/api/chapters/{id}/plan",
		Summary: "Edit a chapter's title, brief and word budget",
		Description: "Writes the plan and stops there. Nothing is re-run and nothing is " +
			"flagged: a chapter whose script has not been generated yet is written from " +
			"this, because every task reads the plan from the chapter rather than from the " +
			"stored blueprint asset. A chapter that already has a script keeps it until " +
			"the operator retries that chapter.",
		Tags: []string{"chapters"},
	}, putChapterPlan(chapters, fields, notifier))

	huma.Register(api, huma.Operation{
		OperationID: "updateChapterScript", Method: "PUT", Path: "/api/chapters/{id}/script",
		Summary: "Edit a chapter's script", Tags: []string{"chapters"},
	}, putChapterScript(chapters, fields, notifier, marker))

	huma.Register(api, huma.Operation{
		OperationID: "regenerateChapterSlide", Method: "POST",
		Path:    "/api/chapters/{id}/slides/{index}/generate",
		Summary: "Redraw one slide from an edited prompt",
		Description: "Writes the prompt at this index and re-runs that one slide task with " +
			"it. Everything downstream keeps its artifact and is flagged stale, exactly as " +
			"re-running the slide from the task table would. There is no way to save a " +
			"prompt without generating from it: the stored prompt is always the one the " +
			"current slide was drawn from.",
		Tags: []string{"chapters"},
	}, postRegenerateSlide(chapters, fields, rerunner, notifier))

	huma.Register(api, huma.Operation{
		OperationID: "retryChapter", Method: "POST", Path: "/api/videos/{key}/chapters/{ordinal}/retry",
		Summary: "Retry a failed chapter and everything downstream", Tags: []string{"chapters"}, DefaultStatus: 204,
	}, postChapterRetry(videos, retrier, prompts))
}
