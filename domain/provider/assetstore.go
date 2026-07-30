package provider

import (
	"context"
	"io"

	"github.com/tbui/yt-studio/domain/entity"
)

// StoredAsset is what an AssetStore returns after ingesting a byte stream.
type StoredAsset struct {
	// ID is the sha256 of the bytes: the content address.
	ID entity.AssetID
	// Path is relative to the store root, so the store can be relocated.
	Path string
	Size int64
	// Existed reports that the content address was already present, which is what
	// makes a partial re-run cheap.
	Existed bool
}

// AssetStore is the content-addressed file store behind every generated file.
//
// Put streams: a multi-GB render is copied with a sized buffer and never read
// into memory.
type AssetStore interface {
	Put(ctx context.Context, kind entity.AssetKind, r io.Reader) (StoredAsset, error)
	Open(ctx context.Context, id entity.AssetID, kind entity.AssetKind) (io.ReadSeekCloser, error)
	// Stat describes a stored asset without opening it — enough to record its
	// metadata row and to answer a range request.
	Stat(ctx context.Context, id entity.AssetID, kind entity.AssetKind) (StoredAsset, error)
}
