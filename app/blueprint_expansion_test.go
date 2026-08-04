package app_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
	"github.com/tbui/yt-studio/domain/scheduler"
)

// These tests are about one question: what happens when the model comes back
// with a different number of chapters than the video was briefed with? That is
// the ordinary case, not the exceptional one — a 50-chapter brief that finds 45
// natural breaks is a good blueprint — so the video ends up with 45 chapters
// and a DAG built for 45.

// shortLLM is a blueprint backend that returns a fixed number of chapters,
// whatever it was asked for.
type shortLLM struct {
	store    provider.AssetStore
	chapters int
	// ordinals, when set, replaces the 1..N ordinals the outline would otherwise
	// carry, so the renumbering rule can be tested against a model that returned
	// gaps or repeats.
	ordinals []int
	asked    int
}

var _ provider.LLMProvider = (*shortLLM)(nil)

func (l *shortLLM) Blueprint(ctx context.Context, req provider.BlueprintRequest) (provider.Blueprint, error) {
	l.asked = req.ChapterCount
	bp := provider.Blueprint{
		BlueprintOutline: provider.BlueprintOutline{
			Title:    req.Title,
			Summary:  "an outline",
			Chapters: make([]provider.BlueprintChapter, 0, l.chapters),
		},
	}
	for i := range l.chapters {
		ordinal := i + 1
		if i < len(l.ordinals) {
			ordinal = l.ordinals[i]
		}
		bp.Chapters = append(bp.Chapters, provider.BlueprintChapter{
			Ordinal: ordinal,
			Title:   fmt.Sprintf("Chapter %d", i+1),
			Summary: "a summary",
		})
	}
	stored, err := l.store.Put(ctx, entity.AssetKindBlueprint, bytes.NewReader([]byte(req.Title)))
	if err != nil {
		return provider.Blueprint{}, err
	}
	bp.AssetID = stored.ID
	return bp, nil
}

func (l *shortLLM) Script(context.Context, provider.ScriptRequest) (provider.Script, error) {
	return provider.Script{}, errors.New("not used")
}

func (l *shortLLM) ImagePrompts(context.Context, entity.VideoID) ([]provider.ImagePrompt, error) {
	return nil, errors.New("not used")
}

func (l *shortLLM) Metadata(context.Context, provider.MetadataRequest) (provider.Metadata, error) {
	return provider.Metadata{}, errors.New("not used")
}

func (l *shortLLM) ThumbnailPlan(context.Context, provider.ThumbnailPlanRequest) (provider.ThumbnailPlan, error) {
	return provider.ThumbnailPlan{}, errors.New("not used")
}

// recordingExpander captures the tail an approval would splice on.
type recordingExpander struct {
	videoID entity.VideoID
	tail    scheduler.Tail
	calls   int
	err     error
}

func (e *recordingExpander) Expand(_ context.Context, videoID entity.VideoID, tail scheduler.Tail) error {
	e.calls++
	e.videoID, e.tail = videoID, tail
	return e.err
}

// draft creates a video briefed for chapterCount chapters, with no chapters and
// no tasks: exactly what StartVideo enqueues a blueprint over.
func (f *fixture) draft(ref string, chapterCount, images int) entity.Video {
	f.t.Helper()
	v, err := entity.NewVideo(entity.VideoID(ref), f.channel.ID, entity.Ref(ref),
		"The Long Winter", "a northern port town", chapterCount, images, testThumbnailCells, 0, testTime)
	if err != nil {
		f.t.Fatalf("NewVideo: %v", err)
	}
	if err := f.store.CreateVideo(context.Background(), v); err != nil {
		f.t.Fatalf("CreateVideo: %v", err)
	}
	return v
}

func (f *fixture) blueprintTask(v entity.Video, gate entity.GateKind) entity.Task {
	f.t.Helper()
	return entity.Task{
		ID:          entity.NewTaskID(v.ID, entity.TaskKindBlueprint, -1, -1),
		VideoID:     v.ID,
		Kind:        entity.TaskKindBlueprint,
		Ordinal:     -1,
		Index:       -1,
		Gate:        gate,
		MaxAttempts: 3,
	}
}

