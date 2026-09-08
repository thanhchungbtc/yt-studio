package http

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// SaveMetadataInput is the whole listing, in the shape it is read back in.
//
// The body is MetadataDTO itself rather than a subset of it, so the screen that
// wants to correct a title sends back what it was given with that one field
// changed. A partial body would have to be merged here, and a merge is where the
// question "which of these two rows is the listing" starts having two answers.
type SaveMetadataInput struct {
	Key  string `path:"key" doc:"Video ref or id"`
	Body MetadataDTO
}

// putVideoMetadata replaces a video's listing.
func putVideoMetadata(
	videos repository.VideoReader,
	fields repository.VideoFieldWriter,
	tasks repository.TaskReader,
) func(context.Context, *SaveMetadataInput) (*VideoOutput, error) {
	return func(ctx context.Context, in *SaveMetadataInput) (*VideoOutput, error) {
		v, err := app.GetVideo(ctx, videos, in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		v, err = app.UpdateVideoMetadata(ctx, videos, fields, v.ID, metadataFrom(in.Body))
		if err != nil {
			return nil, mapError(err)
		}
		return videoOutput(ctx, tasks, v)
	}
}

// metadataFrom is videoFrom's listing half, read the other way.
func metadataFrom(dto MetadataDTO) entity.Metadata {
	return entity.Metadata{
		Title:         dto.Title,
		Description:   dto.Description,
		Tags:          dto.Tags,
		ThumbnailText: dto.ThumbnailText,
		CategoryID:    dto.CategoryID,
		Privacy:       dto.Privacy,
	}
}

// registerMetadataRoutes is its own function for the reason the thumbnail
// routes are: registerVideoRoutes is already at the argument limit.
func registerMetadataRoutes(
	api huma.API,
	videos repository.VideoReader,
	fields repository.VideoFieldWriter,
	tasks repository.TaskReader,
) {
	huma.Register(api, huma.Operation{
		OperationID: "saveVideoMetadata", Method: "PUT",
		Path:    "/api/videos/{key}/metadata",
		Summary: "Replace a video's YouTube listing",
		Description: "Takes the whole listing back, not the changed fields. Runs nothing: " +
			"the upload reads this row when it runs, so an edited title is what publishes.",
		Tags: []string{"videos"},
	}, putVideoMetadata(videos, fields, tasks))
}
