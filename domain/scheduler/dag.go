// Package scheduler owns dispatch: the per-video DAG, the global pools, the
// in-memory ready set and the event-driven loop that ties them together.
//
// We own this outright. Every off-the-shelf option costs a server process,
// which breaks the single-binary principle; `semaphore.Weighted` supplies the
// concurrency control an engine would have supplied.
package scheduler

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// ErrInvalidGraph is returned by BuildGraph for an unusable specification.
var ErrInvalidGraph = errors.New("invalid graph")

// ErrCycle is returned by BuildGraph if the constructed graph is not acyclic.
// It exists to make the invariant checkable rather than assumed.
var ErrCycle = errors.New("graph contains a cycle")

// BuildSpec describes the DAG to construct for one video.
type BuildSpec struct {
	VideoID          entity.VideoID
	ChapterCount     int
	ImagesPerChapter int
	MaxAttempts      int
	// BlueprintGate parks the pipeline after the blueprint for human review;
	// UploadGate parks it before upload. Both are settings rows.
	BlueprintGate bool
	UploadGate    bool
	Now           time.Time
}

// Graph is one video's precomputed dependency graph. It is built once at
// enqueue time and never rebuilt: the scheduler answers "what can run now?"
// from memory, never from a query.
type Graph struct {
	VideoID entity.VideoID

	// tasks is the authoritative in-memory copy. The ready set holds pointers into
	// this slice, so it is allocated once at full size and never appended to
	// afterwards — the pointers must stay valid.
	tasks []entity.Task
	// dependents[i] lists the nodes that i releases when it succeeds.
	dependents [][]int32
	// dependencies[i] lists the nodes i waits on. It is what a retry cascade and a
	// recovery consistency check walk.
	dependencies [][]int32
	byID         map[entity.TaskID]int32

	// generations[i] counts how many times node i has been reset. A dispatch
	// carries the generation it started under and its completion is discarded if
	// they no longer match, which is what stops a task that was in flight when its
	// input was retried from landing an answer to the old question.
	//
	// It is deliberately not persisted: a restart reclaims every in-flight task as
	// ready, so there is no completion left to disambiguate.
	generations []uint64

	// installed flips once the dispatch loop owns this graph. After that the loop
	// mutates tasks freely, so no other goroutine may read the slice — which is
	// what makes a repeated Submit a cheap no-op rather than a race.
	installed atomic.Bool
}

// Installed reports whether the dispatch loop has taken ownership of the graph.
func (g *Graph) Installed() bool { return g.installed.Load() }

// markInstalled is called by the loop, and only by the loop.
func (g *Graph) markInstalled() { g.installed.Store(true) }

// NodeCount returns the number of tasks in the graph.
func (g *Graph) NodeCount() int { return len(g.tasks) }

// Task returns a stable pointer to the task at index i.
func (g *Graph) Task(i int) *entity.Task { return &g.tasks[i] }

// Tasks returns the graph's tasks in canonical order, for persistence.
func (g *Graph) Tasks() []entity.Task { return g.tasks }

// IndexOf resolves a task id to its node index.
func (g *Graph) IndexOf(id entity.TaskID) (int32, bool) {
	i, ok := g.byID[id]
	return i, ok
}

// TaskByID returns a stable pointer to a task by id.
func (g *Graph) TaskByID(id entity.TaskID) (*entity.Task, bool) {
	i, ok := g.byID[id]
	if !ok {
		return nil, false
	}
	return &g.tasks[i], true
}

// Generation returns the current dispatch generation of node i.
func (g *Graph) Generation(i int32) uint64 { return g.generations[i] }

// bumpGeneration invalidates any dispatch of node i that is still in flight.
func (g *Graph) bumpGeneration(i int32) { g.generations[i]++ }

// Dependents returns the node indices released when i succeeds.
func (g *Graph) Dependents(i int32) []int32 { return g.dependents[i] }

// Edges materialises the arcs for persistence. It allocates, and is called once
// per video at enqueue time — never on the dispatch path.
func (g *Graph) Edges() []repository.TaskEdge {
	n := 0
	for _, d := range g.dependents {
		n += len(d)
	}
	edges := make([]repository.TaskEdge, 0, n)
	for from, deps := range g.dependents {
		for _, to := range deps {
			edges = append(edges, repository.TaskEdge{
				VideoID: g.VideoID,
				From:    g.tasks[from].ID,
				To:      g.tasks[to].ID,
			})
		}
	}
	return edges
}

