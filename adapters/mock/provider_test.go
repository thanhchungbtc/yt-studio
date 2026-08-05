package mockprovider_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"image/png"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/tbui/yt-studio/adapters/assetstore"
	mockprovider "github.com/tbui/yt-studio/adapters/mock"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

func newStore(t *testing.T) *assetstore.FS {
	t.Helper()
	store, err := assetstore.New(t.TempDir())
	if err != nil {
		t.Fatalf("assetstore.New: %v", err)
	}
	return store
}

func blueprintRequest(chapters int) provider.BlueprintRequest {
	return provider.BlueprintRequest{
		VideoID:      "v1",
		VideoRef:     "DSS-1",
		ChannelSlug:  "deep-sleep-stories",
		Title:        "The Long Winter",
		Topic:        "a northern port town",
		ChapterCount: chapters,
	}
}

func lookupFor(bp provider.Blueprint, slides int) mockprovider.ContextLookup {
	return func(context.Context, entity.VideoID) (mockprovider.VideoContext, error) {
		return mockprovider.VideoContext{
			Ref:              "DSS-1",
			Title:            bp.Title,
			Topic:            "a northern port town",
			Chapters:         bp.Chapters,
			SlidesPerChapter: slides,
		}, nil
	}
}

// The same inputs must always produce the same bytes, or golden-file tests and
// content addressing both stop meaning anything.
func TestProvidersAreDeterministic(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	run := func() (entity.AssetID, entity.AssetID, entity.AssetID, entity.AssetID) {
		store := newStore(t)
		llm := mockprovider.NewLLM(store, nil, nil)
		bp, err := llm.Blueprint(ctx, blueprintRequest(3))
		if err != nil {
			t.Fatal(err)
		}
		// The blueprint's own output is the outline a script is written inside,
		// so it goes straight across with nothing to translate.
		script, err := llm.Script(ctx, provider.ScriptRequest{
			VideoID: "v1", ChapterID: "v1:ch:1", Ordinal: 1,
			Blueprint: bp.BlueprintOutline,
		})
		if err != nil {
			t.Fatal(err)
		}
		tts := mockprovider.NewTTS(store, nil)
		audio, err := tts.Speak(ctx, provider.SpeakRequest{
			VideoID: "v1", ChapterID: "v1:ch:1", Ordinal: 1, Text: script.Text,
		})
		if err != nil {
			t.Fatal(err)
		}
		gen := mockprovider.NewSlide(store, nil)
		slide, err := gen.Generate(ctx, provider.SlideRequest{
			VideoID: "v1", ChapterID: "v1:ch:1", Ordinal: 1, Index: 0,
			Prompt: "a wide harbour at low tide",
		})
		if err != nil {
			t.Fatal(err)
		}
		return bp.AssetID, script.AssetID, audio, slide
	}

	a1, a2, a3, a4 := run()
	b1, b2, b3, b4 := run()
	if a1 != b1 || a2 != b2 || a3 != b3 || a4 != b4 {
		t.Fatalf("content addresses differ between identical runs:\n%v %v %v %v\n%v %v %v %v",
			a1, a2, a3, a4, b1, b2, b3, b4)
	}
}

// Content addressing means a second identical write is a no-op.
func TestIdenticalOutputReusesTheStoredFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)
	llm := mockprovider.NewLLM(store, nil, nil)

	first, err := llm.Blueprint(ctx, blueprintRequest(2))
	if err != nil {
		t.Fatal(err)
	}
	second, err := llm.Blueprint(ctx, blueprintRequest(2))
	if err != nil {
		t.Fatal(err)
	}
	if first.AssetID != second.AssetID {
		t.Fatalf("asset ids differ: %v vs %v", first.AssetID, second.AssetID)
	}
	stored, err := store.Stat(ctx, first.AssetID, entity.AssetKindBlueprint)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.Existed {
		t.Fatal("Stat did not report the file as already present")
	}
}

