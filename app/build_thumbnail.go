package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// BuildThumbnail renders the image that fronts the finished video. It runs
// after the metadata because the headline is part of the listing, and it
// carries the upload gate, so what the operator signs off on is the whole
// listing rather than its text alone.
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

// thumbnailCells pairs each planned caption with the icon in its slot. Every
// icon task is a dependency, so an empty slot means one succeeded without
// recording anything; retrying would read the same slot, so this fails
// permanently and waits for the icon to be re-run.
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
