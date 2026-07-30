package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
)

type rig struct {
	sched     *Scheduler
	store     *recordingStore
	pools     *Pools
	watcher   *poolWatcher
	order     *orderTracker
	lifecycle *noopLifecycle
	notifier  *countingNotifier
}

// newRig wires a scheduler whose runner records pool occupancy and dependency
// order, so every test also checks the pool and ordering invariants.
func newRig(t *testing.T, limits map[entity.Pool]int, g *Graph, run func(t entity.Task) entity.TaskOutcome) *rig {
	t.Helper()
	if limits == nil {
		limits = map[entity.Pool]int{
			entity.PoolLLM: 2, entity.PoolTTS: 2, entity.PoolImage: 2,
			entity.PoolCompose: 2, entity.PoolCache: 8, entity.PoolUpload: 1,
		}
	}
	pools, err := NewPools(limits)
	if err != nil {
		t.Fatalf("NewPools: %v", err)
	}
	store := newRecordingStore()
	watcher := newPoolWatcher()
	order := newOrderTracker()
	lifecycle := newNoopLifecycle()
	notifier := &countingNotifier{}

	runner := funcRunner{fn: func(_ context.Context, task entity.Task) entity.TaskOutcome {
		watcher.enter(task.Pool)
		defer watcher.leave(task.Pool)
		order.starting(g, task)
		outcome := entity.TaskOutcome(entity.Success{})
		if run != nil {
			outcome = run(task)
		}
		if _, ok := outcome.(entity.Success); ok {
			order.finished(task.ID)
		}
		return outcome
	}}

	sched := New(pools, store, runner, lifecycle, notifier, discardLogger(), Config{
		RetryBase:      5 * time.Millisecond,
		RetryMax:       20 * time.Millisecond,
		SafetyInterval: 30 * time.Second,
	})
	return &rig{sched: sched, store: store, pools: pools, watcher: watcher, order: order, lifecycle: lifecycle, notifier: notifier}
}

func (r *rig) assertInvariants(t *testing.T, limits map[entity.Pool]int) {
	t.Helper()
	if v := r.order.failure(); v != "" {
		t.Fatalf("dependency order violated: %s", v)
	}
	for pool, limit := range limits {
		if peak := r.watcher.peakOf(pool); peak > limit {
			t.Fatalf("pool %s peaked at %d, limit is %d", pool, peak, limit)
		}
	}
}

func TestFullPipelineCompletes(t *testing.T) {
	t.Parallel()
	limits := map[entity.Pool]int{
		entity.PoolLLM: 2, entity.PoolTTS: 2, entity.PoolImage: 2,
		entity.PoolCompose: 2, entity.PoolCache: 8, entity.PoolUpload: 1,
	}
	g := testGraph(t, "v1", 50, 2, false)
	r := newRig(t, limits, g, nil)
	ctx := startScheduler(t, r.sched)

	if err := r.sched.Submit(ctx, g); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitFor(t, 30*time.Second, "the 50-chapter pipeline to finish", func() bool {
		return r.sched.Snapshot().Succeeded == g.NodeCount()
	})

	r.assertInvariants(t, limits)
	waitFor(t, 2*time.Second, "video state to settle", func() bool {
		return r.lifecycle.state("v1") == entity.VideoStateCompleted
	})
	for i := range g.NodeCount() {
		if got := r.store.state(g.Task(i).ID); got != entity.TaskStateSucceeded {
			t.Fatalf("%s persisted as %q, want succeeded", g.Task(i).ID, got)
		}
	}
}

