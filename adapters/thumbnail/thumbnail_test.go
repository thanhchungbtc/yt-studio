package thumbnail_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/tbui/yt-studio/adapters/assetstore"
	mockprovider "github.com/tbui/yt-studio/adapters/mock_provider"
	"github.com/tbui/yt-studio/adapters/thumbnail"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// repoResources locates the production resource directory, which is not
// committed — it is operator-supplied and lives under var/.
func repoResources(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "var", "resources"))
	if err != nil {
		t.Fatalf("resolve resources: %v", err)
	}
	for _, f := range []string{
		filepath.Join(dir, "background.jpg"),
		filepath.Join(dir, "fonts", "CabinSketch-Bold.ttf"),
	} {
		if _, err := os.Stat(f); err != nil {
			t.Skipf("skipping: %s is missing", f)
		}
	}
	return dir
}

// The store and the icons in it are shared across the package's tests. They are
// content-addressed and read-only once written, and generating ten icons per
// test costs more than every assertion in the file put together.
var shared = sync.OnceValues(func() (*assetstore.FS, error) {
	return assetstore.New(filepath.Join(os.TempDir(), "yts-thumbnail-test-assets"))
})

func sharedStore(t *testing.T) *assetstore.FS {
	t.Helper()
	store, err := shared()
	if err != nil {
		t.Fatalf("asset store: %v", err)
	}
	return store
}

