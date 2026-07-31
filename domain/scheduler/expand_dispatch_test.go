package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
)

// newExpandingScheduler wires a scheduler whose blueprint task expands its own
// video's DAG, which is what the ungated production path does.
//
// chaptersReturned is what the "model" comes back with, deliberately unequal to
// anything the video was briefed with: the whole point of the two-phase build
// is that the DAG is shaped by the outline rather than by the brief.
func newExpandingScheduler(t *testing.T, videoID entity.VideoID, chaptersReturned, images int) (*Scheduler, *recordingStore, *noopLifecycle) {
	t.Helper()
	store := newRecordingStore()
	lifecycle := newNoopLifecycle()
	pools, err := NewPools(map[entity.Pool]int{})
	if err != nil {
		t.Fatalf("NewPools: %v", err)
	}

	var s *Scheduler
	runner := funcRunner{fn: func(ctx context.Context, task entity.Task) entity.TaskOutcome {
		if task.Kind != entity.TaskKindBlueprint {
			return entity.Success{}
		}
		tail, err := BuildTail(BuildSpec{
			VideoID:          videoID,
			ChapterCount:     chaptersReturned,
			ImagesPerChapter: images,
			ThumbnailCells:   testCells,
			MaxAttempts:      3,
			Now:              time.Unix(0, 0).UTC(),
		})
		if err != nil {
			return entity.Failed{Err: err}
		}
		if err := s.Expand(ctx, videoID, tail); err != nil {
			return entity.Failed{Err: err, Retryable: true}
		}
		return entity.Success{}
	}}

	s = New(pools, store, runner, lifecycle, nil, discardLogger(), Config{
		RetryBase: time.Millisecond, RetryMax: 5 * time.Millisecond,
	})
	return s, store, lifecycle
}

