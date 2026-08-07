package sample

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"strconv"

	xdraw "golang.org/x/image/draw"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// defaultIconSize is used when a caller asks for no particular size.
const defaultIconSize = 512

// Icon serves the thumbnail grid's tiles from the sample set, one file per
// cell.
type Icon struct {
	lib   *Library
	store provider.AssetStore
	png   pngCache
}

var _ provider.IconGenerator = (*Icon)(nil)

// NewIcon wires the backend to the shared library.
func NewIcon(lib *Library, store provider.AssetStore) *Icon {
	return &Icon{lib: lib, store: store}
}

// Generate stores one tile's icon and returns its content address. The file is
// chosen by cell index, so a large enough set gives every tile a different
// picture — a grid of ten copies says nothing about whether the layout works.
func (i *Icon) Generate(ctx context.Context, req provider.IconRequest) (entity.AssetID, error) {
	icons, err := i.lib.Icons()
	if err != nil {
		return "", err
	}
	size := req.Size
	if size < 1 {
		size = defaultIconSize
	}
	path := icons[req.Index%len(icons)]

	// An icon is square by the port's definition and the samples are 16:9, so
	// the crop happens here rather than reaching the renderer stretched.
	key := path + "|" + strconv.Itoa(size)
	encoded, err := i.png.bytes(key, path, func(src image.Image) image.Image {
		return square(src, size)
	})
	if err != nil {
		return "", err
	}
	stored, err := i.store.Put(ctx, entity.AssetKindThumbnailIcon, bytes.NewReader(encoded))
	if err != nil {
		return "", fmt.Errorf("store icon: %w", err)
	}
	return stored.ID, nil
}

// square centre-crops an image to its shorter edge and scales that to size.
// Cropping rather than squashing: the middle of an illustration is a fair icon
// where a compressed copy of the whole is not.
func square(src image.Image, size int) image.Image {
	b := src.Bounds()
	edge := min(b.Dx(), b.Dy())
	crop := image.Rect(
		b.Min.X+(b.Dx()-edge)/2,
		b.Min.Y+(b.Dy()-edge)/2,
		b.Min.X+(b.Dx()-edge)/2+edge,
		b.Min.Y+(b.Dy()-edge)/2+edge,
	)
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, crop, draw.Src, nil)
	return dst
}
