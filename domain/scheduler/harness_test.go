package scheduler

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// discardLogger keeps test output readable without disabling the code paths
// that build log attributes.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// recordingStore is an in-memory TransitionStore that keeps the last state
// written for every task, so a test can assert what would survive a restart.
type recordingStore struct {
	mu          sync.Mutex
	graphs      map[entity.VideoID][]entity.Task
	edges       map[entity.VideoID][]repository.TaskEdge
	last        map[entity.TaskID]repository.TaskTransition
	batches     []int
	transitions int
}

func newRecordingStore() *recordingStore {
	return &recordingStore{
		graphs: make(map[entity.VideoID][]entity.Task),
		edges:  make(map[entity.VideoID][]repository.TaskEdge),
		last:   make(map[entity.TaskID]repository.TaskTransition),
	}
}

func (s *recordingStore) InsertGraph(_ context.Context, videoID entity.VideoID, tasks []entity.Task, edges []repository.TaskEdge) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored := make([]entity.Task, len(tasks))
	copy(stored, tasks)
	s.graphs[videoID] = stored
	s.edges[videoID] = edges
	return nil
}

func (s *recordingStore) ApplyTransitions(_ context.Context, transitions []repository.TaskTransition) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batches = append(s.batches, len(transitions))
	s.transitions += len(transitions)
	for _, t := range transitions {
		s.last[t.ID] = t
	}
	return nil
}

func (s *recordingStore) state(id entity.TaskID) entity.TaskState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last[id].State
}

// persisted rebuilds the durable view of a video, exactly as ListOpenGraphs
// would return it after a restart.
func (s *recordingStore) persisted(videoID entity.VideoID) repository.VideoGraph {
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks := make([]entity.Task, 0, len(s.graphs[videoID]))
	for _, t := range s.graphs[videoID] {
		if last, ok := s.last[t.ID]; ok {
			t.State = last.State
			t.Attempt = last.Attempt
			t.DepsRemaining = last.DepsRemaining
			t.Error = last.Error
			t.StartedAt = last.StartedAt
			t.FinishedAt = last.FinishedAt
			t.NotBefore = last.NotBefore
		}
		tasks = append(tasks, t)
	}
	return repository.VideoGraph{VideoID: videoID, Tasks: tasks, Edges: s.edges[videoID]}
}

// poolWatcher asserts the §5 invariant continuously: no pool ever exceeds its
// limit, and every task holds exactly one slot in exactly one pool.
type poolWatcher struct {
	mu      sync.Mutex
	current map[entity.Pool]int
	peak    map[entity.Pool]int
}

func newPoolWatcher() *poolWatcher {
	return &poolWatcher{
		current: make(map[entity.Pool]int, entity.NumPools),
		peak:    make(map[entity.Pool]int, entity.NumPools),
	}
}

func (w *poolWatcher) enter(p entity.Pool) {
	w.mu.Lock()
	w.current[p]++
	if w.current[p] > w.peak[p] {
		w.peak[p] = w.current[p]
	}
	w.mu.Unlock()
}

func (w *poolWatcher) leave(p entity.Pool) {
	w.mu.Lock()
	w.current[p]--
	w.mu.Unlock()
}

func (w *poolWatcher) peakOf(p entity.Pool) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.peak[p]
}

// funcRunner adapts a function to the Runner port.
type funcRunner struct {
	fn func(ctx context.Context, t entity.Task) entity.TaskOutcome
}

func (r funcRunner) Run(ctx context.Context, t entity.Task) entity.TaskOutcome { return r.fn(ctx, t) }

// orderTracker records completions so a test can assert that no task ever
// started before every one of its dependencies had finished.
type orderTracker struct {
	mu        sync.Mutex
	done      map[entity.TaskID]bool
	started   []entity.TaskID
	violation string
}

func newOrderTracker() *orderTracker {
	return &orderTracker{done: make(map[entity.TaskID]bool, 512)}
}

func (o *orderTracker) starting(g *Graph, t entity.Task) {
	idx, ok := g.IndexOf(t.ID)
	if !ok {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.started = append(o.started, t.ID)
	for _, dep := range g.dependencies[idx] {
		if !o.done[g.tasks[dep].ID] {
			o.violation = string(t.ID) + " started before " + string(g.tasks[dep].ID)
			return
		}
	}
}

func (o *orderTracker) finished(id entity.TaskID) {
	o.mu.Lock()
	o.done[id] = true
	o.mu.Unlock()
}

func (o *orderTracker) failure() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.violation
}

// noopLifecycle records the last derived state per video.
type noopLifecycle struct {
	mu     sync.Mutex
	states map[entity.VideoID]entity.VideoState
}

func newNoopLifecycle() *noopLifecycle {
	return &noopLifecycle{states: make(map[entity.VideoID]entity.VideoState)}
}

func (l *noopLifecycle) SetVideoState(_ context.Context, id entity.VideoID, state entity.VideoState, _ string) error {
	l.mu.Lock()
	l.states[id] = state
	l.mu.Unlock()
	return nil
}

func (l *noopLifecycle) state(id entity.VideoID) entity.VideoState {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.states[id]
}

// countingNotifier proves the SSE path is exercised without a broker.
type countingNotifier struct {
	mu    sync.Mutex
	tasks int
	video int
}

func (n *countingNotifier) NotifyTask(entity.TaskDelta) {
	n.mu.Lock()
	n.tasks++
	n.mu.Unlock()
}

func (n *countingNotifier) NotifyVideo(entity.VideoDelta) {
	n.mu.Lock()
	n.video++
	n.mu.Unlock()
}

func (n *countingNotifier) NotifyScheduler(entity.SchedulerDelta) {}

// waitFor polls a condition until it holds or the deadline passes. It is how
// every scheduler test also asserts "never deadlocks": a stuck loop fails here.
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", timeout, what)
}

// startScheduler runs a scheduler for the duration of the test and guarantees
// its goroutines have exited before the test returns.
func startScheduler(t *testing.T, s *Scheduler) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	waitFor(t, 2*time.Second, "scheduler to start", func() bool { return s.running.Load() })
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("scheduler returned %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("scheduler did not shut down within 5s")
		}
	})
	return ctx
}

func testGraph(t *testing.T, videoID entity.VideoID, chapters, images int, gates bool) *Graph {
	t.Helper()
	g, err := BuildGraph(BuildSpec{
		VideoID:          videoID,
		ChapterCount:     chapters,
		ImagesPerChapter: images,
		MaxAttempts:      3,
		BlueprintGate:    gates,
		UploadGate:       gates,
		Now:              time.Unix(0, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	return g
}

// persistedFrom is the durable projection of a freshly built graph.
func persistedFrom(g *Graph) repository.VideoGraph {
	return repository.VideoGraph{VideoID: g.VideoID, Tasks: g.Tasks(), Edges: g.Edges()}
}
