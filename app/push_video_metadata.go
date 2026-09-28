package app

import (
	"context"
	"fmt"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// PushVideoMetadata sends a published video's listing to the platform again,
// so an edit made after publishing reaches the video it describes.
//
// UpdateVideoMetadata writes the row and says, correctly, that nothing is
// downstream of it but the upload. That stops being true the moment a video is
// published: the row and the platform can now disagree, and this is the only
// thing that settles it. Deliberately not called from there — editing a local
// row should not silently rewrite a live video, and the operator who wants both
// asks for both.
//
// No bytes move and nothing is re-rendered. Run it twice and the second run
// sends the same listing to the same video, which is why, unlike PublishVideo,
// there is no guard against having run already.
func PushVideoMetadata(
	ctx context.Context,
	videos repository.VideoReader,
	channels repository.ChannelReader,
	chapters repository.ChapterReader,
	uploader provider.Uploader,
	videoID entity.VideoID,
	dryRun bool,
) (entity.Video, error) {
	video, err := videos.VideoByID(ctx, videoID)
	if err != nil {
		return entity.Video{}, err
	}
	if video.Metadata == nil {
		return entity.Video{}, fmt.Errorf("%w: %s has no listing to send", ErrValidation, video.Ref)
	}
	// A dry run's receipt names no video, so there is nothing out there to
	// correct. Rehearsing the correction of a rehearsal is not a thing to allow
	// quietly -- it would report success against a video that does not exist.
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

	// The same composition the upload used. Without it a correction would send
	// the prose alone and strip the chapter links off the live video, which is
	// the one way this screen could quietly undo something it never mentioned.
	listing := *video.Metadata
	rows, err := chapters.ListChaptersByVideo(ctx, video.ID)
	if err != nil {
		return entity.Video{}, err
	}
	listing.Description = DescriptionWithChapters(
		listing.Description, video.ChapterOffsets, rows, entity.MaxDescriptionChars)

	if err := uploader.UpdateListing(ctx, provider.ListingRequest{
		VideoRef:    video.Ref,
		ChannelSlug: channel.Slug,
		PublishedID: video.Upload.VideoID,
		Metadata:    listing,
		DryRun:      dryRun,
	}); err != nil {
		return entity.Video{}, fmt.Errorf("push listing for %s: %w", video.Ref, err)
	}
	return video, nil
}
