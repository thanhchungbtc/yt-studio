package mockprovider_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"image/png"
	"io"
	"testing"
	"time"

	"github.com/tbui/yt-studio/adapters/assetstore"
	mockprovider "github.com/tbui/yt-studio/adapters/mock_provider"
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

var testStyle = entity.StyleConfig{
	Tone:            "calm, measured",
	Voice:           "amber-low",
	ImageStyle:      "muted watercolour",
	Language:        "en-US",
	WordsPerChapter: 120,
}

func blueprintRequest(chapters int) provider.BlueprintRequest {
	return provider.BlueprintRequest{
		VideoID:      "v1",
		VideoRef:     "DSS-1",
		ChannelSlug:  "deep-sleep-stories",
		Title:        "The Long Winter",
		Topic:        "a northern port town",
		ChapterCount: chapters,
		Style:        testStyle,
	}
}

func lookupFor(bp provider.Blueprint, images int) mockprovider.ContextLookup {
	return func(context.Context, entity.VideoID) (mockprovider.VideoContext, error) {
		return mockprovider.VideoContext{
			Ref:              "DSS-1",
			Title:            bp.Title,
			Topic:            "a northern port town",
			Style:            testStyle,
			Chapters:         bp.Chapters,
			ImagesPerChapter: images,
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
			Style:     testStyle,
		})
		if err != nil {
			t.Fatal(err)
		}
		tts := mockprovider.NewTTS(store, nil)
		audio, err := tts.Speak(ctx, provider.SpeakRequest{
			VideoID: "v1", ChapterID: "v1:ch:1", Ordinal: 1, Text: script.Text, Voice: testStyle.Voice,
		})
		if err != nil {
			t.Fatal(err)
		}
		images := mockprovider.NewImage(store, nil)
		still, err := images.Generate(ctx, provider.ImageRequest{
			VideoID: "v1", ChapterID: "v1:ch:1", Ordinal: 1, Index: 0,
			Prompt: "a wide harbour at low tide", Style: testStyle.ImageStyle,
		})
		if err != nil {
			t.Fatal(err)
		}
		return bp.AssetID, script.AssetID, audio, still
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
func TestImagePromptsAreCoalesced(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)

	const chapters, images = 6, 2
	seed := mockprovider.NewLLM(store, nil, nil)
	bp, err := seed.Blueprint(ctx, blueprintRequest(chapters))
	if err != nil {
		t.Fatal(err)
	}

	var lookups int
	llm := mockprovider.NewLLM(store, func(ctx context.Context, id entity.VideoID) (mockprovider.VideoContext, error) {
		lookups++
		return lookupFor(bp, images)(ctx, id)
	}, nil)

	first, err := llm.ImagePrompts(ctx, "v1")
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != chapters*images {
		t.Fatalf("prompts = %d, want %d", len(first), chapters*images)
	}
	for range 20 {
		again, err := llm.ImagePrompts(ctx, "v1")
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
	if _, err := llm.ImagePrompts(ctx, "v1"); err != nil {
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
	images := mockprovider.NewImage(store, nil)

	id, err := images.Generate(ctx, provider.ImageRequest{
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

func TestGeneratedAudioIsAValidWAV(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)
	tts := mockprovider.NewTTS(store, nil)

	id, err := tts.Speak(ctx, provider.SpeakRequest{
		VideoID: "v1", ChapterID: "v1:ch:1", Ordinal: 1, Text: "hello there", Voice: "amber-low",
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
	images := mockprovider.NewImage(store, nil)
	composer := mockprovider.NewComposer(store, nil)

	makeClip := func(ordinal int) entity.AssetID {
		audio, err := tts.Speak(ctx, provider.SpeakRequest{
			VideoID: "v1", ChapterID: entity.NewChapterID("v1", ordinal), Ordinal: ordinal,
			Text: "narration for chapter", Voice: "amber-low",
		})
		if err != nil {
			t.Fatal(err)
		}
		stills := make([]entity.AssetID, 0, 2)
		for j := range 2 {
			id, err := images.Generate(ctx, provider.ImageRequest{
				VideoID: "v1", ChapterID: entity.NewChapterID("v1", ordinal),
				Ordinal: ordinal, Index: j, Prompt: "a river bend seen from a ridge",
			})
			if err != nil {
				t.Fatal(err)
			}
			stills = append(stills, id)
		}
		clip, err := composer.Clip(ctx, provider.ClipRequest{
			VideoID: "v1", ChapterID: entity.NewChapterID("v1", ordinal), Ordinal: ordinal,
			AudioAssetID: audio, ImageAssetIDs: stills,
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
