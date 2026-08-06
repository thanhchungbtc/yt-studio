package app_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// These tests are about one question: what happens when the plan and the graph
// disagree about how many cells there are? The graph wins — it already holds
// one icon task per cell and cannot grow — so a short plan is a roll to take
// again rather than a shape to accept.

// planLLM returns a fixed number of cells, whatever it was asked for.
type planLLM struct {
	store provider.AssetStore
	cells int
	// blankCaption empties one caption, for the rejection case.
	blankCaption bool
	asked        int
}

var _ provider.LLM = (*planLLM)(nil)

func (l *planLLM) Blueprint(context.Context, provider.BlueprintRequest) (provider.Blueprint, error) {
	return provider.Blueprint{}, errors.New("not used")
}

func (l *planLLM) Script(context.Context, provider.ScriptRequest) (provider.Script, error) {
	return provider.Script{}, errors.New("not used")
}

func (l *planLLM) SlidePrompts(context.Context, entity.VideoID) ([]provider.SlidePrompt, error) {
	return nil, errors.New("not used")
}

func (l *planLLM) Metadata(context.Context, provider.MetadataRequest) (provider.Metadata, error) {
	return provider.Metadata{}, errors.New("not used")
}

func (l *planLLM) ThumbnailPlan(ctx context.Context, req provider.ThumbnailPlanRequest) (provider.ThumbnailPlan, error) {
	l.asked = req.Cells
	plan := entity.ThumbnailPlan{Cells: make([]entity.ThumbnailCell, 0, l.cells)}
	for i := range l.cells {
		caption := "Cell " + string(rune('A'+i))
		if l.blankCaption && i == 0 {
			caption = "  "
		}
		plan.Cells = append(plan.Cells, entity.ThumbnailCell{
			Caption: caption,
			Prompt:  "  a pocket watch, side view ",
		})
	}
	stored, err := l.store.Put(ctx, entity.AssetKindThumbnailPlan,
		bytes.NewReader([]byte(req.Headline)))
	if err != nil {
		return provider.ThumbnailPlan{}, err
	}
	return provider.ThumbnailPlan{Plan: plan, AssetID: stored.ID}, nil
}

// planned returns a video with metadata written, which is what the plan task
// reads its headline out of.
func (f *fixture) planned(ref string) entity.Video {
	f.t.Helper()
	v := f.draft(ref, 3, 1)
	if err := f.store.SetVideoMetadata(context.Background(), v.ID, entity.Metadata{
		Title:         "The Long Winter",
		ThumbnailText: "FIFTY BROKEN BELIEFS",
	}); err != nil {
		f.t.Fatalf("SetVideoMetadata: %v", err)
	}
	stored, err := f.store.VideoByID(context.Background(), v.ID)
	if err != nil {
		f.t.Fatalf("VideoByID: %v", err)
	}
	return stored
}

func (f *fixture) planTask(v entity.Video) entity.Task {
	f.t.Helper()
	return entity.Task{
		ID:      entity.NewTaskID(v.ID, entity.TaskKindThumbnailPlan, -1, -1),
		VideoID: v.ID,
		Kind:    entity.TaskKindThumbnailPlan,
		Ordinal: -1,
		Index:   -1,
	}
}

