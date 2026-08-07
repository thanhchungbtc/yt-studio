package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
)

// cmdKind enumerates the operations the loop accepts. Every public method hands
// one over and waits, so all mutable state stays owned by one goroutine.
type cmdKind uint8

// The complete set of commands.
const (
	cmdSubmit cmdKind = iota
	cmdExpand
	cmdResume
	cmdApprove
	cmdReject
	cmdCancel
	cmdRetryTask
	cmdRetryChapter
	cmdRequeue
	cmdRerun
	cmdMarkStale
	cmdRunStale
	cmdAcceptStale
	cmdForget
	cmdSetPoolLimit
)

type command struct {
	kind    cmdKind
	graph   *Graph
	graphs  []*Graph
	tail    Tail
	taskID  entity.TaskID
	taskIDs []entity.TaskID
	videoID entity.VideoID
	ordinal int
	pool    entity.Pool
	limit   int
	reason  string
	dryRun  bool
	// out and count are filled before the reply; the caller blocks on it, so the
	// write happens-before the read and no lock is needed.
	out   *[]entity.TaskID
	count *int
	reply chan error
}

func (s *Scheduler) send(ctx context.Context, c command) error {
	if !s.running.Load() {
		return ErrSchedulerClosed
	}
	c.reply = make(chan error, 1)
	select {
	case s.cmds <- c:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-c.reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Scheduler) handleCommand(ctx context.Context, c command) {
	var err error
	switch c.kind {
	case cmdSubmit:
		err = s.doSubmit(c.graph)
	case cmdExpand:
		err = s.doExpand(c.videoID, c.tail)
	case cmdResume:
		err = s.doResume(c.graphs)
	case cmdApprove:
		err = s.doApprove(c.taskID)
	case cmdReject:
		err = s.doReject(c.taskID, c.reason)
	case cmdCancel:
		err = s.doCancel(c.videoID)
	case cmdRetryTask:
		err = s.doRetryTask(c.taskID)
	case cmdRetryChapter:
		err = s.doRetryChapter(c.videoID, c.ordinal)
	case cmdRequeue:
		err = s.doRequeue(c)
	case cmdRerun:
		err = s.doRerun(c)
	case cmdMarkStale:
		err = s.doMarkStale(c)
	case cmdRunStale:
		err = s.doRunStale(c)
	case cmdAcceptStale:
		err = s.doAcceptStale(c)
	case cmdForget:
		err = s.doForget(c.videoID)
	case cmdSetPoolLimit:
		err = s.pools.SetLimit(c.pool, c.limit)
		s.dirty = true
	default:
		panic(fmt.Sprintf("unhandled command kind %d", c.kind))
	}
	c.reply <- err
}

// Submit persists a freshly built DAG and admits it to the loop. Task ids are
// deterministic, so submitting the same video twice is idempotent.
func (s *Scheduler) Submit(ctx context.Context, g *Graph) error {
	if g == nil {
		return fmt.Errorf("%w: nil graph", ErrInvalidGraph)
	}
	if g.Installed() {
		// The loop owns these tasks; reading them here to persist again is a race.
		return nil
	}
	if err := s.store.InsertGraph(ctx, g.VideoID, g.Tasks(), g.Edges()); err != nil {
		return fmt.Errorf("persist graph: %w", err)
	}
	return s.send(ctx, command{kind: cmdSubmit, graph: g})
}

// Expand splices a video's per-chapter body onto its head graph. Rows are
// written before the loop is told, and InsertGraph upserts over deterministic
// ids, so a retried approval recomputes the same tail and writes nothing new.
func (s *Scheduler) Expand(ctx context.Context, videoID entity.VideoID, tail Tail) error {
	if len(tail.Tasks) == 0 {
		return fmt.Errorf("%w: empty tail for %s", ErrInvalidGraph, videoID)
	}
	if err := s.store.InsertGraph(ctx, videoID, tail.Tasks, tail.Edges); err != nil {
		return fmt.Errorf("persist tail: %w", err)
	}
	return s.send(ctx, command{kind: cmdExpand, videoID: videoID, tail: tail})
}

// Resume admits graphs rebuilt from the database at startup, so a crash
// mid-run resumes rather than restarts.
func (s *Scheduler) Resume(ctx context.Context, graphs []*Graph) error {
	return s.send(ctx, command{kind: cmdResume, graphs: graphs})
}

// Approve releases a gated task's successors. A gate is a row update.
func (s *Scheduler) Approve(ctx context.Context, taskID entity.TaskID) error {
	return s.send(ctx, command{kind: cmdApprove, taskID: taskID})
}

// Reject fails a gated task with an operator-supplied reason.
func (s *Scheduler) Reject(ctx context.Context, taskID entity.TaskID, reason string) error {
	return s.send(ctx, command{kind: cmdReject, taskID: taskID, reason: reason})
}

// Cancel stops a video. Its in-flight provider calls are cancelled through the
// per-video context, so its slots come back within 100 ms.
func (s *Scheduler) Cancel(ctx context.Context, videoID entity.VideoID) error {
	return s.send(ctx, command{kind: cmdCancel, videoID: videoID})
}

// RetryTask resets one task and everything downstream of it.
func (s *Scheduler) RetryTask(ctx context.Context, taskID entity.TaskID) error {
	return s.send(ctx, command{kind: cmdRetryTask, taskID: taskID})
}

// RetryChapter resets every task of one chapter and everything downstream.
func (s *Scheduler) RetryChapter(ctx context.Context, videoID entity.VideoID, ordinal int) error {
	return s.send(ctx, command{kind: cmdRetryChapter, videoID: videoID, ordinal: ordinal})
}

// Requeue resets every task a video stopped on — cancelled or failed — plus
// everything downstream, and reports the count. It is what resuming a cancelled
// video means; re-submitting the head graph cannot be, since the DAG has
// already expanded. Nothing stopped is a no-op rather than an error.
func (s *Scheduler) Requeue(ctx context.Context, videoID entity.VideoID) (int, error) {
	var n int
	err := s.send(ctx, command{kind: cmdRequeue, videoID: videoID, count: &n})
	return n, err
}

// Rerun re-runs tasks that already succeeded and marks everything downstream
// stale rather than re-running it: those artifacts may have been reviewed, so
// discarding them is the operator's decision. With dryRun nothing changes and
// the returned ids are what would go stale.
func (s *Scheduler) Rerun(
	ctx context.Context,
	videoID entity.VideoID,
	seeds []entity.TaskID,
	dryRun bool,
) ([]entity.TaskID, error) {
	var out []entity.TaskID
	err := s.send(ctx, command{
		kind: cmdRerun, videoID: videoID, taskIDs: seeds, dryRun: dryRun, out: &out,
	})
	return out, err
}

// MarkStale flags everything downstream of the seeds without touching the seeds
// themselves. It is what an edit made outside the pipeline calls.
func (s *Scheduler) MarkStale(
	ctx context.Context,
	videoID entity.VideoID,
	seeds []entity.TaskID,
) ([]entity.TaskID, error) {
	var out []entity.TaskID
	err := s.send(ctx, command{kind: cmdMarkStale, videoID: videoID, taskIDs: seeds, out: &out})
	return out, err
}

// RunStale re-runs stale tasks. A nil id list means every stale task of the
// video, which is the ordinary case.
func (s *Scheduler) RunStale(
	ctx context.Context,
	videoID entity.VideoID,
	ids []entity.TaskID,
) (int, error) {
	var n int
	err := s.send(ctx, command{kind: cmdRunStale, videoID: videoID, taskIDs: ids, count: &n})
	return n, err
}

// AcceptStale clears the flag without re-running anything: the artifact has
// been looked at and is still good. A nil id list means every stale task.
func (s *Scheduler) AcceptStale(
	ctx context.Context,
	videoID entity.VideoID,
	ids []entity.TaskID,
) (int, error) {
	var n int
	err := s.send(ctx, command{kind: cmdAcceptStale, videoID: videoID, taskIDs: ids, count: &n})
	return n, err
}

// Forget drops a video from the loop, for a delete or a full re-enqueue.
func (s *Scheduler) Forget(ctx context.Context, videoID entity.VideoID) error {
	return s.send(ctx, command{kind: cmdForget, videoID: videoID})
}

// SetPoolLimit applies a settings change to a pool without a restart.
func (s *Scheduler) SetPoolLimit(ctx context.Context, pool entity.Pool, limit int) error {
	return s.send(ctx, command{kind: cmdSetPoolLimit, pool: pool, limit: limit})
}

func (s *Scheduler) doSubmit(g *Graph) error {
	if _, exists := s.graphs[g.VideoID]; exists {
		return nil
	}
	s.install(g)
	return nil
}

func (s *Scheduler) doExpand(videoID entity.VideoID, tail Tail) error {
	g, ok := s.graphs[videoID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownVideo, videoID)
	}
	if g.Expanded() {
		// A repeated expansion describes the same shape; a different one was
		// computed from a different chapter set and must not be spliced.
		if got, want := g.NodeCount(), 1+len(tail.Tasks); got != want {
			return fmt.Errorf("%w: %s is expanded to %d nodes, tail describes %d",
				ErrAlreadyExpanded, videoID, got, want)
		}
		return nil
	}
	if err := g.Expand(tail); err != nil {
		return err
	}
	for i := range tail.Tasks {
		s.taskIndex[tail.Tasks[i].ID] = videoID
	}
	s.touch(videoID)
	s.dirty = true
	s.log.Info("graph expanded",
		slog.String("video_id", videoID.String()),
		slog.Int("chapters", chapterCountOf(g)),
		slog.Int("tasks", g.NodeCount()))
	return nil
}

