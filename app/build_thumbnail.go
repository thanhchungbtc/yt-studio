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
// It runs after the metadata because the headline it renders is part of the
// listing: the hook and the title compete for the same glance, so they are
// written together and only then drawn.
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
	thumbnails provider.ThumbnailRenderer,
	videoFields repository.VideoFieldWriter,
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
	cells, err := thumbnailCells(video)
	if err != nil {
		return classify(err)
	}

	assetID, err := thumbnails.Render(ctx, provider.ThumbnailRequest{
		VideoID:  video.ID,
		VideoRef: video.Ref,
		Title:    video.Metadata.Title,
		Headline: video.Metadata.ThumbnailText,
		Cells:    cells,
	})
	if err != nil {
		return classify(fmt.Errorf("build thumbnail for %s: %w", video.Ref, err))
	}

	if _, err := RecordAsset(ctx, assets, store, assetID, entity.AssetKindThumbnail,
		video.ID, nil, "thumbnail.build", now); err != nil {
		return classify(err)
	}
	if err := videoFields.SetVideoThumbnailAsset(ctx, video.ID, assetID); err != nil {
		return classify(err)
	}
	return entity.Success{Assets: []entity.AssetID{assetID}}
}

// thumbnailCells pairs each planned caption with the icon that landed in its
// slot.
//
// Every icon task is a dependency of this one, so an empty slot means one of
// them succeeded without recording anything. Another attempt here would read
// the same empty slot, so it fails permanently and waits: re-running the icon
// is what fixes it, and that marks this task stale and brings it back.
func thumbnailCells(video entity.Video) ([]provider.IconCell, error) {
	if video.ThumbnailPlan == nil {
		return nil, fmt.Errorf("%w: video has no thumbnail plan", ErrValidation)
	}
	planned := video.ThumbnailPlan.Cells
	if len(video.ThumbnailIconAssetIDs) < len(planned) {
		return nil, fmt.Errorf("%w: %d icons for %d cells",
			ErrValidation, len(video.ThumbnailIconAssetIDs), len(planned))
	}
	cells := make([]provider.IconCell, 0, len(planned))
	for i, c := range planned {
		if video.ThumbnailIconAssetIDs[i] == "" {
			return nil, fmt.Errorf("%w: cell %d has no icon", ErrValidation, i)
		}
		cells = append(cells, provider.IconCell{
			Caption:     c.Caption,
			IconAssetID: video.ThumbnailIconAssetIDs[i],
		})
	}
	return cells, nil
}
