package scheduler

import (
	"errors"
	"testing"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
)

func headAndTail(t *testing.T, videoID entity.VideoID, chapters, images int, gates bool) (*Graph, Tail) {
	t.Helper()
	head, err := BuildHeadGraph(HeadSpec{
		VideoID:       videoID,
		MaxAttempts:   3,
		BlueprintGate: gates,
		Now:           time.Unix(0, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("BuildHeadGraph: %v", err)
	}
	tail, err := BuildTail(BuildSpec{
		VideoID:          videoID,
		ChapterCount:     chapters,
		ImagesPerChapter: images,
		MaxAttempts:      3,
		BlueprintGate:    gates,
		UploadGate:       gates,
		Now:              time.Unix(0, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("BuildTail: %v", err)
	}
	return head, tail
}

// A video is enqueued as a lone blueprint. Everything below it is one branch
// per chapter, and there is no honest chapter count until the blueprint has
// been written.
func TestHeadGraphIsJustTheBlueprint(t *testing.T) {
	t.Parallel()
	g, err := BuildHeadGraph(HeadSpec{VideoID: "v1", MaxAttempts: 3, BlueprintGate: true, Now: time.Unix(0, 0)})
	if err != nil {
		t.Fatalf("BuildHeadGraph: %v", err)
	}
	if g.NodeCount() != 1 {
		t.Fatalf("node count = %d, want 1", g.NodeCount())
	}
	if g.Expanded() {
		t.Fatal("a head graph reports itself expanded")
	}
	head := g.Task(0)
	if head.Kind != entity.TaskKindBlueprint {
		t.Fatalf("head node is %s, want blueprint", head.Kind)
	}
	if head.Gate != entity.GateBlueprint {
		t.Fatalf("head gate = %q, want blueprint", head.Gate)
	}
	if head.DepsRemaining != 0 || len(g.Roots()) != 1 {
		t.Fatal("the blueprint is not runnable as a root")
	}
}

// The load-bearing invariant of the two-phase build: enqueueing a blueprint and
// splicing on the tail its outline called for produces exactly the graph the
// single-shot builder would have produced for that chapter count.
//
// Everything else about expansion — deterministic ids, stable dispatch order,
// the golden-file fixtures — rests on this being true rather than nearly true.
func TestHeadPlusTailEqualsBuildGraph(t *testing.T) {
	t.Parallel()
	for _, chapters := range []int{1, 3, 45} {
		for _, gates := range []bool{false, true} {
			head, tail := headAndTail(t, "v1", chapters, 2, gates)
			if err := head.Expand(tail); err != nil {
				t.Fatalf("chapters=%d gates=%v: Expand: %v", chapters, gates, err)
			}
			want := testGraph(t, "v1", chapters, 2, gates)

			if head.NodeCount() != want.NodeCount() {
				t.Fatalf("chapters=%d: node count = %d, want %d", chapters, head.NodeCount(), want.NodeCount())
			}
			for i := range want.NodeCount() {
				got, expected := head.Task(i), want.Task(i)
				if got.ID != expected.ID {
					t.Fatalf("chapters=%d node %d: id %q, want %q", chapters, i, got.ID, expected.ID)
				}
				if got.Kind != expected.Kind || got.Ordinal != expected.Ordinal || got.Index != expected.Index {
					t.Fatalf("chapters=%d node %d: %+v, want %+v", chapters, i, got, expected)
				}
				if got.Gate != expected.Gate {
					t.Fatalf("chapters=%d node %d (%s): gate %q, want %q", chapters, i, got.Kind, got.Gate, expected.Gate)
				}
				if got.DepsRemaining != expected.DepsRemaining {
					t.Fatalf("chapters=%d node %d (%s): deps remaining %d, want %d",
						chapters, i, got.Kind, got.DepsRemaining, expected.DepsRemaining)
				}
				if got.Pool != expected.Pool || got.MaxAttempts != expected.MaxAttempts {
					t.Fatalf("chapters=%d node %d: pool/attempts %v/%d, want %v/%d",
						chapters, i, got.Pool, got.MaxAttempts, expected.Pool, expected.MaxAttempts)
				}
				if len(head.dependents[i]) != len(want.dependents[i]) ||
					len(head.dependencies[i]) != len(want.dependencies[i]) {
					t.Fatalf("chapters=%d node %d (%s): arcs %d/%d, want %d/%d", chapters, i, got.Kind,
						len(head.dependents[i]), len(head.dependencies[i]),
						len(want.dependents[i]), len(want.dependencies[i]))
				}
			}
			if len(head.Edges()) != len(want.Edges()) {
				t.Fatalf("chapters=%d: edges = %d, want %d", chapters, len(head.Edges()), len(want.Edges()))
			}
		}
	}
}

// Nothing spliced on is runnable on arrival: every tail node depends on the
// blueprint transitively. That is what lets Expand stay a pure graph operation
// and never touch the ready set.
func TestExpandAdmitsNothing(t *testing.T) {
	t.Parallel()
	head, tail := headAndTail(t, "v1", 6, 2, false)
	if err := head.Expand(tail); err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if roots := head.Roots(); len(roots) != 1 || head.Task(int(roots[0])).Kind != entity.TaskKindBlueprint {
		t.Fatalf("roots = %v, want the blueprint alone", roots)
	}
	for i := 1; i < head.NodeCount(); i++ {
		if head.Task(i).DepsRemaining == 0 {
			t.Fatalf("node %d (%s) is runnable straight after expansion", i, head.Task(i).Kind)
		}
	}
}

// A tail spliced on after the blueprint has released would never be woken:
// releaseDependents has already run and will not run again.
func TestExpandRejectsReleasedBlueprint(t *testing.T) {
	t.Parallel()
	head, tail := headAndTail(t, "v1", 4, 2, false)
	head.Task(0).State = entity.TaskStateSucceeded

	if err := head.Expand(tail); !errors.Is(err, ErrInvalidGraph) {
		t.Fatalf("Expand = %v, want ErrInvalidGraph", err)
	}
}

func TestExpandRejectsSecondSplice(t *testing.T) {
	t.Parallel()
	head, tail := headAndTail(t, "v1", 4, 2, false)
	if err := head.Expand(tail); err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if !head.Expanded() {
		t.Fatal("an expanded graph does not report itself expanded")
	}
	if err := head.Expand(tail); !errors.Is(err, ErrInvalidGraph) {
		t.Fatalf("second Expand = %v, want ErrInvalidGraph", err)
	}
}

// An expanded graph survives a restart as one graph, not as a head and a tail.
func TestExpandedGraphRoundTripsThroughPersistence(t *testing.T) {
	t.Parallel()
	head, tail := headAndTail(t, "v1", 7, 3, true)
	if err := head.Expand(tail); err != nil {
		t.Fatalf("Expand: %v", err)
	}

	restored, err := GraphFromPersisted(persistedFrom(head))
	if err != nil {
		t.Fatalf("GraphFromPersisted: %v", err)
	}
	if restored.NodeCount() != head.NodeCount() {
		t.Fatalf("node count = %d, want %d", restored.NodeCount(), head.NodeCount())
	}
	if !restored.Expanded() {
		t.Fatal("a restored expanded graph reports itself unexpanded")
	}
	for i := range head.NodeCount() {
		if restored.Task(i).ID != head.Task(i).ID {
			t.Fatalf("node %d: %q != %q", i, restored.Task(i).ID, head.Task(i).ID)
		}
		if len(restored.dependencies[i]) != len(head.dependencies[i]) {
			t.Fatalf("node %d: dependencies %d, want %d", i, len(restored.dependencies[i]), len(head.dependencies[i]))
		}
	}
}

// A head graph is a legitimate thing to resume: a video can sit on its
// blueprint gate for days, and the daemon may restart underneath it.
func TestHeadGraphRoundTripsThroughPersistence(t *testing.T) {
	t.Parallel()
	head, tail := headAndTail(t, "v1", 5, 2, true)
	head.Task(0).State = entity.TaskStateAwaitingApproval

	restored, err := GraphFromPersisted(persistedFrom(head))
	if err != nil {
		t.Fatalf("GraphFromPersisted: %v", err)
	}
	if restored.Expanded() {
		t.Fatal("a restored head graph reports itself expanded")
	}
	// And it can still be expanded once the operator gets back to it.
	if err := restored.Expand(tail); err != nil {
		t.Fatalf("Expand after restore: %v", err)
	}
	if restored.NodeCount() != NodeCountFor(5, 2) {
		t.Fatalf("node count = %d, want %d", restored.NodeCount(), NodeCountFor(5, 2))
	}
}

func TestBuildTailRejectsBadSpecs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		spec BuildSpec
	}{
		{"no video", BuildSpec{ChapterCount: 1, ImagesPerChapter: 1}},
		{"zero chapters", BuildSpec{VideoID: "v", ChapterCount: 0, ImagesPerChapter: 1}},
		{"too many chapters", BuildSpec{VideoID: "v", ChapterCount: entity.MaxChapterCount + 1, ImagesPerChapter: 1}},
		{"zero images", BuildSpec{VideoID: "v", ChapterCount: 1, ImagesPerChapter: 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := BuildTail(tc.spec); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestBuildHeadGraphRejectsEmptyVideoID(t *testing.T) {
	t.Parallel()
	if _, err := BuildHeadGraph(HeadSpec{}); !errors.Is(err, ErrInvalidGraph) {
		t.Fatalf("BuildHeadGraph = %v, want ErrInvalidGraph", err)
	}
}