// The headline case: 50 asked for, 45 returned, 45 accepted, and the DAG built
// for 45.
func TestBlueprintShortfallIsAcceptedAndShapesTheGraph(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	const briefed, returned, images = 50, 45, 2

	v := f.draft("DSS-1", briefed, images)
	llm := &shortLLM{store: f.assets, chapters: returned}

	outcome := app.GenerateBlueprint(ctx, f.blueprintTask(v, entity.GateBlueprint),
		f.store, f.store, llm, f.store, f.store, f.store, f.assets, nil, 20, testTime)
	if _, ok := outcome.(entity.Success); !ok {
		t.Fatalf("outcome = %#v, want Success", outcome)
	}
	if llm.asked != briefed {
		t.Fatalf("the model was briefed with %d chapters, want %d", llm.asked, briefed)
	}

	chapters, err := f.store.ListChaptersByVideo(ctx, v.ID)
	if err != nil {
		t.Fatalf("ListChaptersByVideo: %v", err)
	}
	if len(chapters) != returned {
		t.Fatalf("chapters = %d, want %d", len(chapters), returned)
	}

	// The video row still records what was asked for. It is the brief, not a
	// contradiction of the outline.
	stored, err := f.store.VideoByID(ctx, v.ID)
	if err != nil {
		t.Fatalf("VideoByID: %v", err)
	}
	if stored.ChapterCount != briefed {
		t.Fatalf("video chapter count = %d, want the target %d", stored.ChapterCount, briefed)
	}

	expander := &recordingExpander{}
	if err := app.ExpandVideoGraph(ctx, f.store, f.store, expander, testTime,
		app.ExpandOptions{MaxAttempts: 3, UploadGate: true}, v.ID); err != nil {
		t.Fatalf("ExpandVideoGraph: %v", err)
	}
	// The tail is every node but the blueprint, built for what came back.
	if got, want := len(expander.tail.Tasks), scheduler.NodeCountFor(returned, images, testThumbnailCells)-1; got != want {
		t.Fatalf("tail tasks = %d, want %d (a %d-chapter DAG minus the blueprint)", got, want, returned)
	}
	if expander.videoID != v.ID {
		t.Fatalf("expanded %q, want %q", expander.videoID, v.ID)
	}
}

// Ordinals are renumbered 1..N in the order the model returned them. They are
// the chapter's natural key and the index every task id is derived from, so a
// gap would put the DAG and the chapter table out of correspondence.
func TestBlueprintOrdinalsAreRenumbered(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	v := f.draft("DSS-1", 4, 1)
	llm := &shortLLM{store: f.assets, chapters: 4, ordinals: []int{3, 9, 9, 1}}

	outcome := app.GenerateBlueprint(ctx, f.blueprintTask(v, entity.GateNone),
		f.store, f.store, llm, f.store, f.store, f.store, f.assets, nil, 20, testTime)
	if _, ok := outcome.(entity.Success); !ok {
		t.Fatalf("outcome = %#v, want Success", outcome)
	}

	chapters, err := f.store.ListChaptersByVideo(ctx, v.ID)
	if err != nil {
		t.Fatalf("ListChaptersByVideo: %v", err)
	}
	if len(chapters) != 4 {
		t.Fatalf("chapters = %d, want 4", len(chapters))
	}
	for i, c := range chapters {
		if c.Ordinal != i+1 {
			t.Fatalf("chapter %d has ordinal %d, want %d", i, c.Ordinal, i+1)
		}
		// And the id the DAG will address it by matches.
		if c.ID != entity.NewChapterID(v.ID, i+1) {
			t.Fatalf("chapter %d id = %q, want %q", i, c.ID, entity.NewChapterID(v.ID, i+1))
		}
	}
}

