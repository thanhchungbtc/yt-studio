package http

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/domain/repository"
)

// RegenerateIconInput is an operator's edited cell prompt and the instruction
// to redraw that cell. One request, like the slide it mirrors: a prompt cannot
// be saved without generating from it.
type RegenerateIconInput struct {
	Key   string `path:"key" doc:"Video ref or id"`
	Index int    `path:"index" minimum:"0" doc:"0-based cell index in the thumbnail grid"`
	Body  struct {
		Prompt string `json:"prompt" required:"true" minLength:"1"`
	}
}

func postRegenerateIcon(
	videos repository.VideoReader,
	fields repository.VideoFieldWriter,
	tasks repository.TaskReader,
	rerunner app.TaskRerunner,
) func(context.Context, *RegenerateIconInput) (*VideoOutput, error) {
	return func(ctx context.Context, in *RegenerateIconInput) (*VideoOutput, error) {
		v, err := app.GetVideo(ctx, videos, in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		v, err = app.RegenerateThumbnailIcon(ctx, videos, fields, rerunner, v.ID, in.Index, in.Body.Prompt)
		if err != nil {
			return nil, mapError(err)
		}
		counts, err := tasks.CountTasksByVideo(ctx, v.ID)
		if err != nil {
			return nil, mapError(err)
		}
		return &VideoOutput{Body: videoFrom(v, counts)}, nil
	}
}

// registerThumbnailRoutes is its own function rather than more parameters on
// registerVideoRoutes, which is already at the argument limit.
func registerThumbnailRoutes(
	api huma.API,
	videos repository.VideoReader,
	fields repository.VideoFieldWriter,
	tasks repository.TaskReader,
	rerunner app.TaskRerunner,
) {
	huma.Register(api, huma.Operation{
		OperationID: "regenerateThumbnailIcon", Method: "POST",
		Path:    "/api/videos/{key}/thumbnail/cells/{index}/generate",
		Summary: "Redraw one thumbnail cell from an edited prompt",
		Description: "Writes the prompt of this grid cell and re-runs that one icon task " +
			"with it. The cell's caption is left alone, and so is the shared style clause, " +
			"which is settings-sourced and appended at generation. The composed thumbnail " +
			"below it keeps its artifact and is flagged stale — and since the upload gate " +
			"rides on that thumbnail, redrawing a cell reopens the publish decision.",
		Tags: []string{"videos"},
	}, postRegenerateIcon(videos, fields, tasks, rerunner))
}
