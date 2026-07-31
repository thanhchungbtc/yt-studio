package sampleprovider_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/tbui/yt-studio/adapters/assetstore"
	sampleprovider "github.com/tbui/yt-studio/adapters/sample_provider"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// resourcesDir locates the repository's resources directory. The media is
// operator-supplied and gitignored, so these tests skip rather than fail on a
// checkout that has not been given any.
func resourcesDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "var", "resources"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sample")); err != nil {
		t.Skipf("no sample media in %s", dir)
	}
	return dir
}

func newStore(t *testing.T) *assetstore.FS {
	t.Helper()
	store, err := assetstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestSpeakStoresRealAudio(t *testing.T) {
	t.Parallel()
	lib := sampleprovider.NewLibrary(resourcesDir(t))
	store := newStore(t)
	tts := sampleprovider.NewTTS(lib, store)

	id, err := tts.Speak(context.Background(), provider.SpeakRequest{Ordinal: 1, Text: "anything"})
	if err != nil {
		t.Fatal(err)
	}

	stat, err := store.Stat(context.Background(), id, entity.AssetKindAudio)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size < 1<<20 {
		t.Fatalf("stored narration is %d bytes; expected real recorded audio", stat.Size)
	}

	path, err := store.Path(id, entity.AssetKindAudio)
	if err != nil {
		t.Fatal(err)
	}
	header, err := os.ReadFile(path) //nolint:gosec // path is the test's own store
	if err != nil {
		t.Fatal(err)
	}
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		t.Fatal("stored narration is not a RIFF/WAVE file")
	}

	// Every chapter shares one recording, so it must also share one content
	// address — that is what keeps fifty chapters to a single file on disk.
	again, err := tts.Speak(context.Background(), provider.SpeakRequest{Ordinal: 2, Text: "different"})
	if err != nil {
		t.Fatal(err)
	}
	if again != id {
		t.Fatalf("second chapter produced %s, want the same address as %s", again.Short(), id.Short())
	}
}

func TestGenerateStoresDecodablePNG(t *testing.T) {
	t.Parallel()
	lib := sampleprovider.NewLibrary(resourcesDir(t))
	store := newStore(t)
	images := sampleprovider.NewImage(lib, store)

	id, err := images.Generate(context.Background(), provider.ImageRequest{Ordinal: 1, Index: 0})
	if err != nil {
		t.Fatal(err)
	}
	path, err := store.Path(id, entity.AssetKindImage)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path) //nolint:gosec // path is the test's own store
	if err != nil {
		t.Fatal(err)
	}

	// The asset row claims image/png and the file is named .png, so it has to
	// actually be one — a JPEG stored under that name would decode here and
	// nowhere else.
	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("stored still is not a PNG: %v", err)
	}
	if got := decoded.Bounds().Size(); got != (image.Point{X: 1344, Y: 768}) {
		t.Fatalf("still is %v; expected the composer's native 1344x768", got)
	}
}

func TestStillsRotateAndDifferWithinAChapter(t *testing.T) {
	t.Parallel()
	lib := sampleprovider.NewLibrary(resourcesDir(t))
	images := sampleprovider.NewImage(lib, newStore(t))

	ids := make([][]entity.AssetID, 0, 4)
	for ordinal := 1; ordinal <= 4; ordinal++ {
		pair := make([]entity.AssetID, 0, 2)
		for index := range 2 {
			id, err := images.Generate(context.Background(),
				provider.ImageRequest{Ordinal: ordinal, Index: index})
			if err != nil {
				t.Fatal(err)
			}
			pair = append(pair, id)
		}
		// A dissolve between two copies of one image is not a dissolve.
		if pair[0] == pair[1] {
			t.Fatalf("chapter %d got the same still twice", ordinal)
		}
		ids = append(ids, pair)
	}

	// Consecutive chapters must not repeat the same pair all the way down.
	if ids[0][0] == ids[1][0] {
		t.Fatal("consecutive chapters lead with the same still")
	}
}

