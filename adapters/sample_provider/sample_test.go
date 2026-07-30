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

func TestMissingMediaIsUnavailableAndNotRetryable(t *testing.T) {
	t.Parallel()
	lib := sampleprovider.NewLibrary(t.TempDir())

	err := lib.Check()
	if !errors.Is(err, sampleprovider.ErrUnavailable) {
		t.Fatalf("Check() = %v, want ErrUnavailable", err)
	}
	// The port's sentinel is what app.classify reads to decide against retrying
	// a directory that will not appear on its own.
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
