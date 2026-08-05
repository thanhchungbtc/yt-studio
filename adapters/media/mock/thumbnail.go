package mock

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

// Thumbnail is the mock thumbnail backend: a black frame, the headline as one
// bar per word, and the icons tiled in a grid with a bar under each for its
// caption.
//
// Text is bars rather than letters because the backend that will really draw
// this is a browser rendering an HTML template, and a Go renderer that grew
// fonts and wrapping would be a second implementation to keep in step. Bars
// carry what the mock is for: the grid is the right shape, the icons are in the
// right cells, and a caption that is too long is visibly too long.
type Thumbnail struct {
	store provider.AssetStore
}

var _ provider.ThumbnailBuilder = (*Thumbnail)(nil)

// NewThumbnail constructs the mock.
func NewThumbnail(store provider.AssetStore) *Thumbnail {
	return &Thumbnail{store: store}
}

// Build renders exactly one thumbnail.
func (b *Thumbnail) Build(ctx context.Context, req provider.ThumbnailRequest) (entity.AssetID, error) {

	img := image.NewRGBA(image.Rect(0, 0, thumbnailWidth, thumbnailHeight))
	fill(img, color.RGBA{R: 8, G: 8, B: 10, A: 255})

	headlineBottom := drawHeadline(img, req.Headline)
	if err := b.drawGrid(ctx, img, req.Cells, headlineBottom); err != nil {
		return "", err
	}

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

// Layout constants. The real layout is CSS in the builder's template; these
// exist so the mock's output is recognisably the same design.
const (
	thumbMargin     = thumbnailWidth / 24
	headlineTop     = thumbnailHeight / 12
	headlineBarH    = 56
	headlineGap     = 14
	captionBarH     = 14
	captionGap      = 10
	iconTileGap     = 18
	headlineToGrid  = 28
	underlineHeight = 8
	underlineInset  = thumbnailWidth / 3
	// A headline is set to fill the line: roughly 26 characters spans the frame,
	// which is what makes a four-word hook wrap and a three-word hook not.
	headlineCharW = (thumbnailWidth - 2*thumbMargin) / 26
	tileBorder    = 3
)

// drawHeadline lays the hook out as one bar per word, wrapping when the line is
// full, and returns the y the grid may start at. An empty hook draws nothing
// and gives the grid the whole frame, so a missing headline is visible as a
// missing headline.
func drawHeadline(img *image.RGBA, headline string) int {
	words := strings.Fields(headline)
	if len(words) == 0 {
		return headlineTop
	}
	white := color.RGBA{R: 245, G: 245, B: 245, A: 255}

	x, y := thumbMargin, headlineTop
	lineEnd := thumbnailWidth - thumbMargin
	for _, w := range words {
		width := min(len(w)*headlineCharW, lineEnd-thumbMargin)
		if x+width > lineEnd && x > thumbMargin {
			x = thumbMargin
			y += headlineBarH + headlineGap
		}
		rect(img, x, y, width, headlineBarH, white)
		x += width + headlineGap
	}
	y += headlineBarH

	// The red rule under the headline, which is the design's one piece of colour.
	rect(img, thumbnailWidth-thumbMargin-underlineInset, y+headlineGap,
		underlineInset, underlineHeight, color.RGBA{R: 220, G: 38, B: 38, A: 255})
	return y + headlineGap + underlineHeight + headlineToGrid
}

// drawGrid tiles the icons into as square a grid as the cell count allows, then
// bars a caption under each. Rows and columns are the mock's own arithmetic:
// the real layout is CSS, and this only has to be close enough to see whether
// the icons and captions line up.
func (b *Thumbnail) drawGrid(ctx context.Context, img *image.RGBA, cells []provider.ThumbnailIconCell, top int) error {
	if len(cells) == 0 {
		// A headline on its own is a thin thumbnail, not a broken one. Whether an
		// empty grid is worth publishing is the server's judgement, not a
		// renderer's.
		return nil
	}
	rows := 2
	if len(cells) <= 3 {
		rows = 1
	}
	cols := (len(cells) + rows - 1) / rows

	available := thumbnailHeight - top - thumbMargin
	rowHeight := available / rows
	tile := min((thumbnailWidth-2*thumbMargin-(cols-1)*iconTileGap)/cols,
		rowHeight-captionBarH-captionGap)
	if tile < 8 {
		return fmt.Errorf("mock thumbnail: %d cells do not fit the frame", len(cells))
	}
	// Centre the grid on what the tiles actually came out as, so a wide grid and
	// a narrow one are both centred rather than left-aligned.
	gridWidth := cols*tile + (cols-1)*iconTileGap
	originX := (thumbnailWidth - gridWidth) / 2

	for i, cell := range cells {
		icon, err := b.icon(ctx, cell.IconAssetID)
		if err != nil {
			return err
		}
		row, col := i/cols, i%cols
		x := originX + col*(tile+iconTileGap)
		y := top + row*rowHeight

		// The border is what separates one tile from the next on a black frame;
		// without it a grid of black squares on black reads as one block.
		rect(img, x, y, tile, tile, color.RGBA{R: 90, G: 90, B: 96, A: 255})
		blit(img, upscale(icon, tile-2*tileBorder, tile-2*tileBorder), x+tileBorder, y+tileBorder)
		// A caption bar as wide as its text is long, centred under the tile.
		width := min(len(cell.Caption)*tile/14, tile)
		rect(img, x+(tile-width)/2, y+tile+captionGap, width, captionBarH,
			color.RGBA{R: 210, G: 210, B: 210, A: 255})
	}
	return nil
}

// icon decodes one stored icon. Each is a small square, so this is the one
// place in the mocks where reading a whole file into memory is the honest thing
// to do.
func (b *Thumbnail) icon(ctx context.Context, id entity.AssetID) (image.Image, error) {
	if id == "" {
		return nil, errors.New("mock thumbnail: a cell has no icon")
	}
	f, err := b.store.Open(ctx, id, entity.AssetKindThumbnailIcon)
	if err != nil {
		return nil, fmt.Errorf("open icon %s: %w", id.Short(), err)
	}
	defer func() { _ = f.Close() }()

	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode icon %s: %w", id.Short(), err)
	}
	return img, nil
}

func fill(img *image.RGBA, c color.RGBA) {
	bounds := img.Bounds()
	for y := range bounds.Dy() {
		for x := range bounds.Dx() {
			img.SetRGBA(x, y, c)
		}
	}
}

func rect(img *image.RGBA, x, y, width, height int, c color.RGBA) {
	bounds := img.Bounds()
	for py := max(y, 0); py < min(y+height, bounds.Dy()); py++ {
		for px := max(x, 0); px < min(x+width, bounds.Dx()); px++ {
			img.SetRGBA(px, py, c)
		}
	}
}

func blit(dst, src *image.RGBA, x, y int) {
	bounds := dst.Bounds()
	for sy := range src.Bounds().Dy() {
		for sx := range src.Bounds().Dx() {
			px, py := x+sx, y+sy
			if px >= 0 && py >= 0 && px < bounds.Dx() && py < bounds.Dy() {
				dst.SetRGBA(px, py, src.RGBAAt(sx, sy))
			}
		}
	}
}

// upscale resamples by nearest neighbour. Icons are drawn at one size and tiled
// at another, and a blocky tile is a fair signal that this is a mock.
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