func TestBlueprintProducesTheRequestedChapters(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	llm := mockprovider.NewLLM(newStore(t), nil, nil)

	for _, n := range []int{1, 7, 50} {
		bp, err := llm.Blueprint(ctx, blueprintRequest(n))
		if err != nil {
			t.Fatal(err)
		}
		if len(bp.Chapters) != n {
			t.Fatalf("chapters = %d, want %d", len(bp.Chapters), n)
		}
		for i, c := range bp.Chapters {
			if c.Ordinal != i+1 {
				t.Fatalf("chapter %d has ordinal %d", i, c.Ordinal)
			}
			if c.Title == "" || c.Summary == "" {
				t.Fatalf("chapter %d is incomplete: %+v", i, c)
			}
		}
	}
}

// All prompts come from one production; concurrent callers get their own slice
// from the cache.
func TestSlidePromptsAreCoalesced(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)

	const chapters, slides = 6, 2
	seed := mockprovider.NewLLM(store, nil, nil)
	bp, err := seed.Blueprint(ctx, blueprintRequest(chapters))
	if err != nil {
		t.Fatal(err)
	}

	var lookups int
	llm := mockprovider.NewLLM(store, func(ctx context.Context, id entity.VideoID) (mockprovider.VideoContext, error) {
		lookups++
		return lookupFor(bp, slides)(ctx, id)
	}, nil)

	first, err := llm.SlidePrompts(ctx, "v1")
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != chapters*slides {
		t.Fatalf("prompts = %d, want %d", len(first), chapters*slides)
	}
	for range 20 {
		again, err := llm.SlidePrompts(ctx, "v1")
		if err != nil {
			t.Fatal(err)
		}
		if len(again) != len(first) {
			t.Fatalf("cached prompts = %d, want %d", len(again), len(first))
		}
		// Each caller must own its slice.
		if &again[0] == &first[0] {
			t.Fatal("the cached slice was handed out directly")
		}
	}
	if lookups != 1 {
		t.Fatalf("the batch was produced %d times, want exactly 1", lookups)
	}

	llm.Forget("v1")
	if _, err := llm.SlidePrompts(ctx, "v1"); err != nil {
		t.Fatal(err)
	}
	if lookups != 2 {
		t.Fatalf("Forget did not invalidate the batch: lookups = %d", lookups)
	}
}

func TestGeneratedStillIsAValidPNG(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)
	gen := mockprovider.NewSlide(store, nil)

	id, err := gen.Generate(ctx, provider.SlideRequest{
		VideoID: "v1", ChapterID: "v1:ch:1", Ordinal: 1, Index: 0, Prompt: "a stone bridge in thin fog",
	})
	if err != nil {
		t.Fatal(err)
	}
	rc, err := store.Open(ctx, id, entity.AssetKindImage)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()

	img, err := png.Decode(rc)
	if err != nil {
		t.Fatalf("output is not a valid PNG: %v", err)
	}
	if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
		t.Fatal("PNG has no pixels")
	}
}

// grid builds the cells a thumbnail is assembled from: one generated icon per
// caption, which is the shape the icon tasks will hand the builder.
func grid(t *testing.T, store provider.AssetStore, captions ...string) []provider.ThumbnailIconCell {
	t.Helper()
	icons := mockprovider.NewIcon(store, nil)
	cells := make([]provider.ThumbnailIconCell, 0, len(captions))
	for i, caption := range captions {
		id, err := icons.Icon(context.Background(), provider.ThumbnailIconRequest{
			VideoID: "v1", Index: i, Prompt: "a stone archway, side view — " + caption, Size: 256,
		})
		if err != nil {
			t.Fatal(err)
		}
		cells = append(cells, provider.ThumbnailIconCell{Caption: caption, IconAssetID: id})
	}
	return cells
}

