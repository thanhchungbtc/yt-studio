// Package scheduler owns dispatch: the per-video DAG, the global pools, the
// in-memory ready set and the event-driven loop that ties them together. It is
// hand-written because every off-the-shelf engine costs a second process.
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

// ErrCycle makes "the graph is acyclic" a checked invariant rather than an
// assumed one.
var ErrCycle = errors.New("graph contains a cycle")

// ErrAlreadyExpanded is returned when a video's per-chapter body is spliced on
// twice with two different shapes. Splicing the same tail twice is a no-op.
var ErrAlreadyExpanded = errors.New("video graph is already expanded")

// HeadSpec describes the graph a video is enqueued with: the blueprint alone.
// The rest is one branch per chapter, and the chapter count is the blueprint's
// output rather than its input.
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
	// ThumbnailCells is the grid width, and so how many icon tasks the graph
	// holds. Independent of the chapters: the grid is its own artifact.
	ThumbnailCells int
	MaxAttempts    int
	// BlueprintGate parks the pipeline after the blueprint for human review;
	// UploadGate parks it before upload. Both are settings rows.
	BlueprintGate bool
	UploadGate    bool
	Now           time.Time
}

// Graph is one video's dependency graph, built once at enqueue time: the
// scheduler answers "what can run now?" from memory, never from a query.
type Graph struct {
	VideoID entity.VideoID

	// tasks is the authoritative copy. The ready set holds pointers into this
	// slice, so it is sized once and the pointers must stay valid.
	tasks []entity.Task
	// dependents[i] lists the nodes that i releases when it succeeds.
	dependents [][]int32
	// dependencies[i] lists the nodes i waits on; a retry cascade walks it.
	dependencies [][]int32
	byID         map[entity.TaskID]int32

	// generations[i] counts resets of node i. A dispatch carries the generation it
	// started under and its completion is discarded on mismatch, so a task in
	// flight when its input was retried cannot answer the old question. Not
	// persisted: a restart reclaims every in-flight task as ready.
	generations []uint64

	// installed flips once the dispatch loop owns this graph and may mutate tasks
	// freely, which makes a repeated Submit a no-op rather than a race.
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

// Edges materialises the arcs for persistence. It allocates, and runs once per
// video at enqueue time — never on the dispatch path.
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

// NodeCountFor returns the number of tasks a spec produces: seven singletons,
// four per chapter, one per slide and one per thumbnail cell. Exported so
// callers can preallocate too.
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

// addNode appends one task and returns its index. Both builders go through it,
// so a head graph's blueprint node is identical to the full builder's.
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

// normaliseAttempts floors the budget at one, so a zero-valued spec still runs
// each task once rather than never.
func normaliseAttempts(maxAttempts int) int {
	if maxAttempts < 1 {
		return 1
	}
	return maxAttempts
}

// BuildHeadGraph constructs the graph a video is enqueued with: a single
// blueprint node. There is no honest chapter count until that blueprint has
// been written and accepted.
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

// Tail is a video's per-chapter body: every node except the blueprint, plus
// every arc. A plain value rather than a Graph because it is assembled off the
// dispatch goroutine and handed in to be spliced.
type Tail struct {
	Tasks []entity.Task
	Edges []repository.TaskEdge
}

// BuildTail constructs the per-chapter body once the chapter count is known.
// It derives from the full graph rather than restating the topology, so a head
// plus its tail is BuildGraph's output node for node.
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

// Expand splices the per-chapter body onto a head graph. Called by the dispatch
// loop and only by it: appending is safe because an unexpanded graph has no
// ready-set entry to invalidate, and no spliced node is runnable on arrival.
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
		// Nothing would ever wake a tail spliced on after the blueprint released.
		return fmt.Errorf("%w: blueprint of %s has already released its dependents",
			ErrInvalidGraph, g.VideoID)
	}

	for _, t := range tail.Tasks {
		if _, dup := g.byID[t.ID]; dup {
			return fmt.Errorf("%w: tail repeats task %s", ErrInvalidGraph, t.ID)
		}
		// Recomputed from the arcs below, so a tail cannot import a count
		// belonging to another topology.
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

// BuildGraph constructs a video's DAG. Slide prompts depend on the blueprint
// alone, not on the chapter script: two independent branches are what let the
// slide pipeline — the longest pole — start early.
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

	// Canonical node order, deterministic so a dispatch sequence is reproducible.
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
	// One icon per cell. The width is fixed here and cannot grow later, which is
	// why the plan's cell count is a contract rather than a target.
	icons := make([]int32, cells)
	for i := range cells {
		icons[i] = add(entity.TaskKindThumbnailIcon, -1, i, entity.GateNone)
	}
	// The upload gate rides on the last node before the upload rather than on the
	// metadata, so what the operator approves is the whole listing.
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
	// The grid is planned from the listing metadata just wrote.
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

// assertAcyclic runs Kahn's algorithm over a copy of the in-degrees; cheap next
// to graph construction.
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

// Downstream returns i plus every node reachable from it, ascending. It is
// what a chapter retry resets.
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
