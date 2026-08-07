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

// AssetWriter records asset metadata. PutAsset upserts by content address and
// video, so a second video producing the same bytes records its own ownership.
type AssetWriter interface {
	PutAsset(ctx context.Context, a entity.Asset) error
}

// AssetAddress is one content address the database still knows about.
type AssetAddress struct {
	ID   entity.AssetID
	Kind entity.AssetKind
}

// AssetOwnerRef is a reference from a video or chapter to a content address,
// read from the data that names it rather than from the assets table.
type AssetOwnerRef struct {
	VideoID   entity.VideoID
	ChapterID *entity.ChapterID
	AssetID   entity.AssetID
	Kind      entity.AssetKind
}

// AssetMaintainer backs the two commands that reason about the store as a
// whole. Neither is on a request path: the repair runs at startup, the sweep
// from the CLI.
type AssetMaintainer interface {
	// MissingAssetOwners lists references no asset row records — the repair's
	// whole input, normally empty.
	MissingAssetOwners(ctx context.Context) ([]AssetOwnerRef, error)
	// AssetAddresses lists every address with an owner, which is what the sweep
	// compares the files on disk against.
	AssetAddresses(ctx context.Context) ([]AssetAddress, error)
}
