package scheduler

import (
	"testing"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
)

func TestBuildGraphShape(t *testing.T) {
	t.Parallel()
	const chapters, images = 50, 2

	g := testGraph(t, "v1", chapters, images, true)

	if got, want := g.NodeCount(), NodeCountFor(chapters, images); got != want {
		t.Fatalf("node count = %d, want %d", got, want)
	}

	byKind := map[entity.TaskKind]int{}
	for i := range g.NodeCount() {
		byKind[g.Task(i).Kind]++
	}
	want := map[entity.TaskKind]int{
		entity.TaskKindBlueprint:         1,
		entity.TaskKindPrimeImagePrompts: 1,
		entity.TaskKindImagePrompts:      chapters,
		entity.TaskKindScript:            chapters,
		entity.TaskKindTTS:               chapters,
		entity.TaskKindImage:             chapters * images,
		entity.TaskKindClip:              chapters,
		entity.TaskKindConcat:            1,
		entity.TaskKindMetadata:          1,
		entity.TaskKindUpload:            1,
	}
	for kind, n := range want {
		if byKind[kind] != n {
			t.Errorf("%s count = %d, want %d", kind, byKind[kind], n)
		}
	}
}

// Image prompts depend on the blueprint alone, not on the chapter script. That
// is what gives the graph two independent branches and starts the longest pole
// early.
func TestImagePromptsDoNotDependOnScripts(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 8, 2, false)

	for i := range g.NodeCount() {
		task := g.Task(i)
		if task.Kind != entity.TaskKindImagePrompts && task.Kind != entity.TaskKindImage {
			continue
		}
		for _, dep := range g.dependencies[i] {
			if g.Task(int(dep)).Kind == entity.TaskKindScript || g.Task(int(dep)).Kind == entity.TaskKindTTS {
				t.Fatalf("%s depends on %s, which serialises the image branch behind narration",
					task.Kind, g.Task(int(dep)).Kind)
			}
		}
	}
}

func TestGraphRootIsBlueprintOnly(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 12, 2, false)

	roots := g.Roots()
	if len(roots) != 1 {
		t.Fatalf("roots = %d, want exactly 1", len(roots))
	}
	if kind := g.Task(int(roots[0])).Kind; kind != entity.TaskKindBlueprint {
		t.Fatalf("root is %s, want blueprint", kind)
	}
}

func TestGatesAreAttachedToTheRightTasks(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 3, 2, true)

	gates := map[entity.TaskKind]entity.GateKind{}
	for i := range g.NodeCount() {
		if gate := g.Task(i).Gate; gate != entity.GateNone {
			gates[g.Task(i).Kind] = gate
		}
	}
	if gates[entity.TaskKindBlueprint] != entity.GateBlueprint {
		t.Errorf("blueprint gate = %q, want blueprint", gates[entity.TaskKindBlueprint])
	}
	// The upload gate hangs off metadata: approving it is what releases upload.
	if gates[entity.TaskKindMetadata] != entity.GateUpload {
		t.Errorf("metadata gate = %q, want upload", gates[entity.TaskKindMetadata])
	}
	if len(gates) != 2 {
		t.Errorf("gates = %v, want exactly two", gates)
	}
}

func TestGatesCanBeDisabled(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 3, 2, false)
	for i := range g.NodeCount() {
		if gate := g.Task(i).Gate; gate != entity.GateNone {
			t.Fatalf("%s carries gate %q with gates disabled", g.Task(i).Kind, gate)
		}
	}
}

func TestEveryTaskAcquiresExactlyOnePool(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 5, 3, false)
	for i := range g.NodeCount() {
		task := g.Task(i)
		if !task.Pool.Valid() {
			t.Fatalf("%s has invalid pool %q", task.Kind, task.Pool)
		}
		if task.Pool != task.Kind.Pool() {
			t.Fatalf("%s pool = %q, want %q", task.Kind, task.Pool, task.Kind.Pool())
		}
	}
}

// Per-chapter prompt tasks must not sit in the LLM pool, or there would be no
// point in the priming task.
func TestPromptFanOutUsesTheCachePool(t *testing.T) {
	t.Parallel()
	if got := entity.TaskKindImagePrompts.Pool(); got != entity.PoolCache {
		t.Fatalf("image_prompts pool = %q, want cache", got)
	}
	if got := entity.TaskKindPrimeImagePrompts.Pool(); got != entity.PoolLLM {
		t.Fatalf("prime_image_prompts pool = %q, want llm", got)
	}
}

