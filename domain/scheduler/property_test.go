package scheduler

import (
	"errors"
	"sync"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/tbui/yt-studio/domain/entity"
)

// Property-based tests over the scheduler's invariants (§8.4):
//
//   - never exceed the budget
//   - never deadlock
//   - never start a task before its dependencies
//   - never lose a task
//
// Each generated case runs a real scheduler against a synthetic runner, so the
// properties are checked against the actual dispatch loop rather than a model
// of it.
func TestSchedulerInvariants(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		chapters := rapid.IntRange(1, 6).Draw(rt, "chapters")
		images := rapid.IntRange(1, 3).Draw(rt, "images")
		gates := rapid.Bool().Draw(rt, "gates")
		limits := map[entity.Pool]int{
			entity.PoolLLM:     rapid.IntRange(1, 4).Draw(rt, "llm"),
			entity.PoolTTS:     rapid.IntRange(1, 4).Draw(rt, "tts"),
			entity.PoolImage:   rapid.IntRange(1, 4).Draw(rt, "image"),
			entity.PoolCompose: rapid.IntRange(1, 4).Draw(rt, "compose"),
			entity.PoolCache:   rapid.IntRange(1, 8).Draw(rt, "cache"),
			entity.PoolUpload:  1,
		}
		// A deterministic subset of tasks fails transiently exactly once, so the
		// retry path is inside the property rather than beside it. Failing more
		// than once per task could exhaust MaxAttempts, which would make the
		// property unsatisfiable rather than falsified.
		failEvery := rapid.IntRange(0, 7).Draw(rt, "failEvery")

		g := testGraph(t, "v1", chapters, images, gates)
		var (
			mu     sync.Mutex
			burned = map[entity.TaskID]bool{}
			seen   int
		)
		r := newRig(t, limits, g, func(task entity.Task) entity.TaskOutcome {
			mu.Lock()
			seen++
			shouldFail := failEvery > 0 && seen%failEvery == 0 && !burned[task.ID]
			if shouldFail {
				burned[task.ID] = true
			}
			mu.Unlock()
			if shouldFail {
				return entity.Failed{Err: errors.New("synthetic"), Retryable: true}
			}
			return entity.Success{}
		})
		ctx := startScheduler(t, r.sched)

		if err := r.sched.Submit(ctx, g); err != nil {
			rt.Fatalf("Submit: %v", err)
		}

		approve := func(kind entity.TaskKind) {
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				if r.sched.Snapshot().AwaitingApproval > 0 {
					if err := r.sched.Approve(ctx, entity.NewTaskID("v1", kind, -1, -1)); err == nil {
						return
					}
				}
				time.Sleep(time.Millisecond)
			}
			rt.Fatalf("gate for %s never opened", kind)
		}
		if gates {
			approve(entity.TaskKindBlueprint)
			approve(entity.TaskKindMetadata)
		}

		// Never deadlocks: the whole graph reaches a terminal state.
		deadline := time.Now().Add(20 * time.Second)
		for {
			snap := r.sched.Snapshot()
			if snap.Succeeded == g.NodeCount() {
				break
			}
			if time.Now().After(deadline) {
				rt.Fatalf("deadlock: %d/%d succeeded, ready=%d running=%d blocked=%d retry=%d",
					snap.Succeeded, g.NodeCount(), snap.Ready, snap.Running, snap.Blocked, snap.RetryPending)
			}
			time.Sleep(time.Millisecond)
		}

		// Never starts a task before its dependencies.
		if v := r.order.failure(); v != "" {
			rt.Fatalf("dependency order violated: %s", v)
		}
		// Never exceeds the budget.
		for pool, limit := range limits {
			if peak := r.watcher.peakOf(pool); peak > limit {
				rt.Fatalf("pool %s peaked at %d, limit is %d", pool, peak, limit)
			}
		}
		// Never loses a task: every node is durably recorded as succeeded.
		for i := range g.NodeCount() {
			if got := r.store.state(g.Task(i).ID); got != entity.TaskStateSucceeded {
				rt.Fatalf("%s persisted as %q", g.Task(i).ID, got)
			}
		}
	})
}

