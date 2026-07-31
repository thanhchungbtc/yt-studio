package sampleprovider

import (
	"bytes"
	"context"
	"fmt"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// Image serves stills from the sample set, rotated across chapters.
type Image struct {
	lib   *Library
	store provider.AssetStore
	png   pngCache
}

var _ provider.ImageProvider = (*Image)(nil)

// NewImage wires the backend to the shared library.
func NewImage(lib *Library, store provider.AssetStore) *Image {
	return &Image{lib: lib, store: store}
}

// Generate stores one still and returns its content address.
//
// The file is chosen by ordinal and index together, so a chapter always gets
// distinct stills — a dissolve between two copies of one image is not a
// dissolve — and consecutive chapters start at different points in the set
// rather than repeating the same pair down the whole video.
func (i *Image) Generate(ctx context.Context, req provider.ImageRequest) (entity.AssetID, error) {
	if err := i.lib.Check(); err != nil {
		return "", err
	}
	path := i.lib.images[(req.Ordinal+req.Index)%len(i.lib.images)]

	// Stills go in at their native size: the composer builds the slideshow from
	// them and is the one that decides how they are framed.
	encoded, err := i.png.bytes(path, path, nil)
	if err != nil {
		return "", err
	}
	stored, err := i.store.Put(ctx, entity.AssetKindImage, bytes.NewReader(encoded))
	if err != nil {
		return "", fmt.Errorf("store still: %w", err)
	}
	return stored.ID, nil
}