func newBuilder(t *testing.T, opts thumbnail.Options) (*thumbnail.Builder, *assetstore.FS) {
	t.Helper()
	store := sharedStore(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return thumbnail.New(store, repoResources(t), func() thumbnail.Options { return opts }, log), store
}

// grid generates real icons through the mock backend, which is the shape the
// icon tasks hand the renderer.
func grid(t *testing.T, store provider.AssetStore, captions ...string) []provider.ThumbnailIconCell {
	t.Helper()
	icons := mockprovider.NewIcon(store, nil)
	cells := make([]provider.ThumbnailIconCell, 0, len(captions))
	for i, caption := range captions {
		id, err := icons.Icon(context.Background(), provider.ThumbnailIconRequest{
			VideoID: "v1", Index: i, Prompt: "a stone archway — " + caption, Size: 96,
		})
		if err != nil {
			t.Fatal(err)
		}
		cells = append(cells, provider.ThumbnailIconCell{Caption: caption, IconAssetID: id})
	}
	return cells
}

func tenCells(t *testing.T, store provider.AssetStore) []provider.ThumbnailIconCell {
	t.Helper()
	return grid(t, store,
		"Unconscious Rules", "Mind Control", "Self Birth", "Trauma Embodied", "Self Needs",
		"Insight Blindness", "Split Personality", "Inner Critic", "Shadow Work", "False Memory")
}

// A builder over its own empty store, for the cases that are about a builder
// rather than about an image.
func bareBuilder(t *testing.T, dir string) *thumbnail.Builder {
	t.Helper()
	store, err := assetstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return thumbnail.New(store, dir, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func decode(t *testing.T, store *assetstore.FS, id entity.AssetID) image.Image {
	t.Helper()
	rc, err := store.Open(context.Background(), id, entity.AssetKindThumbnail)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	img, err := png.Decode(rc)
	if err != nil {
		t.Fatalf("output is not a valid PNG: %v", err)
	}
	return img
}

func TestRendersAtYouTubeSize(t *testing.T) {
	t.Parallel()
	b, store := newBuilder(t, thumbnail.Options{Rows: 2})

	id, err := b.Build(context.Background(), provider.ThumbnailRequest{
		VideoID: "v1", VideoRef: "DSS-1", Title: "The Long Winter",
		Headline: "BIGGEST PSYCHOLOGY IDEAS", Cells: tenCells(t, store),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := decode(t, store, id).Bounds(); got.Dx() != 1280 || got.Dy() != 720 {
		t.Fatalf("thumbnail is %dx%d, want 1280x720", got.Dx(), got.Dy())
	}
}

// Determinism is what keeps content addressing meaningful: a re-run that
// changed nothing must not produce a second file.
func TestRenderIsDeterministic(t *testing.T) {
	t.Parallel()
	b, store := newBuilder(t, thumbnail.Options{Rows: 2})
	req := provider.ThumbnailRequest{
		VideoID: "v1", VideoRef: "DSS-1", Headline: "REAL CHEAT CODES",
		Cells: tenCells(t, store),
	}
	first, err := b.Build(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.Build(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("identical requests produced %s and %s", first.Short(), second.Short())
	}
}

// Every part of the request has to reach the pixels. Without this the renderer
// could be drawing a fixed frame and nobody would notice until upload.
func TestEveryInputReachesTheImage(t *testing.T) {
	t.Parallel()
	b, store := newBuilder(t, thumbnail.Options{Rows: 2})
	cells := tenCells(t, store)
	base := provider.ThumbnailRequest{
		VideoID: "v1", VideoRef: "DSS-1", Headline: "REAL CHEAT CODES", Cells: cells,
	}
	baseline, err := b.Build(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}

	changedHeadline := base
	changedHeadline.Headline = "REAL CHEAT CODEX"

	changedCaption := base
	changedCaption.Cells = append([]provider.ThumbnailIconCell(nil), cells...)
	changedCaption.Cells[len(cells)-1].Caption = "Something Else"

	fewerCells := base
	fewerCells.Cells = cells[:6]

	for name, req := range map[string]provider.ThumbnailRequest{
		"headline": changedHeadline,
		"caption":  changedCaption,
		"cells":    fewerCells,
	} {
		id, err := b.Build(context.Background(), req)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if id == baseline {
			t.Errorf("a changed %s did not reach the image", name)
		}
	}
}

// The rows setting is the one layout knob, so it has to actually change the
// layout rather than being read and ignored.
func TestRowsChangeTheLayout(t *testing.T) {
	t.Parallel()
	two, store := newBuilder(t, thumbnail.Options{Rows: 2})
	cells := tenCells(t, store)
	req := provider.ThumbnailRequest{VideoID: "v1", Headline: "TEN IDEAS", Cells: cells}

	first, err := two.Build(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	one := thumbnail.New(store, repoResources(t),
		func() thumbnail.Options { return thumbnail.Options{Rows: 1} },
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	second, err := one.Build(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("one row and two rows rendered identically")
	}
}

// The design's one piece of colour: if the rule is missing, the thumbnail is
// not the design.
func TestHeadlineRuleIsDrawn(t *testing.T) {
	t.Parallel()
	b, store := newBuilder(t, thumbnail.Options{Rows: 2})

	id, err := b.Build(context.Background(), provider.ThumbnailRequest{
		VideoID: "v1", Headline: "BIGGEST PSYCHOLOGY IDEAS", Cells: tenCells(t, store),
	})
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, store, id)

	var red int
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			if c.R > 180 && c.G < 90 && c.B < 90 {
				red++
			}
		}
	}
	if red < 500 {
		t.Fatalf("found %d red pixels, expected the rule under the headline", red)
	}
}

// A missing font is the operator's to fix, so it must be unavailable rather
// than a failure that three attempts will repeat.
func TestMissingFontIsUnavailable(t *testing.T) {
	t.Parallel()
	b, store := newBuilder(t, thumbnail.Options{Font: "NoSuchFont.ttf", Rows: 2})

	_, err := b.Build(context.Background(), provider.ThumbnailRequest{
		VideoID: "v1", Headline: "ANYTHING", Cells: tenCells(t, store),
	})
	if !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("error = %v, want provider.ErrUnavailable", err)
	}
	if err := b.Check(); !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("Check error = %v, want provider.ErrUnavailable", err)
	}
}

// A missing resources directory is the same class of problem, and Check is
// where an operator should hear about it.
func TestMissingBackgroundIsUnavailable(t *testing.T) {
	t.Parallel()
	if err := bareBuilder(t, t.TempDir()).Check(); !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("Check error = %v, want provider.ErrUnavailable", err)
	}
}

// A cell whose icon never landed is a caller error, not a hole in the grid.
func TestCellWithNoIconIsRejected(t *testing.T) {
	t.Parallel()
	b, _ := newBuilder(t, thumbnail.Options{Rows: 2})

	if _, err := b.Build(context.Background(), provider.ThumbnailRequest{
		VideoID: "v1", Headline: "ANYTHING",
		Cells: []provider.ThumbnailIconCell{{Caption: "Mind Control"}},
	}); err == nil {
		t.Fatal("Build with an iconless cell returned no error")
	}
}

// A headline nobody wrote is not a reason to fail: the grid is still the
// artifact, and a missing hook should be visibly missing.
func TestEmptyHeadlineStillRenders(t *testing.T) {
	t.Parallel()
	b, store := newBuilder(t, thumbnail.Options{Rows: 2})

	id, err := b.Build(context.Background(), provider.ThumbnailRequest{
		VideoID: "v1", Cells: tenCells(t, store),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := decode(t, store, id).Bounds(); got.Dx() != 1280 {
		t.Fatalf("thumbnail is %d wide", got.Dx())
	}
}

// A long hook wraps rather than overflowing the frame or failing.
func TestLongHeadlineWraps(t *testing.T) {
	t.Parallel()
	b, store := newBuilder(t, thumbnail.Options{Rows: 2})

	id, err := b.Build(context.Background(), provider.ThumbnailRequest{
		VideoID: "v1", Cells: tenCells(t, store),
		Headline: "FIFTY BROKEN BELIEFS ABOUT THE MIND AND EVERYTHING ELSE",
	})
	if err != nil {
		t.Fatal(err)
	}
	short, err := b.Build(context.Background(), provider.ThumbnailRequest{
		VideoID: "v1", Cells: tenCells(t, store), Headline: "FIFTY",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == short {
		t.Fatal("headline length made no difference")
	}
}

// dump writes a thumbnail out for eyeballing when YTS_THUMB_DUMP is set. It is
// a development aid, not an assertion.
func dump(t *testing.T, store *assetstore.FS, id entity.AssetID) {
	t.Helper()
	path := os.Getenv("YTS_THUMB_DUMP")
	if path == "" {
		return
	}
	rc, err := store.Open(context.Background(), id, entity.AssetKindThumbnail)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rc); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDump(t *testing.T) {
	if os.Getenv("YTS_THUMB_DUMP") == "" {
		t.Skip("set YTS_THUMB_DUMP to write a sample thumbnail")
	}
	b, store := newBuilder(t, thumbnail.Options{Rows: 2})
	id, err := b.Build(context.Background(), provider.ThumbnailRequest{
		VideoID: "v1", VideoRef: "DSS-1", Title: "The Long Winter",
		Headline: "BIGGEST PSYCHOLOGY IDEAS", Cells: tenCells(t, store),
	})
	if err != nil {
		t.Fatal(err)
	}
	dump(t, store, id)
}