// A gated task must not release its successors; the pipeline parks and consumes
// nothing until a human acts.
func TestBlueprintGateParksAndApprovalReleases(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 6, 2, true)
	r := newRig(t, nil, g, nil)
	ctx := startScheduler(t, r.sched)

	if err := r.sched.Submit(ctx, g); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitFor(t, 5*time.Second, "the blueprint gate to open", func() bool {
		return r.sched.Snapshot().AwaitingApproval == 1
	})

	// Nothing downstream may have run.
	snap := r.sched.Snapshot()
	if snap.Succeeded != 0 {
		t.Fatalf("%d tasks succeeded while the gate was open", snap.Succeeded)
	}
	if snap.Running != 0 || snap.Ready != 0 {
		t.Fatalf("gate did not stop dispatch: running=%d ready=%d", snap.Running, snap.Ready)
	}
	if state := r.lifecycle.state("v1"); state != entity.VideoStateAwaitingApproval {
		t.Fatalf("video state = %q, want awaiting_approval", state)
	}

	blueprintID := entity.NewTaskID("v1", entity.TaskKindBlueprint, -1, -1)
	if err := r.sched.Approve(ctx, blueprintID); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	waitFor(t, 5*time.Second, "the upload gate to open", func() bool {
		s := r.sched.Snapshot()
		return s.AwaitingApproval == 1 && s.Succeeded == g.NodeCount()-2
	})

	metadataID := entity.NewTaskID("v1", entity.TaskKindMetadata, -1, -1)
	if err := r.sched.Approve(ctx, metadataID); err != nil {
		t.Fatalf("Approve upload gate: %v", err)
	}
	waitFor(t, 5*time.Second, "the pipeline to finish", func() bool {
		return r.sched.Snapshot().Succeeded == g.NodeCount()
	})
	r.assertInvariants(t, map[entity.Pool]int{entity.PoolLLM: 2, entity.PoolImage: 2})
}

func TestApproveRejectsUngatedTask(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 2, 1, false)
	r := newRig(t, nil, g, nil)
	ctx := startScheduler(t, r.sched)
	if err := r.sched.Submit(ctx, g); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitFor(t, 5*time.Second, "the pipeline to finish", func() bool {
		return r.sched.Snapshot().Succeeded == g.NodeCount()
	})
	err := r.sched.Approve(ctx, entity.NewTaskID("v1", entity.TaskKindBlueprint, -1, -1))
	if !errors.Is(err, ErrNotGated) {
		t.Fatalf("Approve on an ungated task = %v, want ErrNotGated", err)
	}
	if err := r.sched.Approve(ctx, "nope"); !errors.Is(err, ErrUnknownTask) {
		t.Fatalf("Approve on an unknown task = %v, want ErrUnknownTask", err)
	}
}

func TestRejectFailsTheGatedTask(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 2, 1, true)
	r := newRig(t, nil, g, nil)
	ctx := startScheduler(t, r.sched)
	if err := r.sched.Submit(ctx, g); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitFor(t, 5*time.Second, "the gate to open", func() bool {
		return r.sched.Snapshot().AwaitingApproval == 1
	})
	blueprintID := entity.NewTaskID("v1", entity.TaskKindBlueprint, -1, -1)
	if err := r.sched.Reject(ctx, blueprintID, "outline is wrong"); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	waitFor(t, 5*time.Second, "the task to fail", func() bool {
		return r.store.state(blueprintID) == entity.TaskStateFailed
	})
	waitFor(t, 2*time.Second, "the video to fail", func() bool {
		return r.lifecycle.state("v1") == entity.VideoStateFailed
	})
}

func TestRetryableFailureIsRetriedAndSucceeds(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 3, 2, false)
	var attempts int
	r := newRig(t, nil, g, func(task entity.Task) entity.TaskOutcome {
		if task.Kind == entity.TaskKindTTS && task.Ordinal == 2 && attempts < 2 {
			attempts++
			return entity.Failed{Err: errors.New("transient"), Retryable: true}
		}
		return entity.Success{}
	})
	ctx := startScheduler(t, r.sched)
	if err := r.sched.Submit(ctx, g); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitFor(t, 10*time.Second, "the pipeline to finish despite retries", func() bool {
		return r.sched.Snapshot().Succeeded == g.NodeCount()
	})
	if attempts != 2 {
		t.Fatalf("injected failures = %d, want 2", attempts)
	}
}