// chapterCountOf counts the video's chapter branches, for the log line.
func chapterCountOf(g *Graph) int {
	n := 0
	for i := range g.tasks {
		if g.tasks[i].Kind == entity.TaskKindClip {
			n++
		}
	}
	return n
}

func (s *Scheduler) doResume(graphs []*Graph) error {
	for _, g := range graphs {
		if g == nil {
			continue
		}
		if _, exists := s.graphs[g.VideoID]; exists {
			continue
		}
		s.install(g)
	}
	s.log.Info("scheduler resumed", slog.Int("videos", len(s.graphs)))
	return nil
}

// install admits a graph and rebuilds the ready set from persisted state. A
// task caught mid-flight by a crash is reclaimed as ready — its provider call
// did not finish, and every step is idempotent.
func (s *Scheduler) install(g *Graph) {
	s.graphs[g.VideoID] = g
	g.markInstalled()
	now := time.Now()
	s.reclaimCommittedBlueprint(g, now)
	for i := range g.tasks {
		t := &g.tasks[i]
		s.taskIndex[t.ID] = g.VideoID
		switch t.State {
		case entity.TaskStateRunning:
			t.State = entity.TaskStateReady
			t.StartedAt = nil
			t.UpdatedAt = now
			s.ready.Push(t)
			s.record(t)
		case entity.TaskStateReady:
			s.ready.Push(t)
		case entity.TaskStateBlocked:
			switch {
			case t.NotBefore != nil:
				s.retries.push(retryItem{taskID: t.ID, videoID: g.VideoID, when: *t.NotBefore})
			case t.DepsRemaining == 0:
				t.State = entity.TaskStateReady
				t.UpdatedAt = now
				s.ready.Push(t)
				s.record(t)
			}
		case entity.TaskStateAwaitingApproval, entity.TaskStateSucceeded,
			entity.TaskStateFailed, entity.TaskStateCancelled:
			// Nothing to schedule.
		default:
			// Unknown persisted state: treat as blocked.
			t.State = entity.TaskStateBlocked
			s.record(t)
		}
	}
	s.touch(g.VideoID)
	s.dirty = true
}

