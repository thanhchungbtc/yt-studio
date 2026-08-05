package ffmpeg

import (
	"fmt"
	"image/png"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/tbui/yt-studio/adapters/assetstore"
	mock "github.com/tbui/yt-studio/adapters/media/mock"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// The tests below drive the real binary against the real production resources.
// They are the only way to know the graphs are not merely well-formed strings,
// so they run whenever the machine can support them and skip when it cannot.

func requireFFmpeg(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("skipping: %s is not on PATH", bin)
		}
	}
}

// repoResources locates the production resource directory, which is not
// committed — it is operator-supplied and lives under var/.
func repoResources(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "var", "resources"))
	if err != nil {
		t.Fatalf("resolve resources: %v", err)
	}
	res := NewResources(dir)
	for _, f := range []string{res.Chalkboard, res.BgVideo, res.TitleFont} {
		if _, err := os.Stat(f); err != nil {
			t.Skipf("skipping: %s is missing", f)
		}
	}
	return dir
}

func newTestComposer(t *testing.T) (*Composer, *assetstore.FS) {
	t.Helper()
	store, err := assetstore.New(t.TempDir())
	if err != nil {
		t.Fatalf("asset store: %v", err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(store, repoResources(t), log), store
}

// seedChapter generates one chapter's worth of input with the mock backends,
// which produce genuinely valid PNG and WAV files.
func seedChapter(t *testing.T, store provider.AssetStore, ordinal, slides int) (entity.AssetID, []entity.AssetID) {
	t.Helper()
	ctx := t.Context()

	audio, err := mock.NewTTS(store, nil).Speak(ctx, provider.SpeakRequest{
		VideoID: "video-1",
		Ordinal: ordinal,
		Text:    fmt.Sprintf("narration for chapter %d", ordinal),
	})
	if err != nil {
		t.Fatalf("mock narration: %v", err)
	}

	gen := mock.NewSlide(store, nil)
	ids := make([]entity.AssetID, 0, slides)
	for i := range slides {
		id, err := gen.Generate(ctx, provider.SlideRequest{
			VideoID: "video-1",
			Ordinal: ordinal,
			Index:   i,
			Prompt:  fmt.Sprintf("chapter %d slide %d", ordinal, i),
		})
		if err != nil {
			t.Fatalf("mock slide: %v", err)
		}
		ids = append(ids, id)
	}
	return audio, ids
}

// probeField reads one ffprobe field, so a test can assert on the container
// rather than on the fact that a file exists.
func probeField(t *testing.T, path string, args ...string) string {
	t.Helper()
	argv := append([]string{"-v", "error"}, args...)
	argv = append(argv, "-of", "default=noprint_wrappers=1:nokey=1", path)
	out, err := exec.Command("ffprobe", argv...).Output()
	if err != nil {
		t.Fatalf("ffprobe %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func probeFloat(t *testing.T, path string, args ...string) float64 {
	t.Helper()
	value, err := strconv.ParseFloat(probeField(t, path, args...), 64)
	if err != nil {
		t.Fatalf("parse ffprobe output: %v", err)
	}
	return value
}

func TestClipComposesARealChapter(t *testing.T) {
	requireFFmpeg(t)

	composer, store := newTestComposer(t)
	audio, slides := seedChapter(t, store, 1, 2)

	id, err := composer.Clip(t.Context(), provider.ClipRequest{
		VideoID:       "video-1",
		ChapterID:     "chapter-1",
		Ordinal:       1,
		ChapterTitle:  "The Rise and Fall",
		VideoTitle:    "A History of Rome",
		AudioAssetID:  audio,
		SlideAssetIDs: slides,
	})
	if err != nil {
		t.Fatalf("compose clip: %v", err)
	}

	path, err := store.Path(id, entity.AssetKindClip)
	if err != nil {
		t.Fatalf("resolve clip: %v", err)
	}

	// The mock narrates for exactly one second, so the clip is that plus the two
	// crossfade pads and the trailing beat.
	const want = 1.0 + 2*chapterCrossfade + chapterTailPad
	got := probeFloat(t, path, "-show_entries", "format=duration")
	if diff := got - want; diff > 0.15 || diff < -0.15 {
		t.Errorf("clip runs %.3fs, want %.3fs", got, want)
	}

	size := probeField(t, path, "-select_streams", "v:0", "-show_entries", "stream=width,height")
	if size != fmt.Sprintf("%d\n%d", imageWidth, imageHeight) {
		t.Errorf("clip is %s, want %dx%d", strings.ReplaceAll(size, "\n", "x"), imageWidth, imageHeight)
	}
	if codec := probeField(t, path, "-select_streams", "a:0", "-show_entries", "stream=codec_name"); codec != "aac" {
		t.Errorf("clip audio is %q, want aac", codec)
	}
}

func TestConcatComposesTheFinalRender(t *testing.T) {
	requireFFmpeg(t)

	composer, store := newTestComposer(t)
	ctx := t.Context()

	clips := make([]entity.AssetID, 0, 2)
	clipTotal := 0.0
	for ordinal := 1; ordinal <= 2; ordinal++ {
		audio, slides := seedChapter(t, store, ordinal, 2)
		id, err := composer.Clip(ctx, provider.ClipRequest{
			VideoID:       "video-1",
			ChapterID:     entity.ChapterID(fmt.Sprintf("chapter-%d", ordinal)),
			Ordinal:       ordinal,
			ChapterTitle:  fmt.Sprintf("Chapter %d", ordinal),
			VideoTitle:    "A History of Rome",
			AudioAssetID:  audio,
			SlideAssetIDs: slides,
		})
		if err != nil {
			t.Fatalf("compose clip %d: %v", ordinal, err)
		}
		clipPath, err := store.Path(id, entity.AssetKindClip)
		if err != nil {
			t.Fatalf("resolve clip %d: %v", ordinal, err)
		}
		clipTotal += probeFloat(t, clipPath, "-select_streams", "v:0", "-show_entries", "stream=duration")
		clips = append(clips, id)
	}

	id, err := composer.Concat(ctx, provider.ConcatRequest{VideoID: "video-1", ClipAssetIDs: clips})
	if err != nil {
		t.Fatalf("concatenate: %v", err)
	}
	path, err := store.Path(id, entity.AssetKindFinal)
	if err != nil {
		t.Fatalf("resolve final: %v", err)
	}

	// The chapters overlap by one crossfade, so the render is shorter than the sum
	// of its parts by exactly that much.
	want := clipTotal - chapterCrossfade
	got := probeFloat(t, path, "-select_streams", "v:0", "-show_entries", "stream=duration")
	if diff := got - want; diff > 0.2 || diff < -0.2 {
		t.Errorf("final render runs %.3fs, want %.3fs", got, want)
	}

	size := probeField(t, path, "-select_streams", "v:0", "-show_entries", "stream=width,height")
	if size != fmt.Sprintf("%d\n%d", concatWidth, concatHeight) {
		t.Errorf("final render is %s, want %dx%d", strings.ReplaceAll(size, "\n", "x"), concatWidth, concatHeight)
	}
}

func TestClipRejectsIncompleteRequests(t *testing.T) {
	requireFFmpeg(t)

	composer, store := newTestComposer(t)
	audio, slides := seedChapter(t, store, 1, 1)

	tests := []struct {
		name string
		req  provider.ClipRequest
	}{
		{name: "no slides", req: provider.ClipRequest{AudioAssetID: audio}},
		{name: "no narration", req: provider.ClipRequest{SlideAssetIDs: slides}},
		{name: "missing slide", req: provider.ClipRequest{
			AudioAssetID:  audio,
			SlideAssetIDs: []entity.AssetID{entity.AssetID(strings.Repeat("0", 64))},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := composer.Clip(t.Context(), tc.req); err == nil {
				t.Error("expected an error, got none")
			}
		})
	}
}

// TestFitFontSizeTracksFreeType pins the pure-Go text measurement against the
// only measurement that matters: the one libfreetype makes inside drawtext.
//
// They are different implementations, so they are not asked to agree exactly
// — only closely enough that the chosen size never differs, given that a step
// is two points and around fifty pixels wide at these sizes.
func TestFitFontSizeTracksFreeType(t *testing.T) {
	requireFFmpeg(t)

	composer, _ := newTestComposer(t)
	parsed, err := composer.fonts.load(composer.res.TitleFont)
	if err != nil {
		t.Fatalf("parse font: %v", err)
	}

	titles := []string{
		"SHORT",
		"THE RISE AND FALL",
		"A VERY LONG CHAPTER TITLE THAT KEEPS GOING",
		"WHY THE ROMAN EMPIRE COLLAPSED IN THE FIFTH CENTURY AD",
	}
	for _, title := range titles {
		for _, size := range []int{titleFontMax, 30, titleFontMin} {
			face, err := composer.fonts.face(parsed, size)
			if err != nil {
				t.Fatalf("face at %d: %v", size, err)
			}
			measured := textWidth(face, title)
			rendered := renderedWidth(t, composer.res.TitleFont, title, size)

			// Half a step of tolerance, scaled to the size being measured.
			tolerance := max(4, rendered/50)
			if diff := measured - rendered; diff > tolerance || diff < -tolerance {
				t.Errorf("at %dpt %q measures %dpx, drawtext renders %dpx (diff %+d, tolerance %d)",
					size, title, measured, rendered, diff, tolerance)
			}
		}
	}
}

// renderedWidth draws the text with the same filter the composer uses and
// measures the ink in the resulting frame.
func renderedWidth(t *testing.T, fontFile, text string, size int) int {
	t.Helper()
	out := filepath.Join(t.TempDir(), "text.png")
	filter := fmt.Sprintf("drawtext=fontfile=%s:text='%s':fontsize=%d:fontcolor=white:x=100:y=100",
		escapeFilterPath(fontFile), escapeDrawtext(text), size)

	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=black:s=3000x300:d=0.1",
		"-vf", filter, "-frames:v", "1", out)
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("render text: %v\n%s", err, combined)
	}

	file, err := os.Open(out)
	if err != nil {
		t.Fatalf("open frame: %v", err)
	}
	defer func() { _ = file.Close() }()
	img, err := png.Decode(file)
	if err != nil {
		t.Fatalf("decode frame: %v", err)
	}

	bounds := img.Bounds()
	left, right := bounds.Max.X, bounds.Min.X
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if r, _, _, _ := img.At(x, y).RGBA(); r > 0x2000 {
				left = min(left, x)
				right = max(right, x)
			}
		}
	}
	if right < left {
		t.Fatalf("nothing was drawn for %q at %dpt", text, size)
	}
	return right - left + 1
}