func TestPermanentFailureBlocksOnlyItsTail(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 3, 2, false)
	r := newRig(t, nil, g, func(task entity.Task) entity.TaskOutcome {
		if task.Kind == entity.TaskKindScript && task.Ordinal == 2 {
			return entity.Failed{Err: errors.New("bad chapter"), Retryable: false}
		}
		return entity.Success{}
	})
	ctx := startScheduler(t, r.sched)
	if err := r.sched.Submit(ctx, g); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitFor(t, 10*time.Second, "the video to settle as failed", func() bool {
		s := r.sched.Snapshot()
		return s.Failed == 1 && s.Running == 0 && s.Ready == 0 && s.RetryPending == 0
	})

	// Every other chapter's stills and narration still completed: a failure in one
	// chapter must not stop the rest.
	failedTail := len(g.Downstream(mustIndex(t, g, entity.NewTaskID("v1", entity.TaskKindScript, 2, -1))))
	snap := r.sched.Snapshot()
	if want := g.NodeCount() - failedTail; snap.Succeeded != want {
		t.Fatalf("succeeded = %d, want %d (everything outside the failed tail)", snap.Succeeded, want)
	}
	if state := r.lifecycle.state("v1"); state != entity.VideoStateFailed {
		t.Fatalf("video state = %q, want failed", state)
	}
}

func TestRetryTaskResetsTheTail(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 3, 2, false)
	fail := true
	r := newRig(t, nil, g, func(task entity.Task) entity.TaskOutcome {
		if task.Kind == entity.TaskKindScript && task.Ordinal == 2 && fail {
			return entity.Failed{Err: errors.New("bad chapter"), Retryable: false}
		}
		return entity.Success{}
	})
	ctx := startScheduler(t, r.sched)
	if err := r.sched.Submit(ctx, g); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitFor(t, 10*time.Second, "the failure to settle", func() bool {
		return r.sched.Snapshot().Failed == 1 && r.sched.Snapshot().Running == 0
	})

	fail = false
	if err := r.sched.RetryChapter(ctx, "v1", 2); err != nil {
		t.Fatalf("RetryChapter: %v", err)
	}
	waitFor(t, 10*time.Second, "the retried pipeline to finish", func() bool {
		return r.sched.Snapshot().Succeeded == g.NodeCount()
	})
	r.assertInvariants(t, map[entity.Pool]int{entity.PoolImage: 2})
}

// Retrying a task whose dependent is already in flight must discard that
// dependent's result: it was produced from the input the retry has just
// replaced.
//
// The hazard is that resetFrom leaves a running task alone, and
// releaseDependents only releases dependents in `blocked`. A dependent that
// completes as `succeeded` after the reset is therefore never re-run, and the
// video keeps an artifact derived from an input that no longer exists.
func TestRetryDiscardsDependentAlreadyInFlight(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 1, 1, false)

	var mu sync.Mutex
	runs := make(map[entity.TaskKind]int)
	held := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once

	r := newRig(t, nil, g, func(task entity.Task) entity.TaskOutcome {
		mu.Lock()
		runs[task.Kind]++
		first := runs[task.Kind] == 1
		mu.Unlock()
		// Hold the narration in flight so the retry below lands while it runs.
		if task.Kind == entity.TaskKindTTS && first {
			once.Do(func() { close(held) })
			<-release
		}
		return entity.Success{}
	})
	ctx := startScheduler(t, r.sched)
	defer close(release)

	if err := r.sched.Submit(ctx, g); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	<-held

	// The script is the narration's dependency, so retrying it invalidates the
	// narration currently being produced.
	script := taskOfKind(t, g, entity.TaskKindScript, 1)
	if err := r.sched.RetryTask(ctx, script.ID); err != nil {
		t.Fatalf("RetryTask: %v", err)
	}
	release <- struct{}{}

	waitFor(t, 10*time.Second, "the narration to be re-run against the new script", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return runs[entity.TaskKindTTS] >= 2
	})

	mu.Lock()
	scriptRuns := runs[entity.TaskKindScript]
	mu.Unlock()
	if scriptRuns < 2 {
		t.Fatalf("script ran %d times, want it re-run by the retry", scriptRuns)
	}
}