// reclaimCommittedBlueprint settles a blueprint caught in flight by a crash
// after it had already expanded the graph. Expansion is its commit point, so
// re-running it would roll a fresh outline under a graph shaped by the old one.
// It runs before the reclaim pass so dependents are discharged exactly once.
func (s *Scheduler) reclaimCommittedBlueprint(g *Graph, now time.Time) {
	if !g.Expanded() {
		return
	}
	idx, ok := g.IndexOf(entity.NewTaskID(g.VideoID, entity.TaskKindBlueprint, -1, -1))
	if !ok {
		return
	}
	t := &g.tasks[idx]
	if t.State != entity.TaskStateRunning {
		return
	}
	t.State = entity.TaskStateSucceeded
	t.NotBefore = nil
	t.FinishedAt = &now
	t.UpdatedAt = now
	s.record(t)
	for _, d := range g.Dependents(idx) {
		dep := g.Task(int(d))
		if dep.State != entity.TaskStateBlocked || dep.DepsRemaining == 0 {
			continue
		}
		dep.DepsRemaining--
		dep.UpdatedAt = now
		s.record(dep)
	}
	s.log.Info("blueprint reclaimed as committed",
		slog.String("video_id", g.VideoID.String()),
		slog.String("task", t.ID.String()))
}

func (s *Scheduler) resolve(taskID entity.TaskID) (*Graph, *entity.Task, error) {
	videoID, ok := s.taskIndex[taskID]
	if !ok {
		return nil, nil, fmt.Errorf("%w: %s", ErrUnknownTask, taskID)
	}
	g, ok := s.graphs[videoID]
	if !ok {
		return nil, nil, fmt.Errorf("%w: %s", ErrUnknownVideo, videoID)
	}
	t, ok := g.TaskByID(taskID)
	if !ok {
		return nil, nil, fmt.Errorf("%w: %s", ErrUnknownTask, taskID)
	}
	return g, t, nil
}