// The ready set is the authoritative dispatch structure, so its own invariants
// are worth checking exhaustively and cheaply.
func TestReadySetIsFIFOPerPool(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		n := rapid.IntRange(0, 400).Draw(rt, "n")
		tasks := make([]entity.Task, n)
		expected := map[entity.Pool][]entity.TaskID{}
		rs := NewReadySet()

		for i := range n {
			pool := entity.AllPools[rapid.IntRange(0, entity.NumPools-1).Draw(rt, "pool")]
			tasks[i] = entity.Task{
				ID:    (entity.NewTaskID("v", entity.TaskKindImage, i, 0)),
				Pool:  pool,
				State: entity.TaskStateReady,
			}
			rs.Push(&tasks[i])
			expected[pool] = append(expected[pool], tasks[i].ID)
		}
		if got := rs.Total(); got != n {
			rt.Fatalf("total = %d, want %d", got, n)
		}
		for pool, want := range expected {
			for _, id := range want {
				got := rs.Next(pool)
				if got == nil {
					rt.Fatalf("pool %s ran dry early", pool)
					return
				}
				if got.ID != id {
					rt.Fatalf("pool %s popped %q, want %q", pool, got.ID, id)
				}
				rs.Pop(pool)
			}
			if rs.Next(pool) != nil {
				rt.Fatalf("pool %s still has entries", pool)
			}
		}
	})
}

// Entries whose state moved on are dropped rather than dispatched, which is how
// a cancelled video's queued tasks disappear without a set lookup on the hot
// path.
func TestReadySetDropsStaleEntries(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		n := rapid.IntRange(1, 200).Draw(rt, "n")
		cancelEvery := rapid.IntRange(1, 5).Draw(rt, "cancelEvery")
		tasks := make([]entity.Task, n)
		rs := NewReadySet()
		wantAlive := 0
		for i := range n {
			tasks[i] = entity.Task{
				ID:    (entity.NewTaskID("v", entity.TaskKindImage, i, 0)),
				Pool:  entity.PoolImage,
				State: entity.TaskStateReady,
			}
			rs.Push(&tasks[i])
			if i%cancelEvery == 0 {
				tasks[i].State = entity.TaskStateCancelled
			} else {
				wantAlive++
			}
		}
		got := 0
		for rs.Next(entity.PoolImage) != nil {
			rs.Pop(entity.PoolImage)
			got++
		}
		if got != wantAlive {
			rt.Fatalf("dispatched %d tasks, want %d live ones", got, wantAlive)
		}
	})
}

// The retry heap must always hand back the earliest due item.
func TestRetryQueueOrdering(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		n := rapid.IntRange(0, 200).Draw(rt, "n")
		base := time.Unix(1_700_000_000, 0)
		var q retryQueue
		for i := range n {
			off := rapid.IntRange(0, 10_000).Draw(rt, "offset")
			q.push(retryItem{
				taskID: (entity.NewTaskID("v", entity.TaskKindTTS, i, -1)),
				when:   base.Add(time.Duration(off) * time.Millisecond),
			})
		}
		if q.len() != n {
			rt.Fatalf("len = %d, want %d", q.len(), n)
		}
		last := base.Add(-time.Second)
		popped := 0
		for {
			item, ok := q.popDue(base.Add(24 * time.Hour))
			if !ok {
				break
			}
			if item.when.Before(last) {
				rt.Fatalf("out of order: %s after %s", item.when, last)
			}
			last = item.when
			popped++
		}
		if popped != n {
			rt.Fatalf("popped %d, want %d", popped, n)
		}
		if _, ok := q.earliest(); ok && n > 0 {
			rt.Fatal("queue should be empty")
		}
	})
}

