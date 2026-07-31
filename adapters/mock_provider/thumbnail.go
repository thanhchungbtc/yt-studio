package mockprovider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// YouTube's thumbnail dimensions. The real backend will have to hit these, so
// the mock does too — it is the size the UI has to lay out for.
const (
	thumbnailWidth  = 1280
	thumbnailHeight = 720
)

// Thumbnail is the mock thumbnail backend. It scales the still it was given up
// to thumbnail size and lays the hook over it as one bar per word: the real
// text rendering waits for a backend with a font, but the length and shape of
// the hook are visible, which is what makes the mock output reviewable.
type Thumbnail struct {
	store  provider.AssetStore
	tuning Tuning
}

var _ provider.ThumbnailBuilder = (*Thumbnail)(nil)

// NewThumbnail constructs the mock.
func NewThumbnail(store provider.AssetStore, tuning Tuning) *Thumbnail {
	return &Thumbnail{store: store, tuning: tuning}
}

// Build renders exactly one thumbnail.
func (b *Thumbnail) Build(ctx context.Context, req provider.ThumbnailRequest) (entity.AssetID, error) {
	if err := simulate(ctx, b.tuning, 1); err != nil {
		return "", err
	}
	if len(req.ImageAssetIDs) == 0 {
		return "", errors.New("mock thumbnail: a thumbnail needs a background still")
	}

	// The first candidate is the daemon's preference; a backend that reached past
	// it would be making the choice the daemon already made.
	background, err := b.still(ctx, req.ImageAssetIDs[0])
	if err != nil {
		return "", err
	}
	img := upscale(background, thumbnailWidth, thumbnailHeight)
	drawHookBars(img, req.Text)

	var buf bytes.Buffer
	buf.Grow(thumbnailWidth * thumbnailHeight / 8)
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return "", fmt.Errorf("encode thumbnail: %w", err)
	}
	stored, err := b.store.Put(ctx, entity.AssetKindThumbnail, &buf)
	if err != nil {
		return "", fmt.Errorf("store thumbnail: %w", err)
	}
	return stored.ID, nil
}

// still decodes one stored still. A thumbnail is a single small image, so this
// is the one place in the mocks where reading the whole file into memory is the
// honest thing to do.
func (b *Thumbnail) still(ctx context.Context, id entity.AssetID) (image.Image, error) {
	f, err := b.store.Open(ctx, id, entity.AssetKindImage)
	if err != nil {
		return nil, fmt.Errorf("open still %s: %w", id.Short(), err)
	}
	defer func() { _ = f.Close() }()

	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode still %s: %w", id.Short(), err)
	}
	return img, nil
}

// upscale resamples by nearest neighbour. A thumbnail built from a 320x180 mock
// still is blocky, which is the point: nobody should mistake it for output from
// a real backend.
func upscale(src image.Image, width, height int) *image.RGBA {
	bounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		sy := bounds.Min.Y + y*bounds.Dy()/height
		for x := range width {
			sx := bounds.Min.X + x*bounds.Dx()/width
			r, g, bl, a := src.At(sx, sy).RGBA()
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8(r >> 8),  //nolint:gosec // RGBA returns 16-bit channels
				G: uint8(g >> 8),  //nolint:gosec // RGBA returns 16-bit channels
				B: uint8(bl >> 8), //nolint:gosec // RGBA returns 16-bit channels
				A: uint8(a >> 8),  //nolint:gosec // RGBA returns 16-bit channels
			})
		}
	}
	return dst
}

// drawHookBars darkens the lower band and lays one bar per word of the hook,
// each as wide as its word is long. An empty hook leaves the still untouched,
// so a missing thumbnail text is visible as a missing overlay rather than as a
// silently ordinary picture.
func drawHookBars(img *image.RGBA, text string) {
	words := strings.Fields(text)
	if len(words) == 0 {
		return
	}

	const (
		margin  = thumbnailWidth / 16
		barGap  = 16
		barRise = 4
	)
	bandTop := thumbnailHeight / 2
	for y := bandTop; y < thumbnailHeight; y++ {
		for x := range thumbnailWidth {
			img.SetRGBA(x, y, blend(img.RGBAAt(x, y), color.RGBA{A: 255}, 0.55))
		}
	}

	// One row per word, sharing the band evenly, so a long hook fills it the way
	// wrapped text would.
	rowHeight := (thumbnailHeight - bandTop - margin) / len(words)
	barHeight := max(rowHeight-barGap, barRise)
	unit := (thumbnailWidth - 2*margin) / 12 // a 12-character word spans the band
	for i, word := range words {
		width := min(len(word)*unit, thumbnailWidth-2*margin)
		top := bandTop + margin/2 + i*rowHeight
		for y := top; y < min(top+barHeight, thumbnailHeight); y++ {
			for x := margin; x < margin+width; x++ {
				img.SetRGBA(x, y, color.RGBA{R: 240, G: 232, B: 210, A: 255})
			}
		}
	}
}