// taskOfKind finds the single task of a kind and chapter ordinal.
func taskOfKind(t *testing.T, g *Graph, kind entity.TaskKind, ordinal int) *entity.Task {
	t.Helper()
	for i := range g.NodeCount() {
		if task := g.Task(i); task.Kind == kind && task.Ordinal == ordinal {
			return task
		}
	}
	t.Fatalf("no %s task for chapter %d", kind, ordinal)
	return nil
}

// Re-running a task that already succeeded must not silently redo everything
// below it. The downstream is flagged and left alone until an operator says
// what to do with it.
func TestRerunMarksDownstreamStaleWithoutRunningIt(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 2, 1, false)

	var mu sync.Mutex
	runs := make(map[entity.TaskID]int)
	r := newRig(t, nil, g, func(task entity.Task) entity.TaskOutcome {
		mu.Lock()
		runs[task.ID]++
		mu.Unlock()
		return entity.Success{}
	})
	ctx := startScheduler(t, r.sched)
	if err := r.sched.Submit(ctx, g); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitFor(t, 10*time.Second, "the pipeline to finish", func() bool {
		return r.sched.Snapshot().Succeeded == g.NodeCount()
	})

	countOf := func(id entity.TaskID) int {
		mu.Lock()
		defer mu.Unlock()
		return runs[id]
	}

	script := taskOfKind(t, g, entity.TaskKindScript, 1)
	tts := taskOfKind(t, g, entity.TaskKindTTS, 1)
	ttsBefore := countOf(tts.ID)

	// The preview must not change anything.
	preview, err := r.sched.Rerun(ctx, "v1", []entity.TaskID{script.ID}, true)
	if err != nil {
		t.Fatalf("Rerun dry run: %v", err)
	}
	if len(preview) == 0 {
		t.Fatal("dry run reported nothing downstream of the script")
	}
	if n := r.sched.Snapshot().Succeeded; n != g.NodeCount() {
		t.Fatalf("dry run disturbed the graph: succeeded = %d", n)
	}

	affected, err := r.sched.Rerun(ctx, "v1", []entity.TaskID{script.ID}, false)
	if err != nil {
		t.Fatalf("Rerun: %v", err)
	}
	if len(affected) != len(preview) {
		t.Fatalf("rerun affected %d tasks, preview said %d", len(affected), len(preview))
	}
	waitFor(t, 10*time.Second, "the script to be re-run", func() bool {
		return countOf(script.ID) == 2
	})

	// The narration is downstream of the script, so it is stale — but it must
	// not have run again on its own.
	waitFor(t, 5*time.Second, "the narration to be marked stale", func() bool {
		return r.store.stale(tts.ID)
	})
	if got := countOf(tts.ID); got != ttsBefore {
		t.Fatalf("narration ran %d times, want it left alone at %d", got, ttsBefore)
	}
	if r.store.stale(script.ID) {
		t.Fatal("the re-run task marked itself stale")
	}

	// Accepting clears the flag and still does not run anything.
	n, err := r.sched.AcceptStale(ctx, "v1", []entity.TaskID{tts.ID})
	if err != nil {
		t.Fatalf("AcceptStale: %v", err)
	}
	if n != 1 {
		t.Fatalf("AcceptStale reported %d tasks, want 1", n)
	}
	waitFor(t, 5*time.Second, "the accepted task to lose its flag", func() bool {
		return !r.store.stale(tts.ID)
	})
	if got := countOf(tts.ID); got != ttsBefore {
		t.Fatalf("accepting re-ran the narration (%d runs)", got)
	}
}