// NodeCountFor returns the exact number of tasks a spec produces: blueprint +
// prime + concat + metadata + upload, plus four per-chapter tasks and one per
// still. It is exported so callers can preallocate too.
func NodeCountFor(chapters, imagesPerChapter int) int {
	return 5 + 4*chapters + chapters*imagesPerChapter
}

// BuildGraph constructs a video's DAG.
//
// The structurally important detail: image prompts depend on the blueprint
// alone, not on the chapter script. That gives the graph two independent
// branches and is what makes the image pipeline — the longest pole — start
// early.
func BuildGraph(spec BuildSpec) (*Graph, error) {
	if spec.VideoID == "" {
		return nil, fmt.Errorf("%w: video id must not be empty", ErrInvalidGraph)
	}
	if spec.ChapterCount < entity.MinChapterCount || spec.ChapterCount > entity.MaxChapterCount {
		return nil, fmt.Errorf("%w: chapter count must be %d..%d, got %d",
			ErrInvalidGraph, entity.MinChapterCount, entity.MaxChapterCount, spec.ChapterCount)
	}
	if spec.ImagesPerChapter < entity.MinImagesPerChapter || spec.ImagesPerChapter > entity.MaxImagesPerChapter {
		return nil, fmt.Errorf("%w: images per chapter must be %d..%d, got %d",
			ErrInvalidGraph, entity.MinImagesPerChapter, entity.MaxImagesPerChapter, spec.ImagesPerChapter)
	}
	maxAttempts := spec.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	n := spec.ChapterCount
	m := spec.ImagesPerChapter
	total := NodeCountFor(n, m)

	g := &Graph{
		VideoID:      spec.VideoID,
		tasks:        make([]entity.Task, 0, total),
		dependents:   make([][]int32, 0, total),
		dependencies: make([][]int32, 0, total),
		byID:         make(map[entity.TaskID]int32, total),
		generations:  make([]uint64, total),
	}

	add := func(kind entity.TaskKind, ordinal, index int, gate entity.GateKind) int32 {
		id := entity.NewTaskID(spec.VideoID, kind, ordinal, index)
		var chapterID *entity.ChapterID
		if ordinal >= 0 {
			c := entity.NewChapterID(spec.VideoID, ordinal)
			chapterID = &c
		}
		idx := int32(len(g.tasks)) //nolint:gosec // bounded by MaxChapterCount
		g.tasks = append(g.tasks, entity.Task{
			ID:          id,
			VideoID:     spec.VideoID,
			ChapterID:   chapterID,
			Kind:        kind,
			Ordinal:     ordinal,
			Index:       index,
			State:       entity.TaskStateBlocked,
			Pool:        kind.Pool(),
			Gate:        gate,
			MaxAttempts: maxAttempts,
			CreatedAt:   spec.Now,
			UpdatedAt:   spec.Now,
		})
		g.dependents = append(g.dependents, nil)
		g.dependencies = append(g.dependencies, nil)
		g.byID[id] = idx
		return idx
	}

	link := func(from, to int32) {
		g.dependents[from] = append(g.dependents[from], to)
		g.dependencies[to] = append(g.dependencies[to], from)
		g.tasks[to].DepsRemaining++
	}

	blueprintGate := entity.GateNone
	if spec.BlueprintGate {
		blueprintGate = entity.GateBlueprint
	}
	uploadGate := entity.GateNone
	if spec.UploadGate {
		uploadGate = entity.GateUpload
	}

	// Canonical node order. It is deterministic so that golden-file tests over the
	// dispatch sequence stay stable.
	blueprint := add(entity.TaskKindBlueprint, -1, -1, blueprintGate)
	prime := add(entity.TaskKindPrimeImagePrompts, -1, -1, entity.GateNone)

	prompts := make([]int32, n)
	for i := range n {
		prompts[i] = add(entity.TaskKindImagePrompts, i+1, -1, entity.GateNone)
	}
	scripts := make([]int32, n)
	for i := range n {
		scripts[i] = add(entity.TaskKindScript, i+1, -1, entity.GateNone)
	}
	tts := make([]int32, n)
	for i := range n {
		tts[i] = add(entity.TaskKindTTS, i+1, -1, entity.GateNone)
	}
	images := make([][]int32, n)
	for i := range n {
		images[i] = make([]int32, m)
		for j := range m {
			images[i][j] = add(entity.TaskKindImage, i+1, j, entity.GateNone)
		}
	}
	clips := make([]int32, n)
	for i := range n {
		clips[i] = add(entity.TaskKindClip, i+1, -1, entity.GateNone)
	}
	concat := add(entity.TaskKindConcat, -1, -1, entity.GateNone)
	// The metadata task carries the upload gate: on success it parks in
	// awaiting_approval and does not release the upload task.
	metadata := add(entity.TaskKindMetadata, -1, -1, uploadGate)
	upload := add(entity.TaskKindUpload, -1, -1, entity.GateNone)

	link(blueprint, prime)
	for i := range n {
		link(blueprint, scripts[i])
		link(prime, prompts[i])
		link(scripts[i], tts[i])
		link(tts[i], clips[i])
		for j := range m {
			link(prompts[i], images[i][j])
			link(images[i][j], clips[i])
		}
		link(clips[i], concat)
	}
	link(concat, metadata)
	link(metadata, upload)

	if err := g.assertAcyclic(); err != nil {
		return nil, err
	}
	return g, nil
}