func (s *Scheduler) doApprove(taskID entity.TaskID) error {
	g, t, err := s.resolve(taskID)
	if err != nil {
		return err
	}
	if t.State != entity.TaskStateAwaitingApproval {
		return fmt.Errorf("%w: %s is %s", ErrNotGated, taskID, t.State)
	}
	t.UpdatedAt = time.Now()
	s.markSucceeded(g, t)
	s.record(t)
	s.log.Info("gate approved",
		slog.String("video_id", t.VideoID.String()),
		slog.String("task", t.ID.String()),
		slog.String("gate", string(t.Gate)))
	return nil
}

func (s *Scheduler) doReject(taskID entity.TaskID, reason string) error {
	_, t, err := s.resolve(taskID)
	if err != nil {
		return err
	}
	if t.State != entity.TaskStateAwaitingApproval {
		return fmt.Errorf("%w: %s is %s", ErrNotGated, taskID, t.State)
	}
	if reason == "" {
		reason = "rejected by operator"
	}
	t.State = entity.TaskStateFailed
	t.Error = reason
	t.UpdatedAt = time.Now()
	s.record(t)
	s.log.Info("gate rejected",
		slog.String("video_id", t.VideoID.String()),
		slog.String("task", t.ID.String()),
		slog.String("reason", reason))
	return nil
}

func (s *Scheduler) doCancel(videoID entity.VideoID) error {
	g, ok := s.graphs[videoID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownVideo, videoID)
	}
	if run, ok := s.runs[videoID]; ok {
		run.cancel()
		delete(s.runs, videoID)
	}
	now := time.Now()
	for i := range g.tasks {
		t := &g.tasks[i]
		if t.State.Terminal() {
			continue
		}
		t.State = entity.TaskStateCancelled
		t.NotBefore = nil
		t.UpdatedAt = now
		s.record(t)
	}
	s.ready.Compact()
	s.log.Info("video cancelled", slog.String("video_id", videoID.String()))
	return nil
}

// guardBlueprintReset refuses to re-run a blueprint that is not `failed`.
// Expansion is one-way, so a second roll of the outline would leave tasks
// pointing at chapters that no longer exist. `failed` is safe because nothing
// was expanded from it; a cancelled blueprint is too, but only while the graph
// is still one node — expansion, not the state, is what must not happen twice.
func guardBlueprintReset(g *Graph, indices []int32) error {
	for _, i := range indices {
		t := &g.tasks[i]
		if t.Kind != entity.TaskKindBlueprint || t.State == entity.TaskStateFailed {
			continue
		}
		if t.State == entity.TaskStateCancelled && !g.Expanded() {
			continue
		}
		return fmt.Errorf("%w: %s is %s; reject it first, or start a new video",
			ErrBlueprintLocked, t.ID, t.State)
	}
	return nil
}

func (s *Scheduler) doRetryTask(taskID entity.TaskID) error {
	g, t, err := s.resolve(taskID)
	if err != nil {
		return err
	}
	idx, _ := g.IndexOf(t.ID)
	seeds := []int32{idx}
	if err := guardBlueprintReset(g, seeds); err != nil {
		return err
	}
	s.resetFrom(g, seeds)
	return nil
}

func (s *Scheduler) doRetryChapter(videoID entity.VideoID, ordinal int) error {
	g, ok := s.graphs[videoID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownVideo, videoID)
	}
	seeds := make([]int32, 0, 8)
	for i := range g.tasks {
		if g.tasks[i].Ordinal == ordinal {
			seeds = append(seeds, int32(i)) //nolint:gosec // bounded by graph size
		}
	}
	if len(seeds) == 0 {
		return fmt.Errorf("%w: video %s has no chapter %d", ErrUnknownTask, videoID, ordinal)
	}
	// Video-level tasks carry ordinal -1, so that ordinal sweeps the blueprint in.
	if err := guardBlueprintReset(g, seeds); err != nil {
		return err
	}
	s.resetFrom(g, seeds)
	return nil
}

