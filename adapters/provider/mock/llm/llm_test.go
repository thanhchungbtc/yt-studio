package llm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tbui/yt-studio/adapters/assetstore"
	"github.com/tbui/yt-studio/adapters/provider/mock/llm"
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

func lookupFor(bp provider.Blueprint, slides int) llm.ContextLookup {
	return func(context.Context, entity.VideoID) (llm.VideoContext, error) {
		return llm.VideoContext{
			Ref:              "DSS-1",
			Title:            bp.Title,
			Topic:            "a northern port town",
			Chapters:         bp.Chapters,
			SlidesPerChapter: slides,
		}, nil
	}
}

// Content addressing means a second identical write is a no-op.
func TestIdenticalOutputReusesTheStoredFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newStore(t)
	llm := llm.NewLLM(store, nil)

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
	llm := llm.NewLLM(newStore(t), nil)

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
	seed := llm.NewLLM(store, nil)
	bp, err := seed.Blueprint(ctx, blueprintRequest(chapters))
	if err != nil {
		t.Fatal(err)
	}

	var lookups int
	llm := llm.NewLLM(store, func(ctx context.Context, id entity.VideoID) (llm.VideoContext, error) {
		lookups++
		return lookupFor(bp, slides)(ctx, id)
	})

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

// The plan is a hard contract on count: the DAG already holds one icon task per
// cell by the time it runs.
func TestThumbnailPlanFillsEveryCell(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	llm := llm.NewLLM(newStore(t), nil)

	bp, err := llm.Blueprint(ctx, blueprintRequest(12))
	if err != nil {
		t.Fatal(err)
	}
	req := provider.ThumbnailPlanRequest{
		VideoID: "v1", VideoRef: "DSS-1", Blueprint: bp.BlueprintOutline,
		Headline: "50 BROKEN BELIEFS", Cells: 10,
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
	llm := llm.NewLLM(newStore(t), nil)

	if _, err := llm.ThumbnailPlan(context.Background(), provider.ThumbnailPlanRequest{
		VideoID: "v1", Headline: "50 BROKEN BELIEFS", Cells: 10,
	}); err == nil {
		t.Fatal("ThumbnailPlan with no chapters returned no error")
	}
}