// assertAcyclic runs Kahn's algorithm over a copy of the in-degrees. It is
// cheap next to graph construction and turns "the DAG is acyclic" from a
// comment into a checked invariant.
func (g *Graph) assertAcyclic() error {
	indeg := make([]int32, len(g.tasks))
	for i := range g.tasks {
		indeg[i] = int32(len(g.dependencies[i])) //nolint:gosec // bounded
	}
	queue := make([]int32, 0, len(g.tasks))
	for i := range indeg {
		if indeg[i] == 0 {
			queue = append(queue, int32(i)) //nolint:gosec // bounded
		}
	}
	visited := 0
	for len(queue) > 0 {
		cur := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		visited++
		for _, d := range g.dependents[cur] {
			indeg[d]--
			if indeg[d] == 0 {
				queue = append(queue, d)
			}
		}
	}
	if visited != len(g.tasks) {
		return fmt.Errorf("%w: visited %d of %d nodes", ErrCycle, visited, len(g.tasks))
	}
	return nil
}

// Roots returns the indices of tasks with no dependencies. For a fresh video
// that is exactly the blueprint.
func (g *Graph) Roots() []int32 {
	roots := make([]int32, 0, 1)
	for i := range g.tasks {
		if len(g.dependencies[i]) == 0 {
			roots = append(roots, int32(i)) //nolint:gosec // bounded
		}
	}
	return roots
}

// Downstream returns i plus every node reachable from it, in ascending index
// order. It is what a chapter retry resets.
func (g *Graph) Downstream(i int32) []int32 {
	seen := make([]bool, len(g.tasks))
	stack := make([]int32, 0, 16)
	stack = append(stack, i)
	seen[i] = true
	out := make([]int32, 0, 16)
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		out = append(out, cur)
		for _, d := range g.dependents[cur] {
			if !seen[d] {
				seen[d] = true
				stack = append(stack, d)
			}
		}
	}
	// Ascending order keeps resets deterministic.
	for a := 1; a < len(out); a++ {
		v := out[a]
		b := a - 1
		for b >= 0 && out[b] > v {
			out[b+1] = out[b]
			b--
		}
		out[b+1] = v
	}
	return out
}

// GraphFromPersisted rebuilds an in-memory graph from rows read at startup, so
// a crash 45 minutes into a run resumes rather than restarts.
func GraphFromPersisted(vg repository.VideoGraph) (*Graph, error) {
	if len(vg.Tasks) == 0 {
		return nil, fmt.Errorf("%w: no tasks for video %s", ErrInvalidGraph, vg.VideoID)
	}
	g := &Graph{
		VideoID:      vg.VideoID,
		tasks:        make([]entity.Task, len(vg.Tasks)),
		dependents:   make([][]int32, len(vg.Tasks)),
		dependencies: make([][]int32, len(vg.Tasks)),
		byID:         make(map[entity.TaskID]int32, len(vg.Tasks)),
		generations:  make([]uint64, len(vg.Tasks)),
	}
	copy(g.tasks, vg.Tasks)
	for i := range g.tasks {
		g.byID[g.tasks[i].ID] = int32(i) //nolint:gosec // bounded
	}
	for _, e := range vg.Edges {
		from, ok := g.byID[e.From]
		if !ok {
			return nil, fmt.Errorf("%w: edge references unknown task %s", ErrInvalidGraph, e.From)
		}
		to, ok := g.byID[e.To]
		if !ok {
			return nil, fmt.Errorf("%w: edge references unknown task %s", ErrInvalidGraph, e.To)
		}
		g.dependents[from] = append(g.dependents[from], to)
		g.dependencies[to] = append(g.dependencies[to], from)
	}
	if err := g.assertAcyclic(); err != nil {
		return nil, err
	}
	return g, nil
}
