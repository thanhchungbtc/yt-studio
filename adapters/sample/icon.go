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

var _ provider.ThumbnailIconGenerator = (*Icon)(nil)

// NewIcon wires the backend to the shared library.
func NewIcon(lib *Library, store provider.AssetStore) *Icon {
	return &Icon{lib: lib, store: store}
}

// Icon stores one tile's icon and returns its content address.
//
// The file is chosen by cell index, so a grid drawn from a set at least as
// large shows a different picture in every tile — which is the point of running
// the pipeline on samples at all: a grid of ten copies of one image says
// nothing about whether the layout works.
func (i *Icon) Icon(ctx context.Context, req provider.ThumbnailIconRequest) (entity.AssetID, error) {
	icons, err := i.lib.Icons()
	if err != nil {
		return "", err
	}
	size := req.Size
	if size < 1 {
		size = defaultIconSize
	}
	path := icons[req.Index%len(icons)]

	// Unlike a slide, an icon is square by the port's definition, and the
	// renderer scales whatever it is handed into a square tile. A 16:9 sample
	// passed through untouched would arrive there stretched, so the crop happens
	// here where the aspect is still known to be wrong.
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
//
// Cropping rather than squashing: these samples are illustrations, and the
// middle of one is a fair icon where a horizontally compressed copy of the
// whole is not.
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
