package app

import (
	"context"
	"fmt"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// PushVideoThumbnail sends a published video's thumbnail to the platform again.
//
// The same image the upload would send — the operator's override if there is
// one, otherwise what the renderer produced — so pushing and publishing cannot
// disagree about which picture fronts the video.
//
// Nothing is re-rendered and no video bytes move. Run it twice and the second
// run sends the same image, which is why, unlike PublishVideo, there is no
// guard against having run already.
func PushVideoThumbnail(
	ctx context.Context,
	videos repository.VideoReader,
	channels repository.ChannelReader,
	uploader provider.Uploader,
	videoID entity.VideoID,
	dryRun bool,
) (entity.Video, error) {
	video, err := videos.VideoByID(ctx, videoID)
	if err != nil {
		return entity.Video{}, err
	}
	asset := video.EffectiveThumbnailAssetID()
	if asset == "" {
		return entity.Video{}, fmt.Errorf("%w: %s has no thumbnail", ErrValidation, video.Ref)
	}
	// A dry run's receipt names no video, so there is nothing out there to
	// re-front.
	if video.Upload == nil || video.Upload.DryRun {
		return entity.Video{}, fmt.Errorf("%w: %s is not published", ErrValidation, video.Ref)
	}
	channel, err := channels.ChannelByID(ctx, video.ChannelID)
	if err != nil {
		return entity.Video{}, err
	}
	if channel.Credentials != entity.CredentialStatusValid {
		return entity.Video{}, fmt.Errorf("%w: channel %s has %s credentials",
			ErrValidation, channel.Slug, channel.Credentials)
	}

	if err := uploader.UpdateThumbnail(ctx, provider.ThumbnailPushRequest{
		VideoRef:    video.Ref,
		ChannelSlug: channel.Slug,
		PublishedID: video.Upload.VideoID,
		AssetID:     asset,
		DryRun:      dryRun,
	}); err != nil {
		return entity.Video{}, fmt.Errorf("push thumbnail for %s: %w", video.Ref, err)
	}
	return video, nil
}
