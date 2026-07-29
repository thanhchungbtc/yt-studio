package repository

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
)

// AssetReader reads asset metadata by content address.
type AssetReader interface {
	AssetByID(ctx context.Context, id entity.AssetID) (entity.Asset, error)
	ListAssetsByVideo(ctx context.Context, videoID entity.VideoID) ([]entity.Asset, error)
}

// AssetWriter records asset metadata. Put is an upsert by content address:
// re-running a task that produces identical bytes is a no-op (§3).
type AssetWriter interface {
	PutAsset(ctx context.Context, a entity.Asset) error
}