func TestDepsRemainingMatchesInDegree(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 7, 2, true)
	for i := range g.NodeCount() {
		if got, want := g.Task(i).DepsRemaining, len(g.dependencies[i]); got != want {
			t.Fatalf("%s deps remaining = %d, want %d", g.Task(i).ID, got, want)
		}
	}
}

func TestDownstreamCoversTheWholeTail(t *testing.T) {
	t.Parallel()
	g := testGraph(t, "v1", 4, 2, false)

	// Everything is downstream of the blueprint.
	roots := g.Roots()
	if got := len(g.Downstream(roots[0])); got != g.NodeCount() {
		t.Fatalf("downstream of blueprint = %d nodes, want all %d", got, g.NodeCount())
	}

	// A single chapter's script reaches its tts, clip, then concat, metadata and
	// upload — but not another chapter's script.
	var scriptIdx int32 = -1
	for i := range g.NodeCount() {
		if g.Task(i).Kind == entity.TaskKindScript && g.Task(i).Ordinal == 2 {
			scriptIdx = int32(i)
			break
		}
	}
	if scriptIdx < 0 {
		t.Fatal("no script task for chapter 2")
	}
	reached := map[entity.TaskKind]int{}
	for _, idx := range g.Downstream(scriptIdx) {
		reached[g.Task(int(idx)).Kind]++
	}
	if reached[entity.TaskKindScript] != 1 {
		t.Errorf("reached %d script tasks, want only its own", reached[entity.TaskKindScript])
	}
	for _, kind := range []entity.TaskKind{entity.TaskKindTTS, entity.TaskKindClip, entity.TaskKindConcat, entity.TaskKindMetadata, entity.TaskKindUpload} {
		if reached[kind] == 0 {
			t.Errorf("%s is not downstream of a chapter script", kind)
		}
	}
	if reached[entity.TaskKindImage] != 0 {
		t.Errorf("stills are downstream of a script, which would serialise the branches")
	}
}

func TestBuildGraphRejectsBadSpecs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		spec BuildSpec
	}{
		{"no video", BuildSpec{ChapterCount: 1, ImagesPerChapter: 1}},
		{"zero chapters", BuildSpec{VideoID: "v", ChapterCount: 0, ImagesPerChapter: 1}},
		{"too many chapters", BuildSpec{VideoID: "v", ChapterCount: entity.MaxChapterCount + 1, ImagesPerChapter: 1}},
		{"zero images", BuildSpec{VideoID: "v", ChapterCount: 1, ImagesPerChapter: 0}},
		{"too many images", BuildSpec{VideoID: "v", ChapterCount: 1, ImagesPerChapter: entity.MaxImagesPerChapter + 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := BuildGraph(tc.spec); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// Deterministic ids are what make re-enqueueing idempotent and golden-file
// fixtures stable.
func TestGraphIsDeterministic(t *testing.T) {
	t.Parallel()
	spec := BuildSpec{VideoID: "v1", ChapterCount: 6, ImagesPerChapter: 2, MaxAttempts: 3, Now: time.Unix(0, 0)}
	a, err := BuildGraph(spec)
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildGraph(spec)
	if err != nil {
		t.Fatal(err)
	}
	for i := range a.NodeCount() {
		if a.Task(i).ID != b.Task(i).ID {
			t.Fatalf("node %d: %q != %q", i, a.Task(i).ID, b.Task(i).ID)
		}
	}
}

func TestGraphFromPersistedRoundTrips(t *testing.T) {
	t.Parallel()
	original := testGraph(t, "v1", 5, 2, true)

	restored, err := GraphFromPersisted(persistedFrom(original))
	if err != nil {
		t.Fatalf("GraphFromPersisted: %v", err)
	}
	if restored.NodeCount() != original.NodeCount() {
		t.Fatalf("node count = %d, want %d", restored.NodeCount(), original.NodeCount())
	}
	for i := range original.NodeCount() {
		if len(restored.dependents[i]) != len(original.dependents[i]) {
			t.Fatalf("node %d dependents = %d, want %d", i, len(restored.dependents[i]), len(original.dependents[i]))
		}
		if len(restored.dependencies[i]) != len(original.dependencies[i]) {
			t.Fatalf("node %d dependencies = %d, want %d", i, len(restored.dependencies[i]), len(original.dependencies[i]))
		}
	}
}
