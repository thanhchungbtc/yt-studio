package sample

import (
	"context"
	"fmt"
	"os"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// Composer answers both composition calls with the sample render.
//
// It cuts nothing and encodes nothing: a chapter's clip and the whole video's
// final render are the same file on disk, handed back unchanged. What it exists
// for is everything downstream of composition — the player in the UI, the
// upload path, the sweep, a final asset that is a real MP4 — without ffmpeg
// installed and without the minutes a real encode of fifty chapters costs.
//
// The consequence to know: every clip shares one content address, so fifty
// chapters produce one row and one file. Nothing keys off a clip being unique,
// and the chapter's own clip_asset_id is written per chapter either way. If you
// need to see cuts, timing or burnt-in text, that is what ffmpeg is for.
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

// put streams the sample into the store under the kind the caller asked for.
//
// Streamed rather than read: it is tens of megabytes per call, and the store
// hashes and copies with a pooled buffer, so memory stays flat however long the
// take is.
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
