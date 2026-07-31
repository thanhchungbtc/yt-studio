// Package thumbnail renders a video's thumbnail in Go: the headline, the rule
// under it, and the grid of generated icons over the channel's background.
//
// It is pure Go rather than a filter graph or a browser. Text is the whole
// problem here — fitting a headline to the frame, tracking it, centring
// captions under tiles — and x/image/font already measures and rasterises,
// which is what the composer's title fitting is built on. A browser backend
// will come later for layouts worth editing without a rebuild; this one is the
// deterministic floor: same inputs, same bytes, no external process.
package thumbnail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/image/font/sfnt"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// ErrUnavailable reports a renderer that cannot run until the operator puts
// something in place: a missing background, a font that will not parse.
var ErrUnavailable = fmt.Errorf("thumbnail: %w", provider.ErrUnavailable)

// Options are the settings-sourced knobs, read per render so an edit on the
// settings screen applies to the next thumbnail rather than the next restart.
type Options struct {
	// Font is a filename inside the resources fonts directory.
	Font string
	// Rows is how many rows the grid is laid out in.
	Rows int
}

// Builder implements provider.ThumbnailBuilder.
type Builder struct {
	store provider.AssetStore
	dir   string
	opts  func() Options
	log   *slog.Logger

	fonts fontCache
}

var _ provider.ThumbnailBuilder = (*Builder)(nil)

// New wires the renderer against a resources directory.
func New(store provider.AssetStore, resources string, opts func() Options, log *slog.Logger) *Builder {
	return &Builder{store: store, dir: resources, opts: opts, log: log}
}

// Check reports whether the fixed resources are in place, so an operator learns
// at startup rather than from a parked task forty minutes in.
func (b *Builder) Check() error {
	opts := b.options()
	if _, err := b.background(); err != nil {
		return err
	}
	if _, err := b.face(opts.Font); err != nil {
		return err
	}
	return nil
}

// Build renders exactly one thumbnail.
func (b *Builder) Build(ctx context.Context, req provider.ThumbnailRequest) (entity.AssetID, error) {
	opts := b.options()
	font, err := b.face(opts.Font)
	if err != nil {
		return "", err
	}
	background, err := b.background()
	if err != nil {
		return "", err
	}

	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	copy(canvas.Pix, background.Pix)

	// The grid is laid out first and takes the width it needs; the headline is
	// then fitted into the band above it. That order is the whole reason the
	// tiles run edge to edge instead of floating in the middle of the frame.
	cells := layOutGrid(len(req.Cells), opts.Rows)
	headline := layOutHeadline(font, req.Headline, cells.headlineBudget())
	drawHeadline(canvas, headline)
	cells.place(headlineTop + headline.height())

	if err := b.drawGrid(ctx, canvas, font, req.Cells, cells); err != nil {
		return "", err
	}

	var buf bytes.Buffer
	buf.Grow(width * height / 2)
	// Default compression, not best. On a photographic frame this size, best
	// costs roughly seven times the CPU to save six percent of the bytes — half
	// a megabyte either way, against YouTube's two megabyte ceiling. The mock's
	// flat colours are a different case and it still asks for best.
	enc := png.Encoder{CompressionLevel: png.DefaultCompression}
	if err := enc.Encode(&buf, canvas); err != nil {
		return "", fmt.Errorf("encode thumbnail: %w", err)
	}
	stored, err := b.store.Put(ctx, entity.AssetKindThumbnail, &buf)
	if err != nil {
		return "", fmt.Errorf("store thumbnail: %w", err)
	}
	return stored.ID, nil
}

func (b *Builder) options() Options {
	opts := Options{}
	if b.opts != nil {
		opts = b.opts()
	}
	if opts.Font == "" {
		opts.Font = defaultFont
	}
	if opts.Rows < 1 {
		opts.Rows = defaultRows
	}
	return opts
}

// background returns the frame every thumbnail starts from: the backdrop scaled
// to cover, with a scrim over it. The scrim is not decoration — white text over
// an undimmed photograph is unreadable, and the reference thumbnails are nearly
// black behind their headline.
//
// The result is cached by path rather than per builder. Decoding and
// resampling a quarter-megabyte photograph is by far the most expensive thing
// here and the answer never differs: one backdrop, one frame, for the life of
// the process.
func (b *Builder) background() (*image.RGBA, error) {
	path := filepath.Join(b.dir, backgroundFile)

	backdropsMu.Lock()
	defer backdropsMu.Unlock()
	if cached, ok := backdrops[path]; ok {
		return cached.img, cached.err
	}

	img, err := loadBackground(path)
	if backdrops == nil {
		backdrops = make(map[string]backdrop, 2)
	}
	backdrops[path] = backdrop{img: img, err: err}
	return img, err
}

type backdrop struct {
	img *image.RGBA
	err error
}

var (
	backdropsMu sync.Mutex
	backdrops   map[string]backdrop
)

func loadBackground(path string) (*image.RGBA, error) {
	f, err := os.Open(path) //nolint:gosec // path is operator-configured
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrUnavailable, backgroundFile, err)
	}
	defer func() { _ = f.Close() }()

	src, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%w: decode %s: %w", ErrUnavailable, backgroundFile, err)
	}
	img := cover(src, width, height)
	scrim(img)
	return img, nil
}

// face parses the configured typeface. A font that will not parse is the
// operator's to fix, so it is unavailable rather than retryable.
func (b *Builder) face(name string) (*sfnt.Font, error) {
	path := filepath.Join(b.dir, "fonts", name)
	parsed, err := b.fonts.load(path)
	if err != nil {
		return nil, fmt.Errorf("%w: font %s: %w", ErrUnavailable, name, err)
	}
	return parsed, nil
}

// drawGrid places every cell and paints its icon and caption.
func (b *Builder) drawGrid(
	ctx context.Context,
	canvas *image.RGBA,
	font *sfnt.Font,
	cells []provider.ThumbnailIconCell,
	grid grid,
) error {
	if len(cells) == 0 {
		// A headline on its own is a thin thumbnail, not a broken one. Whether an
		// empty grid is worth publishing is the daemon's judgement, not a
		// renderer's.
		return nil
	}

	// One size for every caption, chosen once the tiles are sized: ten tiles at
	// ten type sizes read as ten unrelated pictures.
	captions := make([]string, 0, len(cells))
	for _, c := range cells {
		captions = append(captions, c.Caption)
	}
	captionFace := fitCaptions(font, captions, grid.tileSize)

	for i, cell := range cells {
		icon, err := b.icon(ctx, cell.IconAssetID)
		if err != nil {
			return err
		}
		box := grid.tile(i)
		drawTile(canvas, box, icon)
		drawCaption(canvas, captionFace, cell.Caption, box, grid.captionTop(i))
	}
	return nil
}

// icon decodes one stored icon. Each is a small square, so this is the one
// place here where reading a whole file into memory is the honest thing to do.
func (b *Builder) icon(ctx context.Context, id entity.AssetID) (image.Image, error) {
	if id == "" {
		return nil, errors.New("thumbnail: a cell has no icon")
	}
	f, err := b.store.Open(ctx, id, entity.AssetKindThumbnailIcon)
	if err != nil {
		return nil, fmt.Errorf("open icon %s: %w", id.Short(), err)
	}
	defer func() { _ = f.Close() }()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode icon %s: %w", id.Short(), err)
	}
	return img, nil
}