// The band separates a blueprint that missed by five from one that came back
// with three. The second is a model failure and stops the line.
func TestBlueprintOutsideToleranceIsRejected(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		returned int
	}{
		{"far too few", 3},
		{"far too many", 90},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			ctx := context.Background()

			v := f.draft("DSS-1", 50, 2)
			llm := &shortLLM{store: f.assets, chapters: tc.returned}

			outcome := app.GenerateBlueprint(ctx, f.blueprintTask(v, entity.GateNone),
				f.store, f.store, llm, f.store, f.store, f.store, f.assets, nil, 20, testTime)
			failed, ok := outcome.(entity.Failed)
			if !ok {
				t.Fatalf("outcome = %#v, want Failed", outcome)
			}
			if !errors.Is(failed.Err, app.ErrBlueprintOffTarget) {
				t.Fatalf("error = %v, want ErrBlueprintOffTarget", failed.Err)
			}
			// How far a roll lands from the brief is a property of the roll, so
			// another attempt is worth having.
			if !failed.Retryable {
				t.Fatal("an off-target outline was failed permanently")
			}
			// Nothing was written: a rejected outline leaves no chapters behind for
			// an expansion to be derived from.
			chapters, err := f.store.ListChaptersByVideo(ctx, v.ID)
			if err != nil {
				t.Fatalf("ListChaptersByVideo: %v", err)
			}
			if len(chapters) != 0 {
				t.Fatalf("chapters = %d, want none", len(chapters))
			}
		})
	}
}

// A zero tolerance is the old behaviour, kept reachable for an operator who
// wants the count they asked for or nothing.
func TestZeroToleranceDemandsAnExactCount(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	v := f.draft("DSS-1", 10, 1)
	llm := &shortLLM{store: f.assets, chapters: 9}

	outcome := app.GenerateBlueprint(ctx, f.blueprintTask(v, entity.GateNone),
		f.store, f.store, llm, f.store, f.store, f.store, f.assets, nil, 0, testTime)
	if _, ok := outcome.(entity.Failed); !ok {
		t.Fatalf("outcome = %#v, want Failed", outcome)
	}
}

// Expanding a video whose blueprint has not produced chapters is a conflict,
// not an empty tail: there is nothing to build a DAG from.
func TestExpandWithoutChaptersIsRejected(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	v := f.draft("DSS-1", 5, 2)

	expander := &recordingExpander{}
	err := app.ExpandVideoGraph(context.Background(), f.store, f.store, expander, testTime,
		app.ExpandOptions{MaxAttempts: 3}, v.ID)
	if !errors.Is(err, app.ErrConflict) {
		t.Fatalf("ExpandVideoGraph = %v, want ErrConflict", err)
	}
	if expander.calls != 0 {
		t.Fatal("an empty video was handed to the scheduler")
	}
}

// The upload gate travels on the thumbnail node, which is part of the tail, so
// it is read when the graph expands rather than when the video was enqueued.
func TestExpandCarriesTheUploadGate(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	v := f.draft("DSS-1", 3, 1)
	llm := &shortLLM{store: f.assets, chapters: 3}
	outcome := app.GenerateBlueprint(ctx, f.blueprintTask(v, entity.GateNone),
		f.store, f.store, llm, f.store, f.store, f.store, f.assets, nil, 20, testTime)
	if _, ok := outcome.(entity.Success); !ok {
		t.Fatalf("outcome = %#v, want Success", outcome)
	}

	for _, gateOn := range []bool{false, true} {
		expander := &recordingExpander{}
		if err := app.ExpandVideoGraph(ctx, f.store, f.store, expander, testTime,
			app.ExpandOptions{MaxAttempts: 3, UploadGate: gateOn}, v.ID); err != nil {
			t.Fatalf("ExpandVideoGraph: %v", err)
		}
		want := entity.GateNone
		if gateOn {
			want = entity.GateUpload
		}
		var found bool
		for _, task := range expander.tail.Tasks {
			if task.Kind != entity.TaskKindThumbnail {
				continue
			}
			found = true
			if task.Gate != want {
				t.Fatalf("uploadGate=%v: thumbnail gate = %q, want %q", gateOn, task.Gate, want)
			}
		}
		if !found {
			t.Fatal("the tail has no thumbnail node to carry the gate")
		}
	}
}

// approver stands in for the scheduler's gate release.
type approver struct {
	approved []entity.TaskID
	err      error
}

func (a *approver) Approve(_ context.Context, id entity.TaskID) error {
	a.approved = append(a.approved, id)
	return a.err
}

