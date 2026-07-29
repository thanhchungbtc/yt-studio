package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
)

// cmdKind enumerates the operations the loop accepts. Every public method is a
// thin wrapper that hands one of these to the loop and waits, which keeps all
// mutable scheduler state owned by a single goroutine.
type cmdKind uint8

// The complete set of commands.
const (
	cmdSubmit cmdKind = iota
	cmdResume
	cmdApprove
	cmdReject
	cmdCancel
	cmdRetryTask
	cmdRetryChapter
	cmdForget
	cmdSetPoolLimit
)

type command struct {
	kind    cmdKind
	graph   *Graph
	graphs  []*Graph
	taskID  entity.TaskID
	videoID entity.VideoID
	ordinal int
	pool    entity.Pool
	limit   int
	reason  string
	reply   chan error
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
// deterministic, so submitting the same video twice is idempotent (§3).
func (s *Scheduler) Submit(ctx context.Context, g *Graph) error {
	if g == nil {
		return fmt.Errorf("%w: nil graph", ErrInvalidGraph)
	}
	if g.Installed() {
		// The loop already owns this graph and is mutating its tasks; reading
		// them here to persist them again would be both a race and pointless.
		return nil
	}
	if err := s.store.InsertGraph(ctx, g.VideoID, g.Tasks(), g.Edges()); err != nil {
		return fmt.Errorf("persist graph: %w", err)
	}
	return s.send(ctx, command{kind: cmdSubmit, graph: g})
}

// Resume admits graphs rebuilt from the database at startup. A crash 45 minutes
// into a run resumes rather than restarts (§2, principle 4).
func (s *Scheduler) Resume(ctx context.Context, graphs []*Graph) error {
	return s.send(ctx, command{kind: cmdResume, graphs: graphs})
}

// Approve releases a gated task's successors. A gate is a row update (§6).
func (s *Scheduler) Approve(ctx context.Context, taskID entity.TaskID) error {
	return s.send(ctx, command{kind: cmdApprove, taskID: taskID})
}

// Reject fails a gated task with an operator-supplied reason.
func (s *Scheduler) Reject(ctx context.Context, taskID entity.TaskID, reason string) error {
	return s.send(ctx, command{kind: cmdReject, taskID: taskID, reason: reason})
}

// Cancel stops a video. Its in-flight provider calls are cancelled through the
// per-video context, so its slots come back within 100 ms (§8.3).
func (s *Scheduler) Cancel(ctx context.Context, videoID entity.VideoID) error {
	return s.send(ctx, command{kind: cmdCancel, videoID: videoID})
}

// RetryTask resets one task and everything downstream of it.
func (s *Scheduler) RetryTask(ctx context.Context, taskID entity.TaskID) error {
	return s.send(ctx, command{kind: cmdRetryTask, taskID: taskID})
}

// RetryChapter resets every task of one chapter and everything downstream (§9).
func (s *Scheduler) RetryChapter(ctx context.Context, videoID entity.VideoID, ordinal int) error {
	return s.send(ctx, command{kind: cmdRetryChapter, videoID: videoID, ordinal: ordinal})
}

// Forget drops a video from the loop, for a delete or a full re-enqueue.
func (s *Scheduler) Forget(ctx context.Context, videoID entity.VideoID) error {
	return s.send(ctx, command{kind: cmdForget, videoID: videoID})
}

// SetPoolLimit applies a settings change to a pool without a restart (§5).
func (s *Scheduler) SetPoolLimit(ctx context.Context, pool entity.Pool, limit int) error {
	return s.send(ctx, command{kind: cmdSetPoolLimit, pool: pool, limit: limit})
}

func (s *Scheduler) doSubmit(g *Graph) error {
	if _, exists := s.graphs[g.VideoID]; exists {
		// Already admitted; nothing to do. Idempotent by construction.
		return nil
	}
	s.install(g)
	return nil
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

// install admits a graph and reconstructs the ready set from persisted state.
// A task caught mid-flight by a crash is reclaimed as ready: its provider call
// did not finish, and every step is idempotent by design (§2, principle 4).
func (s *Scheduler) install(g *Graph) {
	s.graphs[g.VideoID] = g
	g.markInstalled()
	now := time.Now()
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
			// Unknown persisted state: treat conservatively as blocked.
			t.State = entity.TaskStateBlocked
			s.record(t)
		}
	}
	s.touch(g.VideoID)
	s.dirty = true
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

func (s *Scheduler) doRetryTask(taskID entity.TaskID) error {
	g, t, err := s.resolve(taskID)
	if err != nil {
		return err
	}
	idx, _ := g.IndexOf(t.ID)
	s.resetFrom(g, []int32{idx})
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
	s.resetFrom(g, seeds)
	return nil
}

// resetFrom clears the seeds and everything reachable from them, recomputes
// their dependency counts against the surviving successes and re-admits
// whatever is now runnable. A task still in flight is left alone; its
// completion is discarded because its state has moved on.
func (s *Scheduler) resetFrom(g *Graph, seeds []int32) {
	affected := make([]bool, len(g.tasks))
	for _, seed := range seeds {
		for _, idx := range g.Downstream(seed) {
			affected[idx] = true
		}
	}
	now := time.Now()
	for i, hit := range affected {
		if !hit {
			continue
		}
		t := &g.tasks[i]
		if t.State == entity.TaskStateRunning {
			continue
		}
		t.State = entity.TaskStateBlocked
		t.Attempt = 0
		t.Error = ""
		t.NotBefore = nil
		t.StartedAt = nil
		t.FinishedAt = nil
	}
	for i, hit := range affected {
		if !hit {
			continue
		}
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
	s.log.Info("tasks reset for retry",
		slog.String("video_id", g.VideoID.String()),
		slog.Int("seeds", len(seeds)))
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