// An icon is square by the port's definition, and these samples are 16:9 on
// disk. Serving one untouched would reach the renderer stretched, because it
// scales whatever it is given into a square tile.
func TestIconIsStoredSquareAtTheRequestedSize(t *testing.T) {
	t.Parallel()
	lib := sampleprovider.NewLibrary(resourcesDir(t))
	store := newStore(t)
	icons := sampleprovider.NewIcon(lib, store)

	id, err := icons.Icon(context.Background(), provider.ThumbnailIconRequest{
		VideoID: "v1", Index: 0, Prompt: "a pocket watch", Size: 256,
	})
	if err != nil {
		t.Fatal(err)
	}
	path, err := store.Path(id, entity.AssetKindThumbnailIcon)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path) //nolint:gosec // path is the test's own store
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("stored icon is not a PNG: %v", err)
	}
	if got := decoded.Bounds().Size(); got != (image.Point{X: 256, Y: 256}) {
		t.Fatalf("icon is %v, want a 256x256 square", got)
	}

	// The size is part of what was asked for, so it is part of the address.
	larger, err := icons.Icon(context.Background(), provider.ThumbnailIconRequest{
		VideoID: "v1", Index: 0, Prompt: "a pocket watch", Size: 512,
	})
	if err != nil {
		t.Fatal(err)
	}
	if larger == id {
		t.Fatal("two sizes of the same sample landed on one address")
	}
}

// A grid of ten copies of one picture says nothing about whether the layout
// works, which is the only reason to run the pipeline on samples.
func TestIconsDifferAcrossTheGrid(t *testing.T) {
	t.Parallel()
	lib := sampleprovider.NewLibrary(resourcesDir(t))
	icons := sampleprovider.NewIcon(lib, newStore(t))

	seen := make(map[entity.AssetID]int, 4)
	for index := range 4 {
		id, err := icons.Icon(context.Background(), provider.ThumbnailIconRequest{
			VideoID: "v1", Index: index, Prompt: "anything", Size: 128,
		})
		if err != nil {
			t.Fatal(err)
		}
		seen[id]++
	}
	if len(seen) != 4 {
		t.Fatalf("four cells produced %d distinct icons", len(seen))
	}
}

// The icons arrived after the narration and the stills, so a library without
// them still serves those. Only asking for one reports the absence.
func TestLibraryWithoutIconsStillServesStills(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sample := filepath.Join(dir, "sample")
	if err := os.MkdirAll(sample, 0o755); err != nil {
		t.Fatal(err)
	}
	// A bare RIFF header rather than a copy of the real narration: the scan only
	// reads the first twelve bytes, and the sample is five megabytes.
	if err := os.WriteFile(filepath.Join(sample, "audio.wav"),
		[]byte("RIFF\x00\x00\x00\x00WAVEfmt "), 0o600); err != nil {
		t.Fatal(err)
	}
	copyFile(t, filepath.Join(resourcesDir(t), "sample", "img0.jpg"), filepath.Join(sample, "img0.jpg"))

	lib := sampleprovider.NewLibrary(dir)
	if err := lib.Check(); err != nil {
		t.Fatalf("Check() = %v, want a usable library", err)
	}
	if _, err := lib.Icons(); !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("Icons() = %v, want ErrUnavailable", err)
	}
	if _, err := sampleprovider.NewImage(lib, newStore(t)).Generate(context.Background(),
		provider.ImageRequest{Ordinal: 1}); err != nil {
		t.Fatalf("stills stopped working without icons: %v", err)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	raw, err := os.ReadFile(from) //nolint:gosec // both paths are the test's own
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMissingMediaIsUnavailableAndNotRetryable(t *testing.T) {
	t.Parallel()
	lib := sampleprovider.NewLibrary(t.TempDir())

	err := lib.Check()
	if !errors.Is(err, sampleprovider.ErrUnavailable) {
		t.Fatalf("Check() = %v, want ErrUnavailable", err)
	}
	// The port's sentinel is what app.classify reads to decide against retrying a
	// directory that will not appear on its own.
	if !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("Check() does not wrap provider.ErrUnavailable: %v", err)
	}

	store := newStore(t)
	if _, err := sampleprovider.NewTTS(lib, store).Speak(context.Background(), provider.SpeakRequest{}); !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("Speak() = %v, want ErrUnavailable", err)
	}
	if _, err := sampleprovider.NewImage(lib, store).Generate(context.Background(), provider.ImageRequest{}); !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("Generate() = %v, want ErrUnavailable", err)
	}
	if _, err := sampleprovider.NewIcon(lib, store).Icon(context.Background(), provider.ThumbnailIconRequest{}); !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("Icon() = %v, want ErrUnavailable", err)
	}
}

func TestNonWAVMediaIsRejectedUpFront(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sample := filepath.Join(dir, "sample")
	if err := os.MkdirAll(sample, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sample, "audio.wav"), []byte("not audio at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sample, "img0.jpg"), []byte("nor this"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := sampleprovider.NewLibrary(dir).Check()
	if !errors.Is(err, sampleprovider.ErrUnavailable) {
		t.Fatalf("Check() = %v, want a rejection naming the file", err)
	}
}