// The other exit from stale: run it.
func TestRunStaleReRunsTheFlaggedTasks(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 2, 1, false)

	var mu sync.Mutex
	runs := make(map[entity.TaskID]int)
	r := newRig(t, nil, g, func(task entity.Task) entity.TaskOutcome {
		mu.Lock()
		runs[task.ID]++
		mu.Unlock()
		return entity.Success{}
	})
	ctx := startScheduler(t, r.sched)
	if err := r.sched.Submit(ctx, g); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitFor(t, 10*time.Second, "the pipeline to finish", func() bool {
		return r.sched.Snapshot().Succeeded == g.NodeCount()
	})

	script := taskOfKind(t, g, entity.TaskKindScript, 1)
	tts := taskOfKind(t, g, entity.TaskKindTTS, 1)
	mu.Lock()
	ttsBefore := runs[tts.ID]
	mu.Unlock()

	if _, err := r.sched.Rerun(ctx, "v1", []entity.TaskID{script.ID}, false); err != nil {
		t.Fatalf("Rerun: %v", err)
	}
	waitFor(t, 5*time.Second, "the narration to be marked stale", func() bool {
		return r.store.stale(tts.ID)
	})

	n, err := r.sched.RunStale(ctx, "v1", nil)
	if err != nil {
		t.Fatalf("RunStale: %v", err)
	}
	if n == 0 {
		t.Fatal("RunStale reported no tasks")
	}
	waitFor(t, 10*time.Second, "the stale set to be re-run and settle", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return runs[tts.ID] > ttsBefore
	})
	waitFor(t, 10*time.Second, "the video to be whole again", func() bool {
		s := r.sched.Snapshot()
		return s.Succeeded == g.NodeCount() && s.Running == 0
	})
	if r.store.stale(tts.ID) {
		t.Fatal("a re-run task is still marked stale")
	}
}

// A cancelled video frees its slots within 100 ms.
func TestCancelFreesSlotsQuickly(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 20, 2, false)
	release := make(chan struct{})
	r := newRig(t, nil, g, func(task entity.Task) entity.TaskOutcome {
		if task.Kind == entity.TaskKindBlueprint {
			return entity.Success{}
		}
		<-release
		return entity.Success{}
	})
	ctx := startScheduler(t, r.sched)
	defer close(release)

	if err := r.sched.Submit(ctx, g); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitFor(t, 5*time.Second, "tasks to be in flight", func() bool {
		return r.pools.TotalInFlight() > 0
	})

	// The runner ignores its context, so this measures the scheduler's own
	// bookkeeping rather than a cooperative provider.
	start := time.Now()
	if err := r.sched.Cancel(ctx, "v1"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	waitFor(t, 2*time.Second, "the video to be cancelled", func() bool {
		return r.lifecycle.state("v1") == entity.VideoStateCancelled
	})
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("cancel took %s, budget is 100ms", elapsed)
	}
	// The console snapshot is published just after the video state is written, so
	// the queue depth is asserted on its own rather than in the same instant.
	waitFor(t, 2*time.Second, "the ready queue to drain", func() bool {
		return r.sched.Snapshot().Ready == 0
	})
}

// A context-respecting provider releases its slot when the video is cancelled.
func TestCancelPropagatesToRunningTasks(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 10, 2, false)
	r := newRig(t, nil, g, nil)
	// Replace the runner with one that blocks on its context.
	r.sched.runner = funcRunner{fn: func(ctx context.Context, task entity.Task) entity.TaskOutcome {
		if task.Kind == entity.TaskKindBlueprint {
			return entity.Success{}
		}
		<-ctx.Done()
		return entity.Failed{Err: ctx.Err(), Retryable: false}
	}}
	ctx := startScheduler(t, r.sched)
	if err := r.sched.Submit(ctx, g); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitFor(t, 5*time.Second, "tasks to be in flight", func() bool {
		return r.pools.TotalInFlight() > 0
	})
	if err := r.sched.Cancel(ctx, "v1"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	waitFor(t, 1*time.Second, "pool slots to come back", func() bool {
		return r.pools.TotalInFlight() == 0
	})
}