// popDue must never return an item that is not yet due.
func TestRetryQueueRespectsDueTime(t *testing.T) {
	t.Parallel()
	base := time.Unix(1_700_000_000, 0)
	var q retryQueue
	q.push(retryItem{taskID: "a", when: base.Add(time.Second)})
	q.push(retryItem{taskID: "b", when: base.Add(2 * time.Second)})

	if _, ok := q.popDue(base); ok {
		t.Fatal("popped an item before it was due")
	}
	when, ok := q.earliest()
	if !ok || !when.Equal(base.Add(time.Second)) {
		t.Fatalf("earliest = %v (ok=%v), want %v", when, ok, base.Add(time.Second))
	}
	item, ok := q.popDue(base.Add(time.Second))
	if !ok || item.taskID != "a" {
		t.Fatalf("popDue = %v (ok=%v), want a", item.taskID, ok)
	}
}

// BuildGraph must be acyclic and internally consistent for every shape.
func TestGraphIsAlwaysConsistent(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		chapters := rapid.IntRange(1, 60).Draw(rt, "chapters")
		images := rapid.IntRange(1, 5).Draw(rt, "images")
		g, err := BuildGraph(BuildSpec{
			VideoID:          "v1",
			ChapterCount:     chapters,
			ImagesPerChapter: images,
			MaxAttempts:      3,
			BlueprintGate:    rapid.Bool().Draw(rt, "blueprintGate"),
			UploadGate:       rapid.Bool().Draw(rt, "uploadGate"),
			Now:              time.Unix(0, 0),
		})
		if err != nil {
			rt.Fatalf("BuildGraph: %v", err)
		}
		if got, want := g.NodeCount(), NodeCountFor(chapters, images); got != want {
			rt.Fatalf("node count = %d, want %d", got, want)
		}
		// Every dependent arc has a matching dependency arc.
		for i := range g.NodeCount() {
			for _, d := range g.dependents[i] {
				found := false
				for _, back := range g.dependencies[d] {
					if int(back) == i {
						found = true
						break
					}
				}
				if !found {
					rt.Fatalf("arc %d->%d has no reverse edge", i, d)
				}
			}
		}
		if err := g.assertAcyclic(); err != nil {
			rt.Fatalf("graph is not acyclic: %v", err)
		}
		// The whole graph is reachable from the single root.
		roots := g.Roots()
		if len(roots) != 1 {
			rt.Fatalf("roots = %d, want 1", len(roots))
		}
		if got := len(g.Downstream(roots[0])); got != g.NodeCount() {
			rt.Fatalf("reachable = %d, want %d", got, g.NodeCount())
		}
	})
}

// Persisting and reloading a graph must preserve its structure exactly, which
// is the precondition for crash recovery.
func TestGraphSurvivesPersistence(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		chapters := rapid.IntRange(1, 20).Draw(rt, "chapters")
		images := rapid.IntRange(1, 3).Draw(rt, "images")
		original := testGraph(t, "v1", chapters, images, rapid.Bool().Draw(rt, "gates"))

		restored, err := GraphFromPersisted(persistedFrom(original))
		if err != nil {
			rt.Fatalf("GraphFromPersisted: %v", err)
		}
		if restored.NodeCount() != original.NodeCount() {
			rt.Fatalf("node count = %d, want %d", restored.NodeCount(), original.NodeCount())
		}
		for i := range original.NodeCount() {
			a, b := original.Task(i), restored.Task(i)
			if a.ID != b.ID || a.Kind != b.Kind || a.Pool != b.Pool || a.Gate != b.Gate {
				rt.Fatalf("node %d differs: %+v vs %+v", i, a, b)
			}
			if len(original.dependents[i]) != len(restored.dependents[i]) {
				rt.Fatalf("node %d dependents differ", i)
			}
		}
	})
}