// Approving a blueprint is what fixes a video's shape, so ApproveGate expands
// before it releases. The reverse order would let a video whose whole DAG is
// one succeeded blueprint report itself completed.
func TestApproveBlueprintGateExpandsBeforeReleasing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	const briefed, returned, images = 20, 17, 2

	v := f.draft("DSS-1", briefed, images)
	llm := &shortLLM{store: f.assets, chapters: returned}
	outcome := app.GenerateBlueprint(ctx, f.blueprintTask(v, entity.GateBlueprint),
		f.store, f.store, llm, f.store, f.store, f.store, f.assets, nil, 20, testTime)
	if _, ok := outcome.(entity.Success); !ok {
		t.Fatalf("outcome = %#v, want Success", outcome)
	}

	// The durable head graph, parked on its gate, exactly as the server leaves it.
	head, err := scheduler.BuildHeadGraph(scheduler.HeadSpec{
		VideoID: v.ID, MaxAttempts: 3, BlueprintGate: true, Now: testTime,
	})
	if err != nil {
		t.Fatalf("BuildHeadGraph: %v", err)
	}
	if err := f.store.InsertGraph(ctx, v.ID, head.Tasks(), head.Edges()); err != nil {
		t.Fatalf("InsertGraph: %v", err)
	}
	blueprintID := entity.NewTaskID(v.ID, entity.TaskKindBlueprint, -1, -1)
	if err := f.store.ApplyTransitions(ctx, []repository.TaskTransition{{
		ID: blueprintID, State: entity.TaskStateAwaitingApproval, UpdatedAt: testTime,
	}}); err != nil {
		t.Fatalf("ApplyTransitions: %v", err)
	}

	expander := &recordingExpander{}
	gate := &approver{}
	task, err := app.ApproveGate(ctx, f.store, f.store, f.store, expander, gate, testTime,
		app.ExpandOptions{MaxAttempts: 3, UploadGate: true}, v.ID, entity.GateBlueprint)
	if err != nil {
		t.Fatalf("ApproveGate: %v", err)
	}
	if task.ID != blueprintID {
		t.Fatalf("approved %q, want %q", task.ID, blueprintID)
	}
	if expander.calls != 1 {
		t.Fatalf("expansions = %d, want exactly 1", expander.calls)
	}
	if got, want := len(expander.tail.Tasks), scheduler.NodeCountFor(returned, images, testThumbnailCells)-1; got != want {
		t.Fatalf("tail tasks = %d, want %d", got, want)
	}
	if len(gate.approved) != 1 {
		t.Fatalf("approvals = %d, want 1", len(gate.approved))
	}
}

// A failed expansion must not release the gate: the operator can approve again
// once whatever broke is fixed, and the video is not left half-started.
func TestApproveBlueprintGateHoldsWhenExpansionFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	v := f.draft("DSS-1", 6, 1)
	llm := &shortLLM{store: f.assets, chapters: 6}
	if _, ok := app.GenerateBlueprint(ctx, f.blueprintTask(v, entity.GateBlueprint),
		f.store, f.store, llm, f.store, f.store, f.store, f.assets, nil, 20, testTime).(entity.Success); !ok {
		t.Fatal("the blueprint did not succeed")
	}
	head, err := scheduler.BuildHeadGraph(scheduler.HeadSpec{
		VideoID: v.ID, MaxAttempts: 3, BlueprintGate: true, Now: testTime,
	})
	if err != nil {
		t.Fatalf("BuildHeadGraph: %v", err)
	}
	if err := f.store.InsertGraph(ctx, v.ID, head.Tasks(), head.Edges()); err != nil {
		t.Fatalf("InsertGraph: %v", err)
	}
	if err := f.store.ApplyTransitions(ctx, []repository.TaskTransition{{
		ID:    entity.NewTaskID(v.ID, entity.TaskKindBlueprint, -1, -1),
		State: entity.TaskStateAwaitingApproval, UpdatedAt: testTime,
	}}); err != nil {
		t.Fatalf("ApplyTransitions: %v", err)
	}

	expander := &recordingExpander{err: errors.New("the loop is closed")}
	gate := &approver{}
	if _, err := app.ApproveGate(ctx, f.store, f.store, f.store, expander, gate, testTime,
		app.ExpandOptions{MaxAttempts: 3}, v.ID, entity.GateBlueprint); err == nil {
		t.Fatal("ApproveGate succeeded despite the expansion failing")
	}
	if len(gate.approved) != 0 {
		t.Fatal("the gate was released over a video with no DAG below it")
	}
}