func (s *Scheduler) doRequeue(c command) error {
	g, ok := s.graphs[c.videoID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownVideo, c.videoID)
	}
	seeds := make([]int32, 0, 16)
	for i := range g.tasks {
		switch g.tasks[i].State {
		case entity.TaskStateCancelled, entity.TaskStateFailed:
			seeds = append(seeds, int32(i)) //nolint:gosec // bounded by graph size
		case entity.TaskStateBlocked, entity.TaskStateReady, entity.TaskStateRunning,
			entity.TaskStateAwaitingApproval, entity.TaskStateSucceeded:
			// Still open, or holding an artifact: not what it stopped on.
		}
	}
	if len(seeds) == 0 {
		*c.count = 0
		return nil
	}
	if err := guardBlueprintReset(g, seeds); err != nil {
		return err
	}
	s.resetFrom(g, seeds)
	*c.count = len(seeds)
	return nil
}

// strictDownstream returns everything reachable from the seeds, excluding the
// seeds themselves. That exclusion is the difference between a re-run and a
// retry: the seeds are redone, their descendants only flagged.
func strictDownstream(g *Graph, seeds []int32) []int32 {
	affected := make([]bool, len(g.tasks))
	for _, seed := range seeds {
		for _, idx := range g.Downstream(seed) {
			affected[idx] = true
		}
	}
	for _, seed := range seeds {
		affected[seed] = false
	}
	out := make([]int32, 0, 16)
	for i, hit := range affected {
		if hit {
			out = append(out, int32(i)) //nolint:gosec // bounded by graph size
		}
	}
	return out
}

// resolveSeeds maps task ids to node indices, rejecting anything unknown.
func resolveSeeds(g *Graph, ids []entity.TaskID) ([]int32, error) {
	seeds := make([]int32, 0, len(ids))
	for _, id := range ids {
		idx, ok := g.IndexOf(id)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownTask, id)
		}
		seeds = append(seeds, idx)
	}
	if len(seeds) == 0 {
		return nil, fmt.Errorf("%w: no tasks given", ErrUnknownTask)
	}
	return seeds, nil
}

func (s *Scheduler) doRerun(c command) error {
	g, ok := s.graphs[c.videoID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownVideo, c.videoID)
	}
	seeds, err := resolveSeeds(g, c.taskIDs)
	if err != nil {
		return err
	}
	if err := guardBlueprintReset(g, seeds); err != nil {
		return err
	}
	downstream := strictDownstream(g, seeds)

	// Report what will be flagged, not everything reachable: a task that never
	// ran is not marked, so listing it would overstate the blast radius.
	if c.out != nil {
		ids := make([]entity.TaskID, 0, len(downstream))
		for _, idx := range downstream {
			if producedOutput(g.tasks[idx].State) {
				ids = append(ids, g.tasks[idx].ID)
			}
		}
		*c.out = ids
	}
	if c.dryRun {
		return nil
	}

	s.markStale(g, downstream)
	// resetNodes, not resetFrom: only the seeds re-run. Their dependents are
	// `succeeded`, so releaseDependents passes over them and the stale set waits.
	s.resetNodes(g, seeds)
	return nil
}

func (s *Scheduler) doMarkStale(c command) error {
	g, ok := s.graphs[c.videoID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownVideo, c.videoID)
	}
	seeds, err := resolveSeeds(g, c.taskIDs)
	if err != nil {
		return err
	}
	downstream := strictDownstream(g, seeds)
	if c.out != nil {
		ids := make([]entity.TaskID, 0, len(downstream))
		for _, idx := range downstream {
			if producedOutput(g.tasks[idx].State) {
				ids = append(ids, g.tasks[idx].ID)
			}
		}
		*c.out = ids
	}
	s.markStale(g, downstream)
	return nil
}

// markStale flags only tasks that produced something; one that never ran is
// merely pending. A gated task counts — its artifact exists.
func (s *Scheduler) markStale(g *Graph, indices []int32) {
	now := time.Now()
	for _, idx := range indices {
		t := &g.tasks[idx]
		if t.Stale || !producedOutput(t.State) {
			continue
		}
		t.Stale = true
		t.UpdatedAt = now
		s.record(t)
	}
	s.log.Info("tasks marked stale",
		slog.String("video_id", g.VideoID.String()),
		slog.Int("count", len(indices)))
}

// producedOutput reports whether a task has an artifact staleness could be about.
func producedOutput(s entity.TaskState) bool {
	return s == entity.TaskStateSucceeded || s == entity.TaskStateAwaitingApproval
}

