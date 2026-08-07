package app

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// SaveThumbnailDesign stores the browser editor's working document.
//
// The document is opaque here — the editor authored it and is the only thing
// that can render it — so this validates only what storing it requires. It
// touches no asset field, because the editor autosaves as the operator works
// and a half-finished draft must not become the published image.
func SaveThumbnailDesign(
	ctx context.Context,
	videos repository.VideoReader,
	fields repository.VideoFieldWriter,
	videoID entity.VideoID,
	design entity.ThumbnailDesign,
) (entity.Video, error) {
	if err := design.Validate(); err != nil {
		return entity.Video{}, err
	}
	v, err := videos.VideoByID(ctx, videoID)
	if err != nil {
		return entity.Video{}, err
	}
	if err := fields.SetVideoThumbnailDesign(ctx, videoID, design); err != nil {
		return entity.Video{}, err
	}
	v.ThumbnailDesign = design
	return v, nil
}