// recordingSubmitter captures whatever StartVideo hands to the scheduler.
type recordingSubmitter struct{ graphs []*scheduler.Graph }

func (s *recordingSubmitter) Submit(_ context.Context, g *scheduler.Graph) error {
	s.graphs = append(s.graphs, g)
	return nil
}

// recordingResumer and recordingRequeuer stand in for the loop on the resume
// path StartVideo takes over a video that already has tasks.
type recordingResumer struct{ graphs []*scheduler.Graph }

func (r *recordingResumer) Resume(_ context.Context, graphs []*scheduler.Graph) error {
	r.graphs = append(r.graphs, graphs...)
	return nil
}

type recordingRequeuer struct{ videos []entity.VideoID }

func (r *recordingRequeuer) Requeue(_ context.Context, videoID entity.VideoID) (int, error) {
	r.videos = append(r.videos, videoID)
	return 0, nil
}

// A fresh video is enqueued as a lone blueprint.
func TestStartVideoEnqueuesOnlyTheBlueprint(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	v := f.draft("DSS-1", 50, 2)

	submitter := &recordingSubmitter{}
	if _, err := app.StartVideo(context.Background(), f.store, f.store, submitter,
		&recordingResumer{}, &recordingRequeuer{}, testTime,
		app.StartVideoOptions{MaxAttempts: 3, BlueprintGate: true}, string(v.ID)); err != nil {
		t.Fatalf("StartVideo: %v", err)
	}
	if len(submitter.graphs) != 1 {
		t.Fatalf("submitted %d graphs, want 1", len(submitter.graphs))
	}
	g := submitter.graphs[0]
	if g.NodeCount() != 1 {
		t.Fatalf("enqueued %d nodes, want 1: the chapter count is not known yet", g.NodeCount())
	}
	if g.Task(0).Kind != entity.TaskKindBlueprint {
		t.Fatalf("enqueued a %s, want a blueprint", g.Task(0).Kind)
	}
}

// Starting a video that already has tasks never submits a second head graph:
// that would leave the loop believing the video is one node while the database
// holds hundreds. It resumes instead, handing the loop the DAG as persisted —
// which is the only way back to a cancelled video, since one whose tasks are
// all terminal is not reloaded at startup.
func TestStartVideoResumesAnExpandedGraphInsteadOfResubmitting(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	v := f.draft("DSS-1", 6, 2)

	full, err := scheduler.BuildGraph(scheduler.BuildSpec{
		VideoID: v.ID, ChapterCount: 6, ImagesPerChapter: 2,
		ThumbnailCells: testThumbnailCells, MaxAttempts: 3, Now: testTime,
	})
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	if err := f.store.InsertGraph(ctx, v.ID, full.Tasks(), full.Edges()); err != nil {
		t.Fatalf("InsertGraph: %v", err)
	}

	submitter := &recordingSubmitter{}
	resumer := &recordingResumer{}
	requeuer := &recordingRequeuer{}
	if _, err := app.StartVideo(ctx, f.store, f.store, submitter, resumer, requeuer, testTime,
		app.StartVideoOptions{MaxAttempts: 3}, string(v.ID)); err != nil {
		t.Fatalf("StartVideo: %v", err)
	}
	if len(submitter.graphs) != 0 {
		t.Fatalf("submitted %d graphs over an already-enqueued video, want none", len(submitter.graphs))
	}
	if len(resumer.graphs) != 1 {
		t.Fatalf("resumed %d graphs, want 1", len(resumer.graphs))
	}
	if got, want := resumer.graphs[0].NodeCount(), full.NodeCount(); got != want {
		t.Fatalf("resumed a %d-node graph, want the persisted %d", got, want)
	}
	if len(requeuer.videos) != 1 || requeuer.videos[0] != v.ID {
		t.Fatalf("requeued %v, want [%s]", requeuer.videos, v.ID)
	}
}
