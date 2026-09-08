package http

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
	"github.com/tbui/yt-studio/domain/service"
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

// PushMetadataInput names the video whose listing is to be sent again.
type PushMetadataInput struct {
	Key string `path:"key" doc:"Video ref or id"`
}

// postVideoMetadataPush sends a published video's listing to YouTube again.
func postVideoMetadataPush(
	videos repository.VideoReader,
	channels repository.ChannelReader,
	tasks repository.TaskReader,
	uploader provider.Uploader,
	settings *service.Settings,
) func(context.Context, *PushMetadataInput) (*VideoOutput, error) {
	return func(ctx context.Context, in *PushMetadataInput) (*VideoOutput, error) {
		v, err := app.GetVideo(ctx, videos, in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		// The same row the publish path reads. A dry run that publishes nothing
		// and a dry run that corrects nothing are the same rehearsal.
		dry := settings.Bool(entity.SettingUploadDryRun)
		v, err = app.PushVideoMetadata(ctx, videos, channels, uploader, v.ID, dry)
		if err != nil {
			return nil, mapError(err)
		}
		return videoOutput(ctx, tasks, v)
	}
}

// registerMetadataRoutes is its own function for the reason the thumbnail
// routes are: registerVideoRoutes is already at the argument limit.
//
//nolint:revive // the parameter list is the dependency list
func registerMetadataRoutes(
	api huma.API,
	videos repository.VideoReader,
	fields repository.VideoFieldWriter,
	tasks repository.TaskReader,
	channels repository.ChannelReader,
	uploader provider.Uploader,
	settings *service.Settings,
) {
	huma.Register(api, huma.Operation{
		OperationID: "saveVideoMetadata", Method: "PUT",
		Path:    "/api/videos/{key}/metadata",
		Summary: "Replace a video's YouTube listing",
		Description: "Takes the whole listing back, not the changed fields. Runs nothing: " +
			"the upload reads this row when it runs, so an edited title is what publishes.",
		Tags: []string{"videos"},
	}, putVideoMetadata(videos, fields, tasks))

	huma.Register(api, huma.Operation{
		OperationID: "pushVideoMetadata", Method: "POST",
		Path:    "/api/videos/{key}/metadata/push",
		Summary: "Send a published video's listing to YouTube again",
		Description: "For a video already published, whose listing has been edited since. " +
			"Sends no bytes and re-renders nothing; only the title, description, tags and " +
			"category are rewritten, and every other field of the listing is left as YouTube " +
			"holds it. Refused for a video that was never published.",
		Tags: []string{"videos"},
	}, postVideoMetadataPush(videos, channels, tasks, uploader, settings))
}
