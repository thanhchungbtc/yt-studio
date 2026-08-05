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

// ErrAlreadyExpanded is returned when a video's per-chapter body is spliced on
// twice with two different shapes. Splicing the same tail twice is a no-op.
var ErrAlreadyExpanded = errors.New("video graph is already expanded")

// HeadSpec describes the graph a video is enqueued with: the blueprint alone.
//
// The rest of the DAG is not describable yet. Its shape is one branch per
// chapter, and the chapter count is the blueprint's output rather than its
// input — a video briefed for 50 chapters legitimately comes back with 45, and
// that is the number the operator approves.
type HeadSpec struct {
	VideoID     entity.VideoID
	MaxAttempts int
	// BlueprintGate parks the pipeline after the blueprint for human review.
	BlueprintGate bool
	Now           time.Time
}

// BuildSpec describes the DAG to construct for one video.
type BuildSpec struct {
	VideoID          entity.VideoID
	ChapterCount     int
	SlidesPerChapter int
	// ThumbnailCells is the width of the thumbnail's grid, and therefore how many
	// icon tasks the graph holds. It is independent of the chapters: the grid is
	// its own artifact, not a view onto them.
	ThumbnailCells int
	MaxAttempts    int
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
// prime + concat + metadata + thumbnail plan + thumbnail + upload, plus four
// per-chapter tasks, one per slide and one per thumbnail cell. It is exported
// so callers can preallocate too.
func NodeCountFor(chapters, slidesPerChapter, thumbnailCells int) int {
	return 7 + 4*chapters + chapters*slidesPerChapter + thumbnailCells
}

// newGraph allocates an empty graph sized for total nodes.
func newGraph(videoID entity.VideoID, total int) *Graph {
	return &Graph{
		VideoID:      videoID,
		tasks:        make([]entity.Task, 0, total),
		dependents:   make([][]int32, 0, total),
		dependencies: make([][]int32, 0, total),
		byID:         make(map[entity.TaskID]int32, total),
		generations:  make([]uint64, 0, total),
	}
}

// addNode appends one task and returns its index. Both BuildGraph and
// BuildHeadGraph go through it, so the blueprint node a head graph starts life
// with is identical to the one the full builder would have produced.
func (g *Graph) addNode(kind entity.TaskKind, ordinal, index int, gate entity.GateKind, maxAttempts int, now time.Time) int32 {
	id := entity.NewTaskID(g.VideoID, kind, ordinal, index)
	var chapterID *entity.ChapterID
	if ordinal >= 0 {
		c := entity.NewChapterID(g.VideoID, ordinal)
		chapterID = &c
	}
	idx := int32(len(g.tasks)) //nolint:gosec // bounded by MaxChapterCount
	g.tasks = append(g.tasks, entity.Task{
		ID:          id,
		VideoID:     g.VideoID,
		ChapterID:   chapterID,
		Kind:        kind,
		Ordinal:     ordinal,
		Index:       index,
		State:       entity.TaskStateBlocked,
		Pool:        kind.Pool(),
		Gate:        gate,
		MaxAttempts: maxAttempts,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	g.dependents = append(g.dependents, nil)
	g.dependencies = append(g.dependencies, nil)
	g.generations = append(g.generations, 0)
	g.byID[id] = idx
	return idx
}

// linkNodes records one arc and charges it to the dependent's outstanding count.
func (g *Graph) linkNodes(from, to int32) {
	g.dependents[from] = append(g.dependents[from], to)
	g.dependencies[to] = append(g.dependencies[to], from)
	g.tasks[to].DepsRemaining++
}

// normaliseAttempts floors the retry budget at one attempt, so a zero-valued
// spec still runs each task once rather than never.
func normaliseAttempts(maxAttempts int) int {
	if maxAttempts < 1 {
		return 1
	}
	return maxAttempts
}

// BuildHeadGraph constructs the graph a video is enqueued with: a single
// blueprint node.
//
// Everything downstream is one branch per chapter, and there is no honest
// chapter count until the blueprint has been written and accepted. Committing
// to the number a video was briefed with would mean rejecting a blueprint that
// came back with 45 chapters instead of 50, which is a perfectly good blueprint.
func BuildHeadGraph(spec HeadSpec) (*Graph, error) {
	if spec.VideoID == "" {
		return nil, fmt.Errorf("%w: video id must not be empty", ErrInvalidGraph)
	}
	gate := entity.GateNone
	if spec.BlueprintGate {
		gate = entity.GateBlueprint
	}
	g := newGraph(spec.VideoID, 1)
	g.addNode(entity.TaskKindBlueprint, -1, -1, gate, normaliseAttempts(spec.MaxAttempts), spec.Now)
	return g, nil
}

// Tail is a video's per-chapter body: every node of its DAG except the
// blueprint, plus every arc, including the ones reaching back to the blueprint.
//
// It is a plain value rather than a Graph because of where it is built. The
// chapter count comes from a database read and the rows are persisted before
// the loop ever sees them, so the tail is assembled off the dispatch goroutine
// and handed in to be spliced.
type Tail struct {
	Tasks []entity.Task
	Edges []repository.TaskEdge
}

// BuildTail constructs the per-chapter body for a chapter count that is now
// known.
//
// It derives the tail from the canonical full graph rather than rebuilding the
// topology a second time: a head graph plus its tail is BuildGraph's output
// node for node, and TestHeadPlusTailEqualsBuildGraph holds that to be true.
func BuildTail(spec BuildSpec) (Tail, error) {
	full, err := BuildGraph(spec)
	if err != nil {
		return Tail{}, err
	}
	// Node 0 is the blueprint by construction; the head graph already holds it.
	return Tail{Tasks: full.tasks[1:], Edges: full.Edges()}, nil
}

// Expanded reports whether the per-chapter body has been spliced on. An
// unexpanded graph is a lone blueprint node.
func (g *Graph) Expanded() bool { return len(g.tasks) > 1 }

// Expand splices a video's per-chapter body onto its head graph, once the
// blueprint has said how many chapters there are.
//
// It is called by the dispatch loop and only by the dispatch loop. Appending to
// g.tasks is safe despite the ready set holding raw pointers into that slice,
// because an unexpanded graph can have no entry in it: its single node is
// expanded while the blueprint is running or parked on its gate, and a task in
// either state has already been popped.
//
// No spliced node is runnable on arrival — every one of them depends on the
// blueprint transitively — so Expand never has to admit anything to the ready
// set, which is what keeps it a pure graph operation.
func (g *Graph) Expand(tail Tail) error {
	if len(g.tasks) != 1 {
		return fmt.Errorf("%w: expand wants a head graph, %s has %d nodes",
			ErrInvalidGraph, g.VideoID, len(g.tasks))
	}
	if len(tail.Tasks) == 0 {
		return fmt.Errorf("%w: empty tail for %s", ErrInvalidGraph, g.VideoID)
	}
	if head := g.tasks[0]; head.Kind != entity.TaskKindBlueprint {
		return fmt.Errorf("%w: head node of %s is %s, not a blueprint",
			ErrInvalidGraph, g.VideoID, head.Kind)
	}
	if g.tasks[0].State == entity.TaskStateSucceeded {
		// The blueprint has already released. Nothing would ever wake a tail spliced
		// on after that moment, so refuse rather than strand it.
		return fmt.Errorf("%w: blueprint of %s has already released its dependents",
			ErrInvalidGraph, g.VideoID)
	}

	for _, t := range tail.Tasks {
		if _, dup := g.byID[t.ID]; dup {
			return fmt.Errorf("%w: tail repeats task %s", ErrInvalidGraph, t.ID)
		}
		// Outstanding counts are recomputed from the arcs below, so a tail built
		// against one topology cannot import a count belonging to another.
		t.DepsRemaining = 0
		idx := int32(len(g.tasks)) //nolint:gosec // bounded by MaxChapterCount
		g.tasks = append(g.tasks, t)
		g.dependents = append(g.dependents, nil)
		g.dependencies = append(g.dependencies, nil)
		g.generations = append(g.generations, 0)
		g.byID[t.ID] = idx
	}
	for _, e := range tail.Edges {
		from, ok := g.byID[e.From]
		if !ok {
			return fmt.Errorf("%w: edge references unknown task %s", ErrInvalidGraph, e.From)
		}
		to, ok := g.byID[e.To]
		if !ok {
			return fmt.Errorf("%w: edge references unknown task %s", ErrInvalidGraph, e.To)
		}
		g.linkNodes(from, to)
	}
	return g.assertAcyclic()
}

// BuildGraph constructs a video's DAG.
//
// The structurally important detail: slide prompts depend on the blueprint
// alone, not on the chapter script. That gives the graph two independent
// branches and is what makes the slide pipeline — the longest pole — start
// early.
func BuildGraph(spec BuildSpec) (*Graph, error) {
	if spec.VideoID == "" {
		return nil, fmt.Errorf("%w: video id must not be empty", ErrInvalidGraph)
	}
	if spec.ChapterCount < entity.MinChapterCount || spec.ChapterCount > entity.MaxChapterCount {
		return nil, fmt.Errorf("%w: chapter count must be %d..%d, got %d",
			ErrInvalidGraph, entity.MinChapterCount, entity.MaxChapterCount, spec.ChapterCount)
	}
	if spec.SlidesPerChapter < entity.MinSlidesPerChapter || spec.SlidesPerChapter > entity.MaxSlidesPerChapter {
		return nil, fmt.Errorf("%w: slides per chapter must be %d..%d, got %d",
			ErrInvalidGraph, entity.MinSlidesPerChapter, entity.MaxSlidesPerChapter, spec.SlidesPerChapter)
	}
	if spec.ThumbnailCells < entity.MinThumbnailCells || spec.ThumbnailCells > entity.MaxThumbnailCells {
		return nil, fmt.Errorf("%w: thumbnail cells must be %d..%d, got %d",
			ErrInvalidGraph, entity.MinThumbnailCells, entity.MaxThumbnailCells, spec.ThumbnailCells)
	}
	maxAttempts := normaliseAttempts(spec.MaxAttempts)

	n := spec.ChapterCount
	m := spec.SlidesPerChapter
	cells := spec.ThumbnailCells

	g := newGraph(spec.VideoID, NodeCountFor(n, m, cells))

	add := func(kind entity.TaskKind, ordinal, index int, gate entity.GateKind) int32 {
		return g.addNode(kind, ordinal, index, gate, maxAttempts, spec.Now)
	}
	link := g.linkNodes

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
	prime := add(entity.TaskKindPrimeSlidePrompts, -1, -1, entity.GateNone)

	prompts := make([]int32, n)
	for i := range n {
		prompts[i] = add(entity.TaskKindSlidePrompts, i+1, -1, entity.GateNone)
	}
	scripts := make([]int32, n)
	for i := range n {
		scripts[i] = add(entity.TaskKindScript, i+1, -1, entity.GateNone)
	}
	tts := make([]int32, n)
	for i := range n {
		tts[i] = add(entity.TaskKindTTS, i+1, -1, entity.GateNone)
	}
	slides := make([][]int32, n)
	for i := range n {
		slides[i] = make([]int32, m)
		for j := range m {
			slides[i][j] = add(entity.TaskKindSlide, i+1, j, entity.GateNone)
		}
	}
	clips := make([]int32, n)
	for i := range n {
		clips[i] = add(entity.TaskKindClip, i+1, -1, entity.GateNone)
	}
	concat := add(entity.TaskKindConcat, -1, -1, entity.GateNone)
	metadata := add(entity.TaskKindMetadata, -1, -1, entity.GateNone)
	plan := add(entity.TaskKindThumbnailPlan, -1, -1, entity.GateNone)
	// One icon per cell, indexed like a chapter's slides are. The width is fixed
	// here and cannot grow later, which is why the plan's cell count is a
	// contract rather than a target.
	icons := make([]int32, cells)
	for i := range cells {
		icons[i] = add(entity.TaskKindThumbnailIcon, -1, i, entity.GateNone)
	}
	// The thumbnail task carries the upload gate: on success it parks in
	// awaiting_approval and does not release the upload task.
	//
	// The gate sits on the last node before the upload rather than on the
	// metadata that opens the listing, because what the operator is approving is
	// publication — and the thumbnail is the part of the listing they most need
	// to have seen before it goes out.
	thumbnail := add(entity.TaskKindThumbnail, -1, -1, uploadGate)
	upload := add(entity.TaskKindUpload, -1, -1, entity.GateNone)

	link(blueprint, prime)
	for i := range n {
		link(blueprint, scripts[i])
		link(prime, prompts[i])
		link(scripts[i], tts[i])
		link(tts[i], clips[i])
		for j := range m {
			link(prompts[i], slides[i][j])
			link(slides[i][j], clips[i])
		}
		link(clips[i], concat)
	}
	link(concat, metadata)
	// The grid is planned from the listing the metadata just wrote, drawn one
	// cell at a time, and only then composed.
	link(metadata, plan)
	for i := range cells {
		link(plan, icons[i])
		link(icons[i], thumbnail)
	}
	link(thumbnail, upload)

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
