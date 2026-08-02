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

// UpdateScriptInput is an operator's inline script edit.
type UpdateScriptInput struct {
	ID   string `path:"id" doc:"Chapter id"`
	Body struct {
		Script string `json:"script" required:"true" minLength:"1"`
	}
}

// RegenerateStillInput is an operator's edited prompt and the instruction to
// draw the still again with it. The two are one request because they are one
// decision; there is no way to save a prompt without generating from it.
type RegenerateStillInput struct {
	ID    string `path:"id" doc:"Chapter id"`
	Index int    `path:"index" minimum:"0" doc:"0-based still index within the chapter"`
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

func postRegenerateStill(
	chapters repository.ChapterReader,
	fields repository.ChapterFieldWriter,
	rerunner app.TaskRerunner,
	notifier app.ChapterNotifier,
) func(context.Context, *RegenerateStillInput) (*ChapterOutput, error) {
	return func(ctx context.Context, in *RegenerateStillInput) (*ChapterOutput, error) {
		c, err := app.RegenerateChapterStill(ctx, chapters, fields, rerunner, notifier,
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
		OperationID: "updateChapterScript", Method: "PUT", Path: "/api/chapters/{id}/script",
		Summary: "Edit a chapter's script", Tags: []string{"chapters"},
	}, putChapterScript(chapters, fields, notifier, marker))

	huma.Register(api, huma.Operation{
		OperationID: "regenerateChapterStill", Method: "POST",
		Path:    "/api/chapters/{id}/stills/{index}/generate",
		Summary: "Redraw one still from an edited prompt",
		Description: "Writes the prompt at this index and re-runs that one image task with " +
			"it. Everything downstream keeps its artifact and is flagged stale, exactly as " +
			"re-running the still from the task table would. There is no way to save a " +
			"prompt without generating from it: the stored prompt is always the one the " +
			"current still was drawn from.",
		Tags: []string{"chapters"},
	}, postRegenerateStill(chapters, fields, rerunner, notifier))

	huma.Register(api, huma.Operation{
		OperationID: "retryChapter", Method: "POST", Path: "/api/videos/{key}/chapters/{ordinal}/retry",
		Summary: "Retry a failed chapter and everything downstream", Tags: []string{"chapters"}, DefaultStatus: 204,
	}, postChapterRetry(videos, retrier, prompts))
}
