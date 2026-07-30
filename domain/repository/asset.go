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

// AssetWriter records asset metadata. Put is an upsert by content address and
// video: re-running a task that produces identical bytes is a no-op, and a
// second video that produces the same bytes records its own ownership of them.
type AssetWriter interface {
	PutAsset(ctx context.Context, a entity.Asset) error
}

// AssetAddress is one content address the database still knows about.
type AssetAddress struct {
	ID   entity.AssetID
	Kind entity.AssetKind
}

// AssetOwnerRef is a reference from a video, or one of its chapters, to a
// content address — read from the data that names the address rather than from
// the assets table.
type AssetOwnerRef struct {
	VideoID   entity.VideoID
	ChapterID *entity.ChapterID
	AssetID   entity.AssetID
	Kind      entity.AssetKind
}

// AssetMaintainer backs the two maintenance commands that reason about the store
// as a whole rather than about one video's artifacts.
//
// Neither is on a request path: the repair runs once at startup and the sweep
// runs from the CLI.
type AssetMaintainer interface {
	// MissingAssetOwners lists references that no asset row records. It is the
	// repair's whole input, and it normally returns nothing.
	MissingAssetOwners(ctx context.Context) ([]AssetOwnerRef, error)
	// AssetAddresses lists every address with at least one owner, which is what
	// the sweep compares the files on disk against.
	AssetAddresses(ctx context.Context) ([]AssetAddress, error)
}