func TestThumbnailPlanFillsTheGridAndIsTidied(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	v := f.planned("DSS-1")
	llm := &planLLM{store: f.assets, cells: testThumbnailCells}

	outcome := app.GenerateThumbnailPlan(ctx, f.planTask(v), f.store, f.store, llm,
		f.store, f.store, f.assets, testTime)
	if _, ok := outcome.(entity.Success); !ok {
		t.Fatalf("outcome = %#v, want Success", outcome)
	}
	if llm.asked != testThumbnailCells {
		t.Fatalf("the model was asked for %d cells, want %d", llm.asked, testThumbnailCells)
	}

	stored, err := f.store.VideoByID(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ThumbnailPlan == nil {
		t.Fatal("the plan was not written to the video")
	}
	if got := len(stored.ThumbnailPlan.Cells); got != testThumbnailCells {
		t.Fatalf("cells = %d, want %d", got, testThumbnailCells)
	}
	// Whitespace is the model's, not ours: the renderer should never have to
	// guess whether a caption was meant to be padded.
	if got := stored.ThumbnailPlan.Cells[0].Prompt; got != "a pocket watch, side view" {
		t.Errorf("prompt = %q, want it trimmed", got)
	}
	// The slots the icons will land in are sized with the plan.
	if got := len(stored.ThumbnailIconAssetIDs); got != testThumbnailCells {
		t.Fatalf("icon slots = %d, want %d", got, testThumbnailCells)
	}
}

// A grid the model came back short on cannot be accepted: icon tasks already
// exist for cells it did not write.
func TestThumbnailPlanShortOfTheGridIsRetried(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	v := f.planned("DSS-1")
	llm := &planLLM{store: f.assets, cells: testThumbnailCells - 1}

	outcome := app.GenerateThumbnailPlan(context.Background(), f.planTask(v),
		f.store, f.store, llm, f.store, f.store, f.assets, testTime)
	failed, ok := outcome.(entity.Failed)
	if !ok {
		t.Fatalf("outcome = %#v, want Failed", outcome)
	}
	if !errors.Is(failed.Err, app.ErrThumbnailPlanOffTarget) {
		t.Fatalf("error = %v, want ErrThumbnailPlanOffTarget", failed.Err)
	}
	// The input was fine and the roll was not, so it is worth another attempt.
	if !failed.Retryable {
		t.Error("a short plan was treated as permanent")
	}
}

// A generous model is cut rather than re-rolled: the surplus costs nothing to
// drop, and re-rolling costs a generation.
func TestThumbnailPlanLongerThanTheGridIsTruncated(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	v := f.planned("DSS-1")
	llm := &planLLM{store: f.assets, cells: testThumbnailCells + 3}

	outcome := app.GenerateThumbnailPlan(ctx, f.planTask(v), f.store, f.store, llm,
		f.store, f.store, f.assets, testTime)
	if _, ok := outcome.(entity.Success); !ok {
		t.Fatalf("outcome = %#v, want Success", outcome)
	}
	stored, err := f.store.VideoByID(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(stored.ThumbnailPlan.Cells); got != testThumbnailCells {
		t.Fatalf("cells = %d, want %d", got, testThumbnailCells)
	}
}

func TestThumbnailPlanRejectsABlankCaption(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	v := f.planned("DSS-1")
	llm := &planLLM{store: f.assets, cells: testThumbnailCells, blankCaption: true}

	outcome := app.GenerateThumbnailPlan(context.Background(), f.planTask(v),
		f.store, f.store, llm, f.store, f.store, f.assets, testTime)
	failed, ok := outcome.(entity.Failed)
	if !ok {
		t.Fatalf("outcome = %#v, want Failed", outcome)
	}
	if !errors.Is(failed.Err, app.ErrThumbnailPlanOffTarget) {
		t.Fatalf("error = %v, want ErrThumbnailPlanOffTarget", failed.Err)
	}
}

// recordingIcons captures what the icon task asked its backend for.
type recordingIcons struct {
	store    provider.AssetStore
	requests []provider.IconRequest
}

func (r *recordingIcons) Generate(ctx context.Context, req provider.IconRequest) (entity.AssetID, error) {
	r.requests = append(r.requests, req)
	stored, err := r.store.Put(ctx, entity.AssetKindThumbnailIcon,
		bytes.NewReader([]byte(req.Prompt)))
	if err != nil {
		return "", err
	}
	return stored.ID, nil
}

// The style clause is joined by the server, not stored in the plan: that is
// what makes restyling the grid cost a dozen cheap generations rather than a
// new set of captions.
func TestThumbnailIconJoinsTheSharedStyleAndLandsInItsSlot(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	v := f.planned("DSS-1")
	llm := &planLLM{store: f.assets, cells: testThumbnailCells}
	if outcome := app.GenerateThumbnailPlan(ctx, f.planTask(v), f.store, f.store, llm,
		f.store, f.store, f.assets, testTime); !isSuccess(outcome) {
		t.Fatalf("plan outcome = %#v", outcome)
	}

	icons := &recordingIcons{store: f.assets}
	opts := app.IconOptions{Style: "white line art on black", Size: 128}

	// Deliberately out of order, as the image pool would return them.
	for _, index := range []int{2, 0, 1, 3} {
		task := entity.Task{
			ID:      entity.NewTaskID(v.ID, entity.TaskKindThumbnailIcon, -1, index),
			VideoID: v.ID,
			Kind:    entity.TaskKindThumbnailIcon,
			Ordinal: -1,
			Index:   index,
		}
		if outcome := app.GenerateThumbnailIcon(ctx, task, f.store, icons,
			f.store, f.store, f.assets, opts, testTime); !isSuccess(outcome) {
			t.Fatalf("icon %d outcome = %#v", index, outcome)
		}
	}

	if got := icons.requests[0].Prompt; got != "a pocket watch, side view — white line art on black" {
		t.Errorf("prompt = %q, want the style appended", got)
	}
	if got := icons.requests[0].Size; got != 128 {
		t.Errorf("size = %d, want 128", got)
	}

	stored, err := f.store.VideoByID(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.ThumbnailIconAssetIDs) != testThumbnailCells {
		t.Fatalf("icons = %v", stored.ThumbnailIconAssetIDs)
	}
	for i, id := range stored.ThumbnailIconAssetIDs {
		if id == "" {
			t.Errorf("cell %d has no icon after every task ran", i)
		}
	}
}

// An icon task for a cell the plan does not have cannot be fixed by trying
// again here: the plan is what has to change.
func TestThumbnailIconRejectsACellThePlanLacks(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	v := f.planned("DSS-1")
	llm := &planLLM{store: f.assets, cells: testThumbnailCells}
	if outcome := app.GenerateThumbnailPlan(ctx, f.planTask(v), f.store, f.store, llm,
		f.store, f.store, f.assets, testTime); !isSuccess(outcome) {
		t.Fatalf("plan outcome = %#v", outcome)
	}

	task := entity.Task{
		ID:      entity.NewTaskID(v.ID, entity.TaskKindThumbnailIcon, -1, testThumbnailCells),
		VideoID: v.ID,
		Kind:    entity.TaskKindThumbnailIcon,
		Ordinal: -1,
		Index:   testThumbnailCells,
	}
	outcome := app.GenerateThumbnailIcon(ctx, task, f.store, &recordingIcons{store: f.assets},
		f.store, f.store, f.assets, app.IconOptions{}, testTime)
	failed, ok := outcome.(entity.Failed)
	if !ok {
		t.Fatalf("outcome = %#v, want Failed", outcome)
	}
	if failed.Retryable {
		t.Error("an out-of-range cell was treated as transient")
	}
}

func isSuccess(o entity.TaskOutcome) bool {
	_, ok := o.(entity.Success)
	return ok
}
