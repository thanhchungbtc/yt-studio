package provider

import (
	"context"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
)

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
