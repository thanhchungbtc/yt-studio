package app

import (
	"context"
	"io"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// OpenedAsset is a stored file ready to be streamed to a client.
type OpenedAsset struct {
	Asset  entity.Asset
	Reader io.ReadSeekCloser
}

// OpenAsset resolves an asset by content address and opens it for streaming.
//
// The caller serves it with immutable cache headers: the hash is the cache key,
// so a still caches forever for free.
func OpenAsset(
	ctx context.Context,
	assets repository.AssetReader,
	store provider.AssetStore,
	id entity.AssetID,
) (OpenedAsset, error) {
	a, err := assets.AssetByID(ctx, id)
	if err != nil {
		return OpenedAsset{}, err
	}
	r, err := store.Open(ctx, a.ID, a.Kind)
	if err != nil {
		return OpenedAsset{}, err
	}
	return OpenedAsset{Asset: a, Reader: r}, nil
}

// ListAssets returns every artifact a video produced.
func ListAssets(
	ctx context.Context,
	assets repository.AssetReader,
	videoID entity.VideoID,
) ([]entity.Asset, error) {
	return assets.ListAssetsByVideo(ctx, videoID)
}