// The thumbnail is the one asset whose dimensions are fixed by YouTube rather
// than by us, so the mock is held to them.
func TestThumbnailIsAValidPNGAtYouTubeSize(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)
	thumbnails := mockprovider.NewThumbnail(store, nil)
	cells := grid(t, store, "Mind Control", "Split Personality", "Inner Critic", "False Memory")

	id, err := thumbnails.Build(ctx, provider.ThumbnailRequest{
		VideoID: "v1", VideoRef: "DSS-1", Title: "The Long Winter",
		Headline: "50 BROKEN BELIEFS", Cells: cells,
	})
	if err != nil {
		t.Fatal(err)
	}
	rc, err := store.Open(ctx, id, entity.AssetKindThumbnail)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()

	img, err := png.Decode(rc)
	if err != nil {
		t.Fatalf("output is not a valid PNG: %v", err)
	}
	if got := img.Bounds(); got.Dx() != 1280 || got.Dy() != 720 {
		t.Fatalf("thumbnail is %dx%d, want 1280x720", got.Dx(), got.Dy())
	}

	// The headline is the reason the task exists: a thumbnail built without it
	// must not land on the same content address as one built with it.
	plain, err := thumbnails.Build(ctx, provider.ThumbnailRequest{
		VideoID: "v1", VideoRef: "DSS-1", Title: "The Long Winter", Cells: cells,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plain == id {
		t.Fatal("the headline made no difference to the rendered bytes")
	}
}

// Every cell is drawn, so changing one caption changes the image. Without this
// the grid could silently be rendering the same tile ten times.
func TestThumbnailRendersEveryCell(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)
	thumbnails := mockprovider.NewThumbnail(store, nil)

	base := provider.ThumbnailRequest{
		VideoID: "v1", VideoRef: "DSS-1", Headline: "REAL CHEAT CODES",
		Cells: grid(t, store, "Mirroring", "Give First", "Speak Slow", "Smile First"),
	}
	first, err := thumbnails.Build(ctx, base)
	if err != nil {
		t.Fatal(err)
	}

	// Only the last cell's caption differs.
	changed := base
	changed.Cells = append([]provider.ThumbnailIconCell(nil), base.Cells...)
	changed.Cells[len(changed.Cells)-1].Caption = "Smile Very Much First"
	second, err := thumbnails.Build(ctx, changed)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("a caption change in the last cell did not reach the image")
	}
}

// A cell whose icon never landed is a caller error, not a hole in the grid.
func TestThumbnailRejectsACellWithNoIcon(t *testing.T) {
	t.Parallel()
	thumbnails := mockprovider.NewThumbnail(newStore(t), nil)

	if _, err := thumbnails.Build(context.Background(), provider.ThumbnailRequest{
		VideoID: "v1", VideoRef: "DSS-1", Headline: "50 BROKEN BELIEFS",
		Cells: []provider.ThumbnailIconCell{{Caption: "Mind Control"}},
	}); err == nil {
		t.Fatal("Build with an iconless cell returned no error")
	}
}

// The icons carry the whole look of the grid, so they are held to being square,
// black-backed and reproducible.
func TestIconIsASquarePNG(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)
	icons := mockprovider.NewIcon(store, nil)

	req := provider.ThumbnailIconRequest{
		VideoID: "v1", Index: 3, Prompt: "a pocket watch, side view", Size: 256,
	}
	id, err := icons.Icon(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := store.Open(ctx, id, entity.AssetKindThumbnailIcon)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()

	img, err := png.Decode(rc)
	if err != nil {
		t.Fatalf("output is not a valid PNG: %v", err)
	}
	if got := img.Bounds(); got.Dx() != 256 || got.Dy() != 256 {
		t.Fatalf("icon is %dx%d, want 256x256", got.Dx(), got.Dy())
	}

	// The index is not part of the seed: two cells that asked for the same thing
	// are the same file, and content addressing should say so.
	elsewhere := req
	elsewhere.Index = 7
	same, err := icons.Icon(ctx, elsewhere)
	if err != nil {
		t.Fatal(err)
	}
	if same != id {
		t.Fatal("the same prompt at a different index produced different bytes")
	}

	different, err := icons.Icon(ctx, provider.ThumbnailIconRequest{
		VideoID: "v1", Index: 3, Prompt: "a coil of rope, top-down view", Size: 256,
	})
	if err != nil {
		t.Fatal(err)
	}
	if different == id {
		t.Fatal("a different prompt produced the same icon")
	}
}

