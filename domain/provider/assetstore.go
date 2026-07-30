package provider

import (
	"context"
	"io"
	"time"

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
	// Delete unlinks a file whose last owner is gone. It is idempotent: an
	// address that is not stored is already in the desired state, because the
	// caller unlinks only after the database has committed and may be retrying a
	// half-finished reclaim.
	Delete(ctx context.Context, id entity.AssetID, kind entity.AssetKind) error
}

// StoredFile is one file found in the store by a walk.
type StoredFile struct {
	// Rel is the path relative to the store root, always set.
	Rel string
	// ID and Kind are set only when Rel is a well-formed content address. A file
	// the layout does not explain — a crashed write's temporary file, something a
	// person copied in — arrives with them empty and is never deleted on the
	// strength of a database lookup, because no lookup describes it.
	ID   entity.AssetID
	Kind entity.AssetKind
	Size int64
	// Temporary marks a file in the store's staging area.
	Temporary bool
	ModTime   time.Time
}

// AssetSweeper is the store seen by the sweep: the one consumer that has to ask
// what is on disk rather than address a file whose hash it already holds.
//
// It is separate from AssetStore because nothing on a request path may enumerate
// the store, and because keeping it separate lets the sweep work in paths — the
// only way to reach a file that is not a valid content address.
type AssetSweeper interface {
	Walk(ctx context.Context, fn func(StoredFile) error) error
	// Remove unlinks one path relative to the store root. A missing file is
	// success, for the same reason it is in Delete.
	Remove(ctx context.Context, rel string) error
	// PruneEmptyDirs removes the directories left holding nothing once files have
	// gone, and reports how many. It is the last step of a sweep rather than part
	// of removing a file, because a shard directory is shared by every address
	// with the same digest prefix.
	PruneEmptyDirs(ctx context.Context) (int, error)
}