// staleIndices resolves the requested ids, or every stale task when none are
// given. Ids that are no longer stale are ignored, so a UI acting on a list
// that moved under it is idempotent rather than an error.
func staleIndices(g *Graph, ids []entity.TaskID) []int32 {
	out := make([]int32, 0, 16)
	if len(ids) == 0 {
		for i := range g.tasks {
			if g.tasks[i].Stale {
				out = append(out, int32(i)) //nolint:gosec // bounded by graph size
			}
		}
		return out
	}
	for _, id := range ids {
		if idx, ok := g.IndexOf(id); ok && g.tasks[idx].Stale {
			out = append(out, idx)
		}
	}
	return out
}

func (s *Scheduler) doRunStale(c command) error {
	g, ok := s.graphs[c.videoID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownVideo, c.videoID)
	}
	indices := staleIndices(g, c.taskIDs)
	if err := guardBlueprintReset(g, indices); err != nil {
		return err
	}
	if c.count != nil {
		*c.count = len(indices)
	}
	if len(indices) == 0 {
		return nil
	}
	// Clearing the flag first re-admits them as ordinary work.
	for _, idx := range indices {
		g.tasks[idx].Stale = false
	}
	s.resetNodes(g, indices)
	return nil
}

func (s *Scheduler) doAcceptStale(c command) error {
	g, ok := s.graphs[c.videoID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownVideo, c.videoID)
	}
	indices := staleIndices(g, c.taskIDs)
	if c.count != nil {
		*c.count = len(indices)
	}
	now := time.Now()
	for _, idx := range indices {
		t := &g.tasks[idx]
		t.Stale = false
		t.UpdatedAt = now
		s.record(t)
	}
	if len(indices) > 0 {
		s.log.Info("stale tasks accepted",
			slog.String("video_id", g.VideoID.String()),
			slog.Int("count", len(indices)))
	}
	return nil
}

// resetFrom clears the seeds and everything reachable from them, recomputes
// dependency counts against the surviving successes and re-admits what is
// runnable. A task in flight has its generation bumped, so the answer it is
// about to produce is discarded; the provider call itself is not interrupted.
func (s *Scheduler) resetFrom(g *Graph, seeds []int32) {
	affected := make([]bool, len(g.tasks))
	for _, seed := range seeds {
		for _, idx := range g.Downstream(seed) {
			affected[idx] = true
		}
	}
	closure := make([]int32, 0, len(g.tasks))
	for i, hit := range affected {
		if hit {
			closure = append(closure, int32(i)) //nolint:gosec // bounded by graph size
		}
	}
	s.resetNodes(g, closure)
	s.log.Info("tasks reset for retry",
		slog.String("video_id", g.VideoID.String()),
		slog.Int("seeds", len(seeds)),
		slog.Int("reset", len(closure)))
}

// resetNodes clears exactly the nodes it is given, without walking the graph.
// Retrying a failure resets the whole closure, since nothing in it ever ran;
// re-running a success resets only the seeds, since their descendants hold
// artifacts flagged stale and awaiting a decision.
func (s *Scheduler) resetNodes(g *Graph, indices []int32) {
	now := time.Now()
	for _, i := range indices {
		t := &g.tasks[i]
		g.bumpGeneration(i)
		t.Stale = false
		t.State = entity.TaskStateBlocked
		t.Attempt = 0
		t.Error = ""
		t.NotBefore = nil
		t.StartedAt = nil
		t.FinishedAt = nil
	}
	// Counts are recomputed only after every node is cleared, so a node sees its
	// siblings' new states rather than a half-applied mixture.
	for _, i := range indices {
		t := &g.tasks[i]
		if t.State != entity.TaskStateBlocked {
			continue
		}
		remaining := 0
		for _, dep := range g.dependencies[i] {
			if g.tasks[dep].State != entity.TaskStateSucceeded {
				remaining++
			}
		}
		t.DepsRemaining = remaining
		t.UpdatedAt = now
		if remaining == 0 {
			t.State = entity.TaskStateReady
			s.ready.Push(t)
		}
		s.record(t)
	}
}

func (s *Scheduler) doForget(videoID entity.VideoID) error {
	g, ok := s.graphs[videoID]
	if !ok {
		return nil
	}
	if run, ok := s.runs[videoID]; ok {
		run.cancel()
		delete(s.runs, videoID)
	}
	for i := range g.tasks {
		delete(s.taskIndex, g.tasks[i].ID)
	}
	delete(s.graphs, videoID)
	delete(s.counts, videoID)
	delete(s.touched, videoID)
	s.ready.Compact()
	s.dirty = true
	return nil
}
