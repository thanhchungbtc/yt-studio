package app

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"io"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// ApplyThumbnailOverride stores a thumbnail the operator built by hand and makes
// it the one that publishes.
//
// The image arrives finished: the browser editor both drew it and rasterised
// it, so there is nothing to render here and no second renderer to keep in step
// — what the operator saw is the bytes. The rendered thumbnail is deliberately
// left where it is, on its own field, so that re-running the thumbnail task
// (which redrawing any single icon does) cannot discard this, and so the
// operator can always see what the renderer would have produced and revert.
//
//nolint:revive // the parameter list is the dependency list
func ApplyThumbnailOverride(
	ctx context.Context,
	videos repository.VideoReader,
	fields repository.VideoFieldWriter,
	assets repository.AssetWriter,
	store provider.AssetStore,
	videoID entity.VideoID,
	body io.Reader,
	now time.Time,
) (entity.Video, error) {
	v, err := videos.VideoByID(ctx, videoID)
	if err != nil {
		return entity.Video{}, err
	}
	// Read into memory rather than streaming to the store: the bytes have to be
	// checked before they are kept, the ceiling is two megabytes, and a store
	// that accepts anything would content-address a rejected file into
	// existence for the sweep to find later.
	raw, err := readThumbnailBody(body)
	if err != nil {
		return entity.Video{}, err
	}
	if err := validateThumbnailImage(raw); err != nil {
		return entity.Video{}, err
	}

	stored, err := store.Put(ctx, entity.AssetKindThumbnail, bytes.NewReader(raw))
	if err != nil {
		return entity.Video{}, fmt.Errorf("store thumbnail override: %w", err)
	}
	if _, err := RecordAsset(ctx, assets, store, stored.ID, entity.AssetKindThumbnail,
		videoID, nil, "thumbnail.override", now); err != nil {
		return entity.Video{}, err
	}
	if err := fields.SetVideoThumbnailOverride(ctx, videoID, stored.ID); err != nil {
		return entity.Video{}, err
	}
	v.ThumbnailOverrideAssetID = &stored.ID
	return v, nil
}

// ClearThumbnailOverride reverts to the rendered thumbnail. The design document
// is kept: reverting is a decision about which image publishes, not an
// instruction to throw away the work behind the other one.
func ClearThumbnailOverride(
	ctx context.Context,
	videos repository.VideoReader,
	fields repository.VideoFieldWriter,
	videoID entity.VideoID,
) (entity.Video, error) {
	v, err := videos.VideoByID(ctx, videoID)
	if err != nil {
		return entity.Video{}, err
	}
	if err := fields.ClearVideoThumbnailOverride(ctx, videoID); err != nil {
		return entity.Video{}, err
	}
	v.ThumbnailOverrideAssetID = nil
	return v, nil
}

// readThumbnailBody reads the upload under a hard ceiling. The limit is read
// one byte over so an oversized body is rejected as too large rather than
// silently truncated into a corrupt PNG.
func readThumbnailBody(body io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(body, entity.MaxThumbnailBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read thumbnail upload: %w", err)
	}
	if len(raw) == 0 {
		return nil, Invalid("image", "must not be empty")
	}
	if len(raw) > entity.MaxThumbnailBytes {
		return nil, Invalid("image", fmt.Sprintf(
			"is over the %d byte limit YouTube accepts", entity.MaxThumbnailBytes))
	}
	return raw, nil
}

// validateThumbnailImage checks the upload is the picture it claims to be.
// PNG specifically, because entity.AssetKind pins the extension and the MIME
// type per kind — a JPEG stored here would be served as image/png.
func validateThumbnailImage(raw []byte) error {
	cfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return Invalid("image", "must be a PNG")
	}
	if cfg.Width < entity.ThumbnailWidth || cfg.Height < entity.ThumbnailHeight {
		return Invalid("image", fmt.Sprintf("must be at least %dx%d, got %dx%d",
			entity.ThumbnailWidth, entity.ThumbnailHeight, cfg.Width, cfg.Height))
	}
	// Anything else is letterboxed or cropped by YouTube, so it is better
	// refused here than discovered in the listing.
	if cfg.Width*entity.ThumbnailHeight != cfg.Height*entity.ThumbnailWidth {
		return Invalid("image", fmt.Sprintf("must be 16:9, got %dx%d", cfg.Width, cfg.Height))
	}
	return nil
}
