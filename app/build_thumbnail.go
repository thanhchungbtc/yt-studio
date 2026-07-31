package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// BuildThumbnail renders the image that fronts the finished video.
//
// It runs after the metadata because the hook it overlays is part of the
// listing: the thumbnail text and the title compete for the same glance, so
// they are written together and only then rendered.
//
// When the upload gate is enabled this task carries it: on success the
// scheduler parks in awaiting_approval and does not release the upload, so what
// the operator signs off on is the listing they are about to publish, thumbnail
// included.
//
//nolint:revive // the parameter list is the dependency list
func BuildThumbnail(
	ctx context.Context,
	t entity.Task,
	videos repository.VideoReader,
	chapters repository.ChapterReader,
	thumbnails provider.ThumbnailBuilder,
	assets repository.AssetWriter,
	store provider.AssetStore,
	now time.Time,
) entity.TaskOutcome {
	video, err := videos.VideoByID(ctx, t.VideoID)
	if err != nil {
		return classify(err)
	}
	if video.Metadata == nil {
		return entity.Failed{Err: fmt.Errorf("%w: video has no metadata", ErrValidation), Retryable: true}
	}

	rows, err := chapters.ListChaptersByVideo(ctx, video.ID)
	if err != nil {
		return classify(err)
	}
	// Which still fronts the video is settled here rather than in the backend.
	// The opening chapter's stills are a provisional answer: they exist by the
	// time this runs, and changing the rule is a change to this line.
	var candidates []entity.AssetID
	for _, c := range rows {
		if c.Ordinal == 1 {
			candidates = c.ImageAssetIDs
			break
		}
	}
	if len(candidates) == 0 {
		return entity.Failed{
			Err:       fmt.Errorf("%w: no still to build a thumbnail from", ErrValidation),
			Retryable: true,
		}
	}

	assetID, err := thumbnails.Build(ctx, provider.ThumbnailRequest{
		VideoID:       video.ID,
		VideoRef:      video.Ref,
		Title:         video.Metadata.Title,
		Text:          video.Metadata.ThumbnailText,
		ImageAssetIDs: candidates,
	})
	if err != nil {
		return classify(fmt.Errorf("build thumbnail for %s: %w", video.Ref, err))
	}

	if _, err := RecordAsset(ctx, assets, store, assetID, entity.AssetKindThumbnail,
		video.ID, nil, "thumbnail.build", now); err != nil {
		return classify(err)
	}
	return entity.Success{Assets: []entity.AssetID{assetID}}
}
