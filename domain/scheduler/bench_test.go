package scheduler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
)

// The performance budgets, expressed as runnable gates. `benchstat` in CI
// compares these against the main baseline and blocks a regression over 5 %;
// the tests below additionally fail outright if an absolute budget is missed.

const benchChapters = 50

func benchGraph(b *testing.B) *Graph {
	b.Helper()
	g, err := BuildGraph(BuildSpec{
		VideoID:          "bench",
		ChapterCount:     benchChapters,
		ImagesPerChapter: 2,
		MaxAttempts:      3,
		Now:              time.Unix(0, 0),
	})
	if err != nil {
		b.Fatal(err)
	}
	return g
}

func BenchmarkBuildGraph(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		g, err := BuildGraph(BuildSpec{
			VideoID:          "bench",
			ChapterCount:     benchChapters,
			ImagesPerChapter: 2,
			MaxAttempts:      3,
			Now:              time.Unix(0, 0),
		})
		if err != nil || g == nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGraphEdges(b *testing.B) {
	g := benchGraph(b)
	b.ReportAllocs()
	for b.Loop() {
		if len(g.Edges()) == 0 {
			b.Fatal("no edges")
		}
	}
}

// dispatchFixture builds a saturated ready set: one video's whole image branch
// queued behind a pool with free slots, which is the steady state the dispatch
// decision runs in.
func dispatchFixture(tb testing.TB) (*ReadySet, *Pools, []entity.Task) {
	tb.Helper()
	pools, err := NewPools(map[entity.Pool]int{entity.PoolImage: 64})
	if err != nil {
		tb.Fatal(err)
	}
	tasks := make([]entity.Task, 256)
	rs := NewReadySet()
	for i := range tasks {
		tasks[i] = entity.Task{
			ID:    entity.NewTaskID("bench", entity.TaskKindImage, i, 0),
			Pool:  entity.PoolImage,
			State: entity.TaskStateReady,
		}
		rs.Push(&tasks[i])
	}
	return rs, pools, tasks
}

// pickNext is the dispatch decision in isolation: choose the next runnable task
// and take its slot. The budget is under 1 ms and zero allocations.
func pickNext(rs *ReadySet, pools *Pools) *entity.Task {
	t := rs.Next(entity.PoolImage)
	if t == nil {
		return nil
	}
	if !pools.TryAcquire(entity.PoolImage) {
		return nil
	}
	rs.Pop(entity.PoolImage)
	return t
}

func BenchmarkDispatchDecision(b *testing.B) {
	rs, pools, tasks := dispatchFixture(b)
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		t := pickNext(rs, pools)
		if t == nil {
			b.Fatal("ready set ran dry")
		}
		pools.Release(entity.PoolImage)
		// Recycle so the ring stays at its steady-state capacity.
		t.State = entity.TaskStateReady
		rs.Push(t)
		i++
		_ = tasks
	}
}

// TestDispatchDecisionIsAllocationFree asserts the zero-allocation requirement
// directly rather than trusting the benchmark's report.
func TestDispatchDecisionIsAllocationFree(t *testing.T) {
	// Not parallel: AllocsPerRun requires exclusive use of the process.
	rs, pools, _ := dispatchFixture(t)
	// Warm the rings so growth is not counted.
	for range 64 {
		task := pickNext(rs, pools)
		pools.Release(entity.PoolImage)
		task.State = entity.TaskStateReady
		rs.Push(task)
	}
	allocs := testing.AllocsPerRun(1000, func() {
		task := pickNext(rs, pools)
		if task == nil {
			t.Fatal("ready set ran dry")
		}
		pools.Release(entity.PoolImage)
		task.State = entity.TaskStateReady
		rs.Push(task)
	})
	if allocs != 0 {
		t.Fatalf("dispatch decision allocated %.0f times per iteration, budget is 0", allocs)
	}
}

// TestDispatchDecisionLatency asserts the absolute budget: picking the next
// runnable task must take well under 1 ms.
func TestDispatchDecisionLatency(t *testing.T) {
	t.Parallel()
	rs, pools, _ := dispatchFixture(t)
	const iterations = 10_000

	start := time.Now()
	for range iterations {
		task := pickNext(rs, pools)
		if task == nil {
			t.Fatal("ready set ran dry")
		}
		pools.Release(entity.PoolImage)
		task.State = entity.TaskStateReady
		rs.Push(task)
	}
	perDecision := time.Since(start) / iterations
	if perDecision > time.Millisecond {
		t.Fatalf("dispatch decision took %s, budget is 1ms", perDecision)
	}
	t.Logf("dispatch decision: %s", perDecision)
}

func BenchmarkReadySetPushNext(b *testing.B) {
	rs := NewReadySet()
	task := entity.Task{ID: "t", Pool: entity.PoolImage, State: entity.TaskStateReady}
	b.ReportAllocs()
	for b.Loop() {
		rs.Push(&task)
		if rs.Next(entity.PoolImage) == nil {
			b.Fatal("empty")
		}
		rs.Pop(entity.PoolImage)
	}
}

func BenchmarkRetryQueue(b *testing.B) {
	base := time.Unix(1_700_000_000, 0)
	var q retryQueue
	// Warm to steady-state capacity so the benchmark measures sift, not growth.
	for i := range 512 {
		q.push(retryItem{taskID: entity.TaskID("t"), when: base.Add(time.Duration(i) * time.Millisecond)})
	}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		q.push(retryItem{taskID: "t", when: base.Add(time.Duration(i%1024) * time.Millisecond)})
		q.popDue(base.Add(time.Hour))
		i++
	}
}

// BenchmarkTaskDeltaJSON covers the JSON boundary every task transition crosses
// on its way to the SSE stream.
func BenchmarkTaskDeltaJSON(b *testing.B) {
	now := time.Unix(1_700_000_000, 0).UTC()
	chapterID := entity.NewChapterID("v1", 7)
	delta := entity.TaskDelta{
		ID:        entity.NewTaskID("v1", entity.TaskKindImage, 7, 1),
		VideoID:   "v1",
		ChapterID: &chapterID,
		Kind:      entity.TaskKindImage,
		Ordinal:   7,
		Index:     1,
		State:     entity.TaskStateRunning,
		Pool:      entity.PoolImage,
		Attempt:   1,
		UpdatedAt: now,
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := json.Marshal(&delta); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGraphDownstream(b *testing.B) {
	g := benchGraph(b)
	root := g.Roots()[0]
	b.ReportAllocs()
	for b.Loop() {
		if len(g.Downstream(root)) != g.NodeCount() {
			b.Fatal("unexpected reach")
		}
	}
}
