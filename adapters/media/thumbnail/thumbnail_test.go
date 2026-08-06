package thumbnail_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/tbui/yt-studio/adapters/assetstore"
	"github.com/tbui/yt-studio/adapters/media/thumbnail"
	"github.com/tbui/yt-studio/adapters/mock/media"
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
	icons := media.NewIcon(store)
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

// baselineRequest is the reference thumbnail the assertions below share. A
// render is a second of CPU at this frame size, so the tests that only inspect
// one build it once between them rather than each paying for its own.
func baselineRequest(t *testing.T) provider.ThumbnailRequest {
	t.Helper()
	return provider.ThumbnailRequest{
		VideoID: "v1", VideoRef: "DSS-1", Title: "The Long Winter",
		Headline: "BIGGEST PSYCHOLOGY IDEAS", Cells: tenCells(t, sharedStore(t)),
	}
}

var baseline = sync.OnceValues(func() (entity.AssetID, error) {
	store, err := shared()
	if err != nil {
		return "", err
	}
	dir, err := filepath.Abs(filepath.Join("..", "..", "var", "resources"))
	if err != nil {
		return "", err
	}
	icons := media.NewIcon(store)
	cells := make([]provider.ThumbnailIconCell, 0, len(baselineCaptions))
	for i, caption := range baselineCaptions {
		id, iconErr := icons.Icon(context.Background(), provider.ThumbnailIconRequest{
			VideoID: "v1", Index: i, Prompt: "a stone archway — " + caption, Size: 96,
		})
		if iconErr != nil {
			return "", iconErr
		}
		cells = append(cells, provider.ThumbnailIconCell{Caption: caption, IconAssetID: id})
	}
	b := thumbnail.New(store, dir,
		func() thumbnail.Options { return thumbnail.Options{Rows: 2} },
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	return b.Build(context.Background(), provider.ThumbnailRequest{
		VideoID: "v1", VideoRef: "DSS-1", Title: "The Long Winter",
		Headline: "BIGGEST PSYCHOLOGY IDEAS", Cells: cells,
	})
})

var baselineCaptions = []string{
	"Unconscious Rules", "Mind Control", "Self Birth", "Trauma Embodied", "Self Needs",
	"Insight Blindness", "Split Personality", "Inner Critic", "Shadow Work", "False Memory",
}

func baselineID(t *testing.T) entity.AssetID {
	t.Helper()
	repoResources(t)
	id, err := baseline()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRendersAtYouTubeSize(t *testing.T) {
	t.Parallel()
	if got := decode(t, sharedStore(t), baselineID(t)).Bounds(); got.Dx() != 1280 || got.Dy() != 720 {
		t.Fatalf("thumbnail is %dx%d, want 1280x720", got.Dx(), got.Dy())
	}
}

// Determinism is what keeps content addressing meaningful: a re-run that
// changed nothing must not produce a second file.
func TestRenderIsDeterministic(t *testing.T) {
	t.Parallel()
	b, _ := newBuilder(t, thumbnail.Options{Rows: 2})
	again, err := b.Build(context.Background(), baselineRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	if again != baselineID(t) {
		t.Fatalf("identical requests produced %s and %s", baselineID(t).Short(), again.Short())
	}
}

// Every part of the request has to reach the pixels. Without this the renderer
// could be drawing a fixed frame and nobody would notice until upload.
func TestEveryInputReachesTheImage(t *testing.T) {
	t.Parallel()
	b, _ := newBuilder(t, thumbnail.Options{Rows: 2})
	base := baselineRequest(t)
	cells := base.Cells
	want := baselineID(t)

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
		if id == want {
			t.Errorf("a changed %s did not reach the image", name)
		}
	}
}

// The rows setting is the one layout knob, so it has to actually change the
// layout rather than being read and ignored.
func TestRowsChangeTheLayout(t *testing.T) {
	t.Parallel()
	store := sharedStore(t)
	one := thumbnail.New(store, repoResources(t),
		func() thumbnail.Options { return thumbnail.Options{Rows: 1} },
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	single, err := one.Build(context.Background(), baselineRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	if single == baselineID(t) {
		t.Fatal("one row and two rows rendered identically")
	}
}

// A hook nobody wrote is not a reason to fail: the grid is still the artifact,
// and a missing headline should be visibly missing rather than fatal.
func TestEmptyHeadlineStillRenders(t *testing.T) {
	t.Parallel()
	b, store := newBuilder(t, thumbnail.Options{Rows: 2})

	bare := baselineRequest(t)
	bare.Headline = ""
	id, err := b.Build(context.Background(), bare)
	if err != nil {
		t.Fatal(err)
	}
	if got := decode(t, store, id).Bounds(); got.Dx() != 1280 || got.Dy() != 720 {
		t.Fatalf("thumbnail is %dx%d, want 1280x720", got.Dx(), got.Dy())
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

// A long hook wraps rather than overflowing the frame or failing.
func TestLongHeadlineWraps(t *testing.T) {
	t.Parallel()
	b, _ := newBuilder(t, thumbnail.Options{Rows: 2})

	long := baselineRequest(t)
	long.Headline = "FIFTY BROKEN BELIEFS ABOUT THE MIND AND EVERYTHING ELSE"
	id, err := b.Build(context.Background(), long)
	if err != nil {
		t.Fatal(err)
	}
	if id == baselineID(t) {
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
