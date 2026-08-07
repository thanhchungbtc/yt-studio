package sample

import (
	"context"
	"fmt"
	"os"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// Composer answers both composition calls with the same sample file, cutting
// and encoding nothing. It exists for everything downstream — the player, the
// upload path, the sweep — without ffmpeg installed. Fifty chapters therefore
// cost one row and one file; ffmpeg is what to select when cuts, timing or
// burnt-in text need to be real.
type Composer struct {
	lib   *Library
	store provider.AssetStore
}

var _ provider.VideoComposer = (*Composer)(nil)

// NewComposer wires the backend to the shared library.
func NewComposer(lib *Library, store provider.AssetStore) *Composer {
	return &Composer{lib: lib, store: store}
}

// Clip stores the sample render as one chapter's clip.
func (c *Composer) Clip(ctx context.Context, _ provider.ClipRequest) (entity.AssetID, error) {
	return c.put(ctx, entity.AssetKindClip)
}

// Concat stores the sample render as the finished video.
func (c *Composer) Concat(ctx context.Context, _ provider.ConcatRequest) (entity.AssetID, error) {
	return c.put(ctx, entity.AssetKindFinal)
}

// put streams the sample into the store under the kind asked for. Streamed
// rather than read, so memory stays flat however long the take is.
func (c *Composer) put(ctx context.Context, kind entity.AssetKind) (entity.AssetID, error) {
	path, err := c.lib.Video()
	if err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrUnavailable, path, err)
	}
	defer func() { _ = file.Close() }()

	stored, err := c.store.Put(ctx, kind, file)
	if err != nil {
		return "", fmt.Errorf("store %s: %w", kind, err)
	}
	return stored.ID, nil
}