func submitHead(t *testing.T, ctx context.Context, s *Scheduler, videoID entity.VideoID, gate bool) {
	t.Helper()
	head, err := BuildHeadGraph(HeadSpec{
		VideoID: videoID, MaxAttempts: 3, BlueprintGate: gate, Now: time.Unix(0, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("BuildHeadGraph: %v", err)
	}
	if err := s.Submit(ctx, head); err != nil {
		t.Fatalf("Submit: %v", err)
	}
}

// The end-to-end shape of the change: a video briefed for one chapter count
// runs to completion with the count its blueprint actually returned.
func TestUngatedBlueprintExpandsAndRunsToCompletion(t *testing.T) {
	t.Parallel()
	const chapters, images = 7, 2
	s, store, lifecycle := newExpandingScheduler(t, "v1", chapters, images)
	ctx := startScheduler(t, s)
	submitHead(t, ctx, s, "v1", false)

	waitFor(t, 5*time.Second, "the video to complete", func() bool {
		return lifecycle.state("v1") == entity.VideoStateCompleted
	})

	// Every node of the expanded DAG ran, and the durable view agrees.
	persisted := store.persisted("v1")
	if len(persisted.Tasks) != NodeCountFor(chapters, images, testCells) {
		t.Fatalf("persisted tasks = %d, want %d", len(persisted.Tasks), NodeCountFor(chapters, images, testCells))
	}
	for _, task := range persisted.Tasks {
		if task.State != entity.TaskStateSucceeded {
			t.Errorf("%s is %s, want succeeded", task.ID, task.State)
		}
	}
	// And it survives a restart as one graph.
	restored, err := GraphFromPersisted(persisted)
	if err != nil {
		t.Fatalf("GraphFromPersisted: %v", err)
	}
	if restored.NodeCount() != NodeCountFor(chapters, images, testCells) {
		t.Fatalf("restored node count = %d, want %d", restored.NodeCount(), NodeCountFor(chapters, images, testCells))
	}
}

// With the gate on, nothing below the blueprint exists until the operator
// approves — and the expansion the approval performs is what releases it.
func TestGatedBlueprintParksBeforeExpanding(t *testing.T) {
	t.Parallel()
	const chapters, images = 4, 2
	store := newRecordingStore()
	lifecycle := newNoopLifecycle()
	pools, err := NewPools(map[entity.Pool]int{})
	if err != nil {
		t.Fatalf("NewPools: %v", err)
	}
	s := New(pools, store, funcRunner{fn: func(context.Context, entity.Task) entity.TaskOutcome {
		return entity.Success{}
	}}, lifecycle, nil, discardLogger(), Config{})
	ctx := startScheduler(t, s)
	submitHead(t, ctx, s, "v1", true)

	blueprint := entity.NewTaskID("v1", entity.TaskKindBlueprint, -1, -1)
	waitFor(t, 5*time.Second, "the blueprint gate to open", func() bool {
		return store.state(blueprint) == entity.TaskStateAwaitingApproval
	})
	if lifecycle.state("v1") != entity.VideoStateAwaitingApproval {
		t.Fatalf("video state = %q, want awaiting_approval", lifecycle.state("v1"))
	}
	if got := len(store.persisted("v1").Tasks); got != 1 {
		t.Fatalf("persisted tasks at the gate = %d, want 1", got)
	}

	// Approval expands first, then releases: the reverse order would let a video
	// whose whole DAG is one succeeded blueprint report itself completed.
	tail, err := BuildTail(BuildSpec{
		VideoID: "v1", ChapterCount: chapters, ImagesPerChapter: images,
		ThumbnailCells: testCells, MaxAttempts: 3,
		Now: time.Unix(0, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("BuildTail: %v", err)
	}
	if err := s.Expand(ctx, "v1", tail); err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if err := s.Approve(ctx, blueprint); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	waitFor(t, 5*time.Second, "the video to complete", func() bool {
		return lifecycle.state("v1") == entity.VideoStateCompleted
	})
	if got := len(store.persisted("v1").Tasks); got != NodeCountFor(chapters, images, testCells) {
		t.Fatalf("persisted tasks = %d, want %d", got, NodeCountFor(chapters, images, testCells))
	}
}

// Expansion is idempotent for the same shape — an approval retried after a
// partial failure must converge — and refuses a different one, because that
// means the two were computed from different chapter sets.
func TestExpandIsIdempotentForTheSameShape(t *testing.T) {
	t.Parallel()
	store := newRecordingStore()
	pools, err := NewPools(map[entity.Pool]int{})
	if err != nil {
		t.Fatalf("NewPools: %v", err)
	}
	// A runner that never returns keeps the blueprint in flight, so the graph
	// stays expandable for the duration of the test.
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	s := New(pools, store, funcRunner{fn: func(ctx context.Context, _ entity.Task) entity.TaskOutcome {
		select {
		case <-block:
		case <-ctx.Done():
		}
		return entity.Failed{Err: errors.New("abandoned"), Retryable: false}
	}}, newNoopLifecycle(), nil, discardLogger(), Config{})
	ctx := startScheduler(t, s)
	submitHead(t, ctx, s, "v1", false)

	blueprint := entity.NewTaskID("v1", entity.TaskKindBlueprint, -1, -1)
	waitFor(t, 5*time.Second, "the blueprint to start", func() bool {
		return store.state(blueprint) == entity.TaskStateRunning
	})

	spec := BuildSpec{VideoID: "v1", ChapterCount: 5, ImagesPerChapter: 2,
		ThumbnailCells: testCells, MaxAttempts: 3, Now: time.Unix(0, 0).UTC()}
	tail, err := BuildTail(spec)
	if err != nil {
		t.Fatalf("BuildTail: %v", err)
	}
	if err := s.Expand(ctx, "v1", tail); err != nil {
		t.Fatalf("first Expand: %v", err)
	}
	if err := s.Expand(ctx, "v1", tail); err != nil {
		t.Fatalf("repeated Expand of the same shape: %v", err)
	}

	spec.ChapterCount = 6
	other, err := BuildTail(spec)
	if err != nil {
		t.Fatalf("BuildTail: %v", err)
	}
	if err := s.Expand(ctx, "v1", other); !errors.Is(err, ErrAlreadyExpanded) {
		t.Fatalf("Expand with a different shape = %v, want ErrAlreadyExpanded", err)
	}
}

func TestExpandRejectsUnknownVideo(t *testing.T) {
	t.Parallel()
	s, _, _ := newExpandingScheduler(t, "v1", 3, 2)
	ctx := startScheduler(t, s)
	tail, err := BuildTail(BuildSpec{VideoID: "ghost", ChapterCount: 2, ImagesPerChapter: 1,
		ThumbnailCells: testCells, MaxAttempts: 1})
	if err != nil {
		t.Fatalf("BuildTail: %v", err)
	}
	if err := s.Expand(ctx, "ghost", tail); !errors.Is(err, ErrUnknownVideo) {
		t.Fatalf("Expand = %v, want ErrUnknownVideo", err)
	}
}

// The whole DAG below a blueprint is built from the chapters it produced, and
// expansion is one-way. Re-running an accepted outline would leave tasks
// addressing chapters that no longer exist.
func TestBlueprintCannotBeRerunOnceAccepted(t *testing.T) {
	t.Parallel()
	s, store, lifecycle := newExpandingScheduler(t, "v1", 3, 2)
	ctx := startScheduler(t, s)
	submitHead(t, ctx, s, "v1", false)
	waitFor(t, 5*time.Second, "the video to complete", func() bool {
		return lifecycle.state("v1") == entity.VideoStateCompleted
	})

	blueprint := entity.NewTaskID("v1", entity.TaskKindBlueprint, -1, -1)
	if err := s.RetryTask(ctx, blueprint); !errors.Is(err, ErrBlueprintLocked) {
		t.Fatalf("RetryTask(blueprint) = %v, want ErrBlueprintLocked", err)
	}
	if err := s.RetryChapter(ctx, "v1", -1); !errors.Is(err, ErrBlueprintLocked) {
		t.Fatalf("RetryChapter(-1) = %v, want ErrBlueprintLocked: video-level tasks share that ordinal", err)
	}
	if _, err := s.Rerun(ctx, "v1", []entity.TaskID{blueprint}, false); !errors.Is(err, ErrBlueprintLocked) {
		t.Fatalf("Rerun(blueprint) = %v, want ErrBlueprintLocked", err)
	}
	if _, err := s.Rerun(ctx, "v1", []entity.TaskID{blueprint}, true); !errors.Is(err, ErrBlueprintLocked) {
		t.Fatalf("Rerun(blueprint, dryRun) = %v, want ErrBlueprintLocked", err)
	}
	if store.state(blueprint) != entity.TaskStateSucceeded {
		t.Fatalf("the blueprint was disturbed: %s", store.state(blueprint))
	}

	// Everything downstream is still ordinary work.
	if err := s.RetryChapter(ctx, "v1", 1); err != nil {
		t.Fatalf("RetryChapter(1) = %v, want the chapter to be retryable", err)
	}
}

// A failed blueprint is the one that may be run again: nothing has been built
// from it. Without this a transient provider error, or an operator rejecting an
// outline, would kill the video permanently.
func TestFailedBlueprintCanBeRerun(t *testing.T) {
	t.Parallel()
	store := newRecordingStore()
	lifecycle := newNoopLifecycle()
	pools, err := NewPools(map[entity.Pool]int{})
	if err != nil {
		t.Fatalf("NewPools: %v", err)
	}
	s := New(pools, store, funcRunner{fn: func(context.Context, entity.Task) entity.TaskOutcome {
		return entity.Failed{Err: errors.New("the model is down"), Retryable: false}
	}}, lifecycle, nil, discardLogger(), Config{})
	ctx := startScheduler(t, s)
	submitHead(t, ctx, s, "v1", false)

	blueprint := entity.NewTaskID("v1", entity.TaskKindBlueprint, -1, -1)
	waitFor(t, 5*time.Second, "the blueprint to fail", func() bool {
		return store.state(blueprint) == entity.TaskStateFailed
	})
	if err := s.RetryTask(ctx, blueprint); err != nil {
		t.Fatalf("RetryTask on a failed blueprint = %v, want it to be allowed", err)
	}
}

// A rejected blueprint is failed, so it can be run again too: rejecting an
// outline and asking for another is the flow, not a dead end.
func TestRejectedBlueprintCanBeRerun(t *testing.T) {
	t.Parallel()
	store := newRecordingStore()
	pools, err := NewPools(map[entity.Pool]int{})
	if err != nil {
		t.Fatalf("NewPools: %v", err)
	}
	s := New(pools, store, funcRunner{fn: func(context.Context, entity.Task) entity.TaskOutcome {
		return entity.Success{}
	}}, newNoopLifecycle(), nil, discardLogger(), Config{})
	ctx := startScheduler(t, s)
	submitHead(t, ctx, s, "v1", true)

	blueprint := entity.NewTaskID("v1", entity.TaskKindBlueprint, -1, -1)
	waitFor(t, 5*time.Second, "the gate to open", func() bool {
		return store.state(blueprint) == entity.TaskStateAwaitingApproval
	})
	// Parked is not rejected: the outline is still the one the graph would expand
	// from, so it is not re-runnable yet.
	if err := s.RetryTask(ctx, blueprint); !errors.Is(err, ErrBlueprintLocked) {
		t.Fatalf("RetryTask on a parked blueprint = %v, want ErrBlueprintLocked", err)
	}
	if err := s.Reject(ctx, blueprint, "not the story I wanted"); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if err := s.RetryTask(ctx, blueprint); err != nil {
		t.Fatalf("RetryTask on a rejected blueprint = %v, want it to be allowed", err)
	}
	waitFor(t, 5*time.Second, "the second outline to park", func() bool {
		return store.state(blueprint) == entity.TaskStateAwaitingApproval
	})
}

// A crash between expansion and the blueprint's completion leaves the task
// `running` over a graph that is already built from its output. Re-running it
// would roll a fresh outline underneath that graph, so it is reclaimed as the
// committed thing it is.
func TestExpandedBlueprintIsReclaimedAsCommitted(t *testing.T) {
	t.Parallel()
	const chapters, images = 3, 2
	head, tail := headAndTail(t, "v1", chapters, images, false)
	if err := head.Expand(tail); err != nil {
		t.Fatalf("Expand: %v", err)
	}
	// Exactly what a crash in that window persists.
	head.Task(0).State = entity.TaskStateRunning

	store := newRecordingStore()
	lifecycle := newNoopLifecycle()
	pools, err := NewPools(map[entity.Pool]int{})
	if err != nil {
		t.Fatalf("NewPools: %v", err)
	}
	// Atomic because the assertion below reads what worker goroutines wrote.
	var ranBlueprint atomic.Bool
	s := New(pools, store, funcRunner{fn: func(_ context.Context, task entity.Task) entity.TaskOutcome {
		if task.Kind == entity.TaskKindBlueprint {
			ranBlueprint.Store(true)
		}
		return entity.Success{}
	}}, lifecycle, nil, discardLogger(), Config{})
	ctx := startScheduler(t, s)

	restored, err := GraphFromPersisted(persistedFrom(head))
	if err != nil {
		t.Fatalf("GraphFromPersisted: %v", err)
	}
	if err := s.Resume(ctx, []*Graph{restored}); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitFor(t, 5*time.Second, "the resumed video to complete", func() bool {
		return lifecycle.state("v1") == entity.VideoStateCompleted
	})
	if ranBlueprint.Load() {
		t.Fatal("the blueprint was re-run over a graph already built from its output")
	}
	if store.state(entity.NewTaskID("v1", entity.TaskKindBlueprint, -1, -1)) != entity.TaskStateSucceeded {
		t.Fatal("the reclaimed blueprint was not recorded as succeeded")
	}
}

// An unexpanded blueprint caught in flight is reclaimed the ordinary way: it
// has committed nothing, and every step is idempotent.
func TestUnexpandedBlueprintIsReclaimedAsRunnable(t *testing.T) {
	t.Parallel()
	head, err := BuildHeadGraph(HeadSpec{VideoID: "v1", MaxAttempts: 3, Now: time.Unix(0, 0).UTC()})
	if err != nil {
		t.Fatalf("BuildHeadGraph: %v", err)
	}
	head.Task(0).State = entity.TaskStateRunning

	store := newRecordingStore()
	pools, err := NewPools(map[entity.Pool]int{})
	if err != nil {
		t.Fatalf("NewPools: %v", err)
	}
	ran := make(chan struct{}, 1)
	s := New(pools, store, funcRunner{fn: func(context.Context, entity.Task) entity.TaskOutcome {
		select {
		case ran <- struct{}{}:
		default:
		}
		return entity.Success{}
	}}, newNoopLifecycle(), nil, discardLogger(), Config{})
	ctx := startScheduler(t, s)
	if err := s.Resume(ctx, []*Graph{head}); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	select {
	case <-ran:
	case <-time.After(5 * time.Second):
		t.Fatal("an interrupted blueprint with nothing built from it was not re-run")
	}
}
