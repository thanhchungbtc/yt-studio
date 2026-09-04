// Package thumbnail renders a video's thumbnail in Go: the headline and the
// grid of generated icons, over the channel's background.
//
// Pure Go rather than a filter graph or a browser, because text is the whole
// problem here and x/image/font already measures and rasterises. An HTML
// backend will come later for layouts worth editing without a rebuild; this one
// is the deterministic floor — same inputs, same bytes, no external process.
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
	"sort"
	"strings"
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
	// MinorWords are the headline words drawn in headlineMinorColor, as the
	// settings row carries them: separated by commas or by whitespace. Empty
	// draws the whole headline in one colour.
	MinorWords string
}

// Renderer implements provider.ThumbnailRenderer.
type Renderer struct {
	store provider.AssetStore
	dir   string
	opts  func() Options
	log   *slog.Logger

	fonts fontCache
}

var _ provider.ThumbnailRenderer = (*Renderer)(nil)

// New wires the renderer against a resources directory.
func New(store provider.AssetStore, resources string, opts func() Options, log *slog.Logger) *Renderer {
	return &Renderer{store: store, dir: resources, opts: opts, log: log}
}

// Check reports whether the fixed resources are in place, at startup rather
// than from a parked task forty minutes in.
func (b *Renderer) Check() error {
	opts := b.options()
	if _, err := b.background(); err != nil {
		return err
	}
	if _, err := b.face(opts.Font); err != nil {
		return err
	}
	return nil
}

// Render renders exactly one thumbnail.
func (b *Renderer) Render(ctx context.Context, req provider.ThumbnailRequest) (entity.AssetID, error) {
	opts := b.options()
	font, err := b.face(opts.Font)
	if err != nil {
		return "", err
	}
	background, err := b.background()
	if err != nil {
		return "", err
	}

	canvas := image.NewRGBA(image.Rect(0, 0, frameWidth, frameHeight))
	copy(canvas.Pix, background.Pix)

	// The grid takes the width it needs and the headline is fitted into what is
	// left, which is why the tiles run edge to edge.
	cells := layOutGrid(len(req.Cells), opts.Rows)
	headline := layOutHeadline(font, req.Headline, cells.headlineBudget())
	drawHeadline(canvas, headline, minorWords(opts.MinorWords))
	cells.place(headlineTopMargin + headline.height())

	if err := b.drawGrid(ctx, canvas, font, req.Cells, cells); err != nil {
		return "", err
	}

	var buf bytes.Buffer
	buf.Grow(frameWidth * frameHeight / 2)
	// Default compression, not best: on a photographic frame this size, best
	// costs seven times the CPU to save six percent, against a two megabyte
	// ceiling. The sample's flat colours are a different case.
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

func (b *Renderer) options() Options {
	opts := Options{}
	if b.opts != nil {
		opts = b.opts()
	}
	if opts.Font == "" {
		opts.Font = defaultFontFile
	}
	if opts.Rows < 1 {
		opts.Rows = defaultGridRows
	}
	return opts
}

// background returns the frame every thumbnail starts from: the backdrop scaled
// to cover, scrimmed so white text over it is legible. Cached by path, because
// decoding and resampling the photograph is the most expensive thing here and
// the answer never differs.
func (b *Renderer) background() (*image.RGBA, error) {
	path := filepath.Join(b.dir, backgroundFileName)

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
		return nil, fmt.Errorf("%w: %s: %w", ErrUnavailable, backgroundFileName, err)
	}
	defer func() { _ = f.Close() }()

	src, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%w: decode %s: %w", ErrUnavailable, backgroundFileName, err)
	}
	img := cover(src, frameWidth, frameHeight)
	scrim(img)
	return img, nil
}

// Fonts lists the typefaces in a resources directory, for the settings screen
// to offer by name. Read from disk, because what is installed is a fact about
// the machine; offered as suggestions rather than a closed list, because the
// scan happens once at startup and a font added later must not be refused.
//
// An unreadable directory yields nothing rather than an error: Check already
// fails over a typeface it cannot parse.
func Fonts(resources string) []string {
	entries, err := os.ReadDir(filepath.Join(resources, "fonts"))
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".ttf", ".otf":
			out = append(out, entry.Name())
		}
	}
	sort.Strings(out)
	return out
}

// face parses the configured typeface. A font that will not parse is the
// operator's to fix, so it is unavailable rather than retryable.
func (b *Renderer) face(name string) (*sfnt.Font, error) {
	path := filepath.Join(b.dir, "fonts", name)
	parsed, err := b.fonts.load(path)
	if err != nil {
		return nil, fmt.Errorf("%w: font %s: %w", ErrUnavailable, name, err)
	}
	return parsed, nil
}

// drawGrid places every cell and paints its icon and caption.
func (b *Renderer) drawGrid(
	ctx context.Context,
	canvas *image.RGBA,
	font *sfnt.Font,
	cells []provider.IconCell,
	grid grid,
) error {
	if len(cells) == 0 {
		// A headline on its own is a thin thumbnail, not a broken one, and
		// whether that is worth publishing is the server's judgement.
		return nil
	}

	// One size for every caption: a dozen tiles at a dozen type sizes read as a
	// dozen unrelated pictures.
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
func (b *Renderer) icon(ctx context.Context, id entity.AssetID) (image.Image, error) {
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
