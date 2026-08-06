package sample

import (
	"bytes"
	"context"
	"fmt"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// Slide serves slides from the sample set, rotated across chapters.
type Slide struct {
	lib   *Library
	store provider.AssetStore
	png   pngCache
}

var _ provider.SlideGenerator = (*Slide)(nil)

// NewSlide wires the backend to the shared library.
func NewSlide(lib *Library, store provider.AssetStore) *Slide {
	return &Slide{lib: lib, store: store}
}

// Generate stores one slide and returns its content address.
//
// The file is chosen by ordinal and index together, so a chapter always gets
// distinct slides — a dissolve between two copies of one image is not a
// dissolve — and consecutive chapters start at different points in the set
// rather than repeating the same pair down the whole video.
func (i *Slide) Generate(ctx context.Context, req provider.SlideRequest) (entity.AssetID, error) {
	if err := i.lib.Check(); err != nil {
		return "", err
	}
	path := i.lib.slides[(req.Ordinal+req.Index)%len(i.lib.slides)]

	// Slides go in at their native size: the composer builds the slideshow from
	// them and is the one that decides how they are framed.
	encoded, err := i.png.bytes(path, path, nil)
	if err != nil {
		return "", err
	}
	stored, err := i.store.Put(ctx, entity.AssetKindImage, bytes.NewReader(encoded))
	if err != nil {
		return "", fmt.Errorf("store slide: %w", err)
	}
	return stored.ID, nil
}