// A crash 45 minutes into a run must resume, not restart.
func TestResumeContinuesFromPersistedState(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 8, 2, false)
	gate := make(chan struct{})
	first := newRig(t, nil, g, func(task entity.Task) entity.TaskOutcome {
		if task.Kind == entity.TaskKindImage {
			<-gate // stall the image branch so the run is genuinely partial
		}
		return entity.Success{}
	})
	ctx := startScheduler(t, first.sched)
	if err := first.sched.Submit(ctx, g); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitFor(t, 10*time.Second, "the narration branch to finish", func() bool {
		return first.store.state(entity.NewTaskID("v1", entity.TaskKindTTS, 8, -1)) == entity.TaskStateSucceeded
	})
	succeededBefore := first.sched.Snapshot().Succeeded
	if succeededBefore == g.NodeCount() {
		t.Fatal("the run was not partial")
	}
	close(gate)

	// Simulate a restart: rebuild from what the store holds and resume.
	restored, err := GraphFromPersisted(first.store.persisted("v1"))
	if err != nil {
		t.Fatalf("GraphFromPersisted: %v", err)
	}
	second := newRig(t, nil, restored, nil)
	second.store = first.store
	// The new tracker starts empty, so it must be told what the previous process
	// already finished — otherwise it would flag every survivor.
	for i := range restored.NodeCount() {
		if restored.Task(i).State == entity.TaskStateSucceeded {
			second.order.finished(restored.Task(i).ID)
		}
	}
	ctx2 := startScheduler(t, second.sched)
	if err := second.sched.Resume(ctx2, []*Graph{restored}); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitFor(t, 20*time.Second, "the resumed pipeline to finish", func() bool {
		return second.sched.Snapshot().Succeeded == g.NodeCount()
	})
	if v := second.order.failure(); v != "" {
		t.Fatalf("dependency order violated after resume: %s", v)
	}
}

// Two videos compete for the same global slots.
func TestPoolsAreGlobalAcrossVideos(t *testing.T) {
	t.Parallel()
	limits := map[entity.Pool]int{
		entity.PoolLLM: 2, entity.PoolTTS: 1, entity.PoolImage: 2,
		entity.PoolCompose: 2, entity.PoolCache: 8, entity.PoolUpload: 1,
	}
	a := testGraph(t, "va", 10, 2, false)
	b := testGraph(t, "vb", 10, 2, false)
	r := newRig(t, limits, a, func(task entity.Task) entity.TaskOutcome {
		if task.Pool == entity.PoolTTS {
			time.Sleep(2 * time.Millisecond)
		}
		return entity.Success{}
	})
	ctx := startScheduler(t, r.sched)
	if err := r.sched.Submit(ctx, a); err != nil {
		t.Fatalf("Submit a: %v", err)
	}
	if err := r.sched.Submit(ctx, b); err != nil {
		t.Fatalf("Submit b: %v", err)
	}
	waitFor(t, 30*time.Second, "both videos to finish", func() bool {
		return r.sched.Snapshot().Succeeded == a.NodeCount()+b.NodeCount()
	})
	if peak := r.watcher.peakOf(entity.PoolTTS); peak > 1 {
		t.Fatalf("tts pool peaked at %d across two videos, limit is 1", peak)
	}
}

func TestSubmitIsIdempotent(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 3, 2, false)
	r := newRig(t, nil, g, nil)
	ctx := startScheduler(t, r.sched)
	for range 3 {
		if err := r.sched.Submit(ctx, g); err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}
	waitFor(t, 10*time.Second, "the pipeline to finish", func() bool {
		return r.sched.Snapshot().Succeeded == g.NodeCount()
	})
	if v := r.sched.Snapshot().Videos; v != 1 {
		t.Fatalf("tracked videos = %d, want 1", v)
	}
}

