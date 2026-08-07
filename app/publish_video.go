package app

import (
	"context"
	"fmt"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// PublishVideo uploads the final render. Dry run is the default and stays the
// default until a real backend is wired in.
func PublishVideo(
	ctx context.Context,
	t entity.Task,
	videos repository.VideoReader,
	channels repository.ChannelReader,
	uploader provider.Uploader,
	videoFields repository.VideoFieldWriter,
	dryRun func() bool,
) entity.TaskOutcome {
	video, err := videos.VideoByID(ctx, t.VideoID)
	if err != nil {
		return classify(err)
	}
	if video.FinalAssetID == nil || *video.FinalAssetID == "" {
		return entity.Failed{Err: fmt.Errorf("%w: video has no final render", ErrValidation), Retryable: true}
	}
	if video.Metadata == nil {
		return entity.Failed{Err: fmt.Errorf("%w: video has no metadata", ErrValidation), Retryable: true}
	}
	// The one the operator built if there is one, otherwise what the renderer
	// produced. The thumbnail is a dependency, so neither being present means
	// its task succeeded without recording anything. Better another attempt
	// than letting YouTube pick a frame.
	thumbnailAssetID := video.EffectiveThumbnailAssetID()
	if thumbnailAssetID == "" {
		return entity.Failed{Err: fmt.Errorf("%w: video has no thumbnail", ErrValidation), Retryable: true}
	}
	channel, err := channels.ChannelByID(ctx, video.ChannelID)
	if err != nil {
		return classify(err)
	}

	dry := true
	if dryRun != nil {
		dry = dryRun()
	}
	if !dry && channel.Credentials != entity.CredentialStatusValid {
		return entity.Failed{
			Err: fmt.Errorf("%w: channel %s has %s credentials", ErrValidation,
				channel.Slug, channel.Credentials),
			Retryable: false,
		}
	}

	record, err := uploader.Upload(ctx, provider.UploadRequest{
		VideoID:          video.ID,
		VideoRef:         video.Ref,
		ChannelSlug:      channel.Slug,
		FinalAssetID:     *video.FinalAssetID,
		ThumbnailAssetID: thumbnailAssetID,
		Metadata:         *video.Metadata,
		DryRun:           dry,
	})
	if err != nil {
		return classify(fmt.Errorf("upload %s: %w", video.Ref, err))
	}
	if err := videoFields.SetVideoUpload(ctx, video.ID, record); err != nil {
		return classify(err)
	}
	return entity.Success{}
}