// The plan is a hard contract on count: the DAG already holds one icon task per
// cell by the time it runs.
func TestThumbnailPlanFillsEveryCell(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	llm := mockprovider.NewLLM(newStore(t), nil, nil)

	bp, err := llm.Blueprint(ctx, blueprintRequest(12))
	if err != nil {
		t.Fatal(err)
	}
	req := provider.ThumbnailPlanRequest{
		VideoID: "v1", VideoRef: "DSS-1", Title: bp.Title,
		Headline: "50 BROKEN BELIEFS", Chapters: bp.Chapters, Cells: 10,
	}
	plan, err := llm.ThumbnailPlan(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Plan.Cells) != 10 {
		t.Fatalf("cells = %d, want exactly 10", len(plan.Plan.Cells))
	}
	for i, cell := range plan.Plan.Cells {
		if cell.Caption == "" {
			t.Errorf("cell %d has no caption", i)
		}
		if words := len(strings.Fields(cell.Caption)); words > 3 {
			t.Errorf("cell %d caption %q is %d words, want at most 3", i, cell.Caption, words)
		}
		if cell.Prompt == "" {
			t.Errorf("cell %d has no prompt", i)
		}
	}

	// The cells are drawn from across the video, not off the front of it: a grid
	// taken from the first ten chapters of fifty is a bug that looks like output.
	if plan.Plan.Cells[0].Caption == plan.Plan.Cells[len(plan.Plan.Cells)-1].Caption {
		t.Error("first and last cell came from the same chapter")
	}

	again, err := llm.ThumbnailPlan(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if again.AssetID != plan.AssetID {
		t.Fatal("the same request produced a different plan")
	}
}

// A plan with nothing to draw from cannot be invented, and saying so beats
// returning ten empty tiles.
func TestThumbnailPlanNeedsAnOutline(t *testing.T) {
	t.Parallel()
	llm := mockprovider.NewLLM(newStore(t), nil, nil)

	if _, err := llm.ThumbnailPlan(context.Background(), provider.ThumbnailPlanRequest{
		VideoID: "v1", Headline: "50 BROKEN BELIEFS", Cells: 10,
	}); err == nil {
		t.Fatal("ThumbnailPlan with no chapters returned no error")
	}
}

func TestGeneratedAudioIsAValidWAV(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)
	tts := mockprovider.NewTTS(store, nil)

	id, err := tts.Speak(ctx, provider.SpeakRequest{
		VideoID: "v1", ChapterID: "v1:ch:1", Ordinal: 1, Text: "hello there",
	})
	if err != nil {
		t.Fatal(err)
	}
	rc, err := store.Open(ctx, id, entity.AssetKindAudio)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()

	raw, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 44 {
		t.Fatalf("WAV is %d bytes, shorter than its own header", len(raw))
	}
	if string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" || string(raw[12:16]) != "fmt " {
		t.Fatalf("bad RIFF header: %q", raw[0:16])
	}
	declared := binary.LittleEndian.Uint32(raw[4:8])
	if int(declared) != len(raw)-8 {
		t.Fatalf("RIFF size = %d, want %d", declared, len(raw)-8)
	}
	if got := binary.LittleEndian.Uint16(raw[20:22]); got != 1 {
		t.Fatalf("audio format = %d, want 1 (PCM)", got)
	}
	if string(raw[36:40]) != "data" {
		t.Fatalf("expected a data chunk at offset 36, got %q", raw[36:40])
	}
}