// A burst of completions must commit in one transaction, not N.
func TestTransitionsAreBatched(t *testing.T) {
	t.Parallel()
	limits := map[entity.Pool]int{
		entity.PoolLLM: 4, entity.PoolTTS: 4, entity.PoolImage: 8,
		entity.PoolCompose: 4, entity.PoolCache: 16, entity.PoolUpload: 1,
	}
	g := testGraph(t, "v1", 30, 2, false)
	r := newRig(t, limits, g, nil)
	ctx := startScheduler(t, r.sched)
	if err := r.sched.Submit(ctx, g); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitFor(t, 30*time.Second, "the pipeline to finish", func() bool {
		return r.sched.Snapshot().Succeeded == g.NodeCount()
	})
	r.store.mu.Lock()
	batches, transitions := len(r.store.batches), r.store.transitions
	r.store.mu.Unlock()
	if transitions == 0 {
		t.Fatal("no transitions were persisted")
	}
	if batches >= transitions {
		t.Fatalf("%d transitions committed in %d batches; batching is not happening", transitions, batches)
	}
}

func TestRunnerPanicBecomesAPermanentFailure(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 2, 1, false)
	r := newRig(t, nil, g, func(task entity.Task) entity.TaskOutcome {
		if task.Kind == entity.TaskKindBlueprint {
			panic("provider exploded")
		}
		return entity.Success{}
	})
	ctx := startScheduler(t, r.sched)
	if err := r.sched.Submit(ctx, g); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	blueprintID := entity.NewTaskID("v1", entity.TaskKindBlueprint, -1, -1)
	waitFor(t, 5*time.Second, "the panicking task to fail", func() bool {
		return r.store.state(blueprintID) == entity.TaskStateFailed
	})
}

func TestSetPoolLimitAppliesLive(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 2, 1, false)
	r := newRig(t, nil, g, nil)
	ctx := startScheduler(t, r.sched)

	if err := r.sched.SetPoolLimit(ctx, entity.PoolImage, 7); err != nil {
		t.Fatalf("SetPoolLimit: %v", err)
	}
	waitFor(t, 2*time.Second, "the new limit to apply", func() bool {
		return r.pools.Limit(entity.PoolImage) == 7
	})
	if err := r.sched.SetPoolLimit(ctx, entity.PoolImage, 3); err != nil {
		t.Fatalf("SetPoolLimit down: %v", err)
	}
	waitFor(t, 2*time.Second, "the lowered limit to apply", func() bool {
		return r.pools.Limit(entity.PoolImage) == 3
	})
	if err := r.sched.SetPoolLimit(ctx, entity.Pool("nope"), 1); !errors.Is(err, ErrUnknownPool) {
		t.Fatalf("SetPoolLimit on unknown pool = %v, want ErrUnknownPool", err)
	}
}

func TestNotifierSeesEveryTransition(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 3, 2, false)
	r := newRig(t, nil, g, nil)
	ctx := startScheduler(t, r.sched)
	if err := r.sched.Submit(ctx, g); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitFor(t, 10*time.Second, "the pipeline to finish", func() bool {
		return r.sched.Snapshot().Succeeded == g.NodeCount()
	})
	r.notifier.mu.Lock()
	tasks, videos := r.notifier.tasks, r.notifier.video
	r.notifier.mu.Unlock()
	if tasks < g.NodeCount() {
		t.Fatalf("task notifications = %d, want at least %d", tasks, g.NodeCount())
	}
	if videos == 0 {
		t.Fatal("no video notifications were emitted")
	}
}

func mustIndex(t *testing.T, g *Graph, id entity.TaskID) int32 {
	t.Helper()
	idx, ok := g.IndexOf(id)
	if !ok {
		t.Fatalf("no task %q in graph", id)
	}
	return idx
}
