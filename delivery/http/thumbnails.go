package http

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// RegenerateIconInput is an edited cell prompt and the instruction to redraw
// that cell. One request, like the slide it mirrors.
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
		return videoOutput(ctx, tasks, v)
	}
}

// SaveThumbnailDesignInput carries the browser editor's working document.
//
// The document is `any` on purpose: the editor owns its shape, so a struct here
// would be a second definition of it with no reader on this side. What the
// server guarantees is that it comes back out the way it went in.
type SaveThumbnailDesignInput struct {
	Key  string `path:"key" doc:"Video ref or id"`
	Body struct {
		Design any `json:"design" required:"true" doc:"Opaque editor document"`
	}
}

func putThumbnailDesign(
	videos repository.VideoReader,
	fields repository.VideoFieldWriter,
	tasks repository.TaskReader,
) func(context.Context, *SaveThumbnailDesignInput) (*VideoOutput, error) {
	return func(ctx context.Context, in *SaveThumbnailDesignInput) (*VideoOutput, error) {
		v, err := app.GetVideo(ctx, videos, in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		encoded, err := json.Marshal(in.Body.Design)
		if err != nil {
			return nil, mapError(app.Invalid("design", "must be JSON"))
		}
		v, err = app.SaveThumbnailDesign(ctx, videos, fields, v.ID, entity.ThumbnailDesign(encoded))
		if err != nil {
			return nil, mapError(err)
		}
		return videoOutput(ctx, tasks, v)
	}
}

// ApplyThumbnailOverrideInput is the finished image itself. The browser editor
// rasterises what it drew, so the request body is the picture and there is
// nothing here to render.
type ApplyThumbnailOverrideInput struct {
	Key string `path:"key" doc:"Video ref or id"`
	//nolint:lll // one field, one line
	RawBody []byte `contentType:"image/png" doc:"The thumbnail as PNG, 16:9 and at least 1280x720"`
}

//nolint:revive // the parameter list is the dependency list
func postThumbnailOverride(
	videos repository.VideoReader,
	fields repository.VideoFieldWriter,
	assets repository.AssetWriter,
	store provider.AssetStore,
	tasks repository.TaskReader,
	now func() time.Time,
) func(context.Context, *ApplyThumbnailOverrideInput) (*VideoOutput, error) {
	return func(ctx context.Context, in *ApplyThumbnailOverrideInput) (*VideoOutput, error) {
		v, err := app.GetVideo(ctx, videos, in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		v, err = app.ApplyThumbnailOverride(ctx, videos, fields, assets, store,
			v.ID, bytes.NewReader(in.RawBody), now())
		if err != nil {
			return nil, mapError(err)
		}
		return videoOutput(ctx, tasks, v)
	}
}

// ClearThumbnailOverrideInput reverts to the rendered thumbnail.
type ClearThumbnailOverrideInput struct {
	Key string `path:"key" doc:"Video ref or id"`
}

func deleteThumbnailOverride(
	videos repository.VideoReader,
	fields repository.VideoFieldWriter,
	tasks repository.TaskReader,
) func(context.Context, *ClearThumbnailOverrideInput) (*VideoOutput, error) {
	return func(ctx context.Context, in *ClearThumbnailOverrideInput) (*VideoOutput, error) {
		v, err := app.GetVideo(ctx, videos, in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		v, err = app.ClearThumbnailOverride(ctx, videos, fields, v.ID)
		if err != nil {
			return nil, mapError(err)
		}
		return videoOutput(ctx, tasks, v)
	}
}

// videoOutput is the tail every handler here shares: the counts the video panel
// needs, read after the write so the screen reflects it.
func videoOutput(ctx context.Context, tasks repository.TaskReader, v entity.Video) (*VideoOutput, error) {
	counts, err := tasks.CountTasksByVideo(ctx, v.ID)
	if err != nil {
		return nil, mapError(err)
	}
	return &VideoOutput{Body: videoFrom(v, counts)}, nil
}

// registerThumbnailRoutes is its own function rather than more parameters on
// registerVideoRoutes, which is already at the argument limit.
//
//nolint:revive // the parameter list is the dependency list
func registerThumbnailRoutes(
	api huma.API,
	videos repository.VideoReader,
	fields repository.VideoFieldWriter,
	assets repository.AssetWriter,
	store provider.AssetStore,
	tasks repository.TaskReader,
	rerunner app.TaskRerunner,
	now func() time.Time,
) {
	huma.Register(api, huma.Operation{
		OperationID: "saveThumbnailDesign", Method: "PUT",
		Path:    "/api/videos/{key}/thumbnail/design",
		Summary: "Save the thumbnail editor's working document",
		Description: "Stores the browser editor's document so reopening it restores what was " +
			"built. The document is opaque to the server. This changes nothing about which " +
			"image publishes -- the editor autosaves as it is edited, and a draft must not " +
			"become the listing.",
		Tags: []string{"videos"},
	}, putThumbnailDesign(videos, fields, tasks))

	huma.Register(api, huma.Operation{
		OperationID: "applyThumbnailOverride", Method: "POST",
		Path:    "/api/videos/{key}/thumbnail/override",
		Summary: "Publish a thumbnail built in the editor",
		Description: "Stores the image the editor produced and makes it the thumbnail this " +
			"video publishes with. The rendered thumbnail is left untouched on its own " +
			"field, so re-running the thumbnail task -- which redrawing any single icon " +
			"does -- cannot discard this, and reverting is always available.",
		// Raised from huma's 1 MB default: a photographic thumbnail encodes to
		// well over that, and the real ceiling is YouTube's, checked in the use
		// case so the operator gets that reason rather than a truncated read.
		MaxBodyBytes: entity.MaxThumbnailBytes + 4096,
		Tags:         []string{"videos"},
	}, postThumbnailOverride(videos, fields, assets, store, tasks, now))

	huma.Register(api, huma.Operation{
		OperationID: "clearThumbnailOverride", Method: "DELETE",
		Path:    "/api/videos/{key}/thumbnail/override",
		Summary: "Revert to the rendered thumbnail",
		Description: "Drops the hand-built thumbnail so the renderer's own image publishes " +
			"again. The editor document is kept, so the work can be reopened and re-applied.",
		Tags: []string{"videos"},
	}, deleteThumbnailOverride(videos, fields, tasks))

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