// The composer must produce a structurally valid MP4 whose boxes tile the file
// exactly, and concat must be a stream copy of its inputs.
func TestComposedMP4IsStructurallyValid(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)
	tts := mockprovider.NewTTS(store, nil)
	gen := mockprovider.NewSlide(store, nil)
	composer := mockprovider.NewComposer(store, nil)

	makeClip := func(ordinal int) entity.AssetID {
		audio, err := tts.Speak(ctx, provider.SpeakRequest{
			VideoID: "v1", ChapterID: entity.NewChapterID("v1", ordinal), Ordinal: ordinal,
			Text: "narration for chapter",
		})
		if err != nil {
			t.Fatal(err)
		}
		slides := make([]entity.AssetID, 0, 2)
		for j := range 2 {
			id, err := gen.Generate(ctx, provider.SlideRequest{
				VideoID: "v1", ChapterID: entity.NewChapterID("v1", ordinal),
				Ordinal: ordinal, Index: j, Prompt: "a river bend seen from a ridge",
			})
			if err != nil {
				t.Fatal(err)
			}
			slides = append(slides, id)
		}
		clip, err := composer.Clip(ctx, provider.ClipRequest{
			VideoID: "v1", ChapterID: entity.NewChapterID("v1", ordinal), Ordinal: ordinal,
			AudioAssetID: audio, SlideAssetIDs: slides,
		})
		if err != nil {
			t.Fatalf("Clip: %v", err)
		}
		return clip
	}

	clips := []entity.AssetID{makeClip(1), makeClip(2), makeClip(3)}
	for _, clip := range clips {
		assertBoxesTileFile(t, ctx, store, clip, entity.AssetKindClip)
	}

	final, err := composer.Concat(ctx, provider.ConcatRequest{VideoID: "v1", ClipAssetIDs: clips})
	if err != nil {
		t.Fatalf("Concat: %v", err)
	}
	assertBoxesTileFile(t, ctx, store, final, entity.AssetKindFinal)

	// The final render must be at least as large as the payload of its inputs,
	// which is what a stream copy guarantees.
	var clipTotal int64
	for _, clip := range clips {
		info, err := store.Stat(ctx, clip, entity.AssetKindClip)
		if err != nil {
			t.Fatal(err)
		}
		clipTotal += info.Size
	}
	finalInfo, err := store.Stat(ctx, final, entity.AssetKindFinal)
	if err != nil {
		t.Fatal(err)
	}
	if finalInfo.Size < clipTotal/2 {
		t.Fatalf("final render is %d bytes against %d bytes of clips; payload was lost",
			finalInfo.Size, clipTotal)
	}
}

// assertBoxesTileFile walks the top-level ISO boxes and checks they cover the
// file exactly, which is the property a truncated or misdeclared write breaks.
func assertBoxesTileFile(t *testing.T, ctx context.Context, store *assetstore.FS, id entity.AssetID, kind entity.AssetKind) {
	t.Helper()
	rc, err := store.Open(ctx, id, kind)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	raw, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	offset := 0
	for offset < len(raw) {
		if offset+8 > len(raw) {
			t.Fatalf("%s: truncated box header at offset %d", kind, offset)
		}
		size := int(binary.BigEndian.Uint32(raw[offset : offset+4]))
		name := string(raw[offset+4 : offset+8])
		if size < 8 || offset+size > len(raw) {
			t.Fatalf("%s: box %q declares size %d at offset %d of %d bytes", kind, name, size, offset, len(raw))
		}
		seen[name] = true
		offset += size
	}
	if offset != len(raw) {
		t.Fatalf("%s: boxes cover %d bytes of a %d byte file", kind, offset, len(raw))
	}
	for _, want := range []string{"ftyp", "mdat", "moov"} {
		if !seen[want] {
			t.Fatalf("%s: no %q box", kind, want)
		}
	}
}

func TestUploaderProducesAStableReceipt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)
	stored, err := store.Put(ctx, entity.AssetKindFinal, bytes.NewReader([]byte("not really an mp4, but real bytes")))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Unix(1_700_000_000, 0).UTC()
	uploader := mockprovider.NewUploader(store, nil, func() time.Time { return at })

	req := provider.UploadRequest{
		VideoID: "v1", VideoRef: "DSS-1", ChannelSlug: "deep-sleep-stories",
		FinalAssetID: stored.ID, DryRun: true,
	}
	first, err := uploader.Upload(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := uploader.Upload(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("receipts differ across identical uploads: %+v vs %+v", first, second)
	}
	if !first.DryRun {
		t.Fatal("dry run must stay the default")
	}
	if first.URL == "" || first.VideoID == "" {
		t.Fatalf("incomplete receipt: %+v", first)
	}
}

// A cancelled video's provider calls must stop rather than run to completion.
func TestProvidersRespectContextCancellation(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	slow := mockprovider.Tuning(func() (time.Duration, int) { return time.Hour, 0 })
	llm := mockprovider.NewLLM(store, nil, slow)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := llm.Blueprint(ctx, blueprintRequest(2)); err == nil {
		t.Fatal("Blueprint ignored a cancelled context")
	}
}

func TestFailureInjectionIsExercised(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	always := mockprovider.Tuning(func() (time.Duration, int) { return 0, 100 })
	llm := mockprovider.NewLLM(store, nil, always)

	if _, err := llm.Blueprint(context.Background(), blueprintRequest(2)); err == nil {
		t.Fatal("a 100% failure rate produced no error")
	}
}
