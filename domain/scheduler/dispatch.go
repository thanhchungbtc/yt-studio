package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v4"
	"golang.org/x/sync/errgroup"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// Errors returned by the scheduler's command API.
var (
	ErrSchedulerClosed = errors.New("scheduler is not running")
	ErrUnknownVideo    = errors.New("unknown video")
	ErrUnknownTask     = errors.New("unknown task")
	ErrNotGated        = errors.New("task is not awaiting approval")
	// ErrBlueprintLocked reports an attempt to re-run a blueprint the video's DAG
	// has already been built from. See guardBlueprintReset.
	ErrBlueprintLocked = errors.New("blueprint cannot be re-run")
)

// Runner executes exactly one task and reports its outcome. Everything the
// server owns — lifecycle, the DAG, pools, retries, persistence, gates —
// stays outside it; a Runner does the provider call and records the artifacts.
//
// It takes the task by value: the scheduler keeps mutating its own copy from
// the dispatch goroutine, and a shared pointer would be a data race.
type Runner interface {
	Run(ctx context.Context, t entity.Task) entity.TaskOutcome
}

// TransitionStore is the scheduler's durable backing: the narrow half of
// repository.TaskWriter that dispatch actually uses.
type TransitionStore interface {
	InsertGraph(ctx context.Context, videoID entity.VideoID, tasks []entity.Task, edges []repository.TaskEdge) error
	ApplyTransitions(ctx context.Context, transitions []repository.TaskTransition) error
}

// VideoLifecycle lets the scheduler record derived video state without knowing
// how videos are stored.
type VideoLifecycle interface {
	SetVideoState(ctx context.Context, id entity.VideoID, state entity.VideoState, errMsg string) error
}

// Notifier receives deltas for the SSE stream. Implementations must not block.
type Notifier interface {
	NotifyTask(d entity.TaskDelta)
	NotifyVideo(d entity.VideoDelta)
	NotifyScheduler(d entity.SchedulerDelta)
}

// Config holds the scheduler's tunables, all sourced from settings rows.
type Config struct {
	RetryBase time.Duration
	RetryMax  time.Duration
	// SafetyInterval is the in-memory consistency sweep. Polling is forbidden as
	// the primary mechanism; this exists only as a net.
	SafetyInterval time.Duration
	// CompletionBuffer sizes the channel workers report on.
	CompletionBuffer int
}

func (c Config) withDefaults() Config {
	if c.RetryBase <= 0 {
		c.RetryBase = 250 * time.Millisecond
	}
	if c.RetryMax <= 0 {
		c.RetryMax = 30 * time.Second
	}
	if c.SafetyInterval < 30*time.Second {
		c.SafetyInterval = 30 * time.Second
	}
	if c.CompletionBuffer <= 0 {
		c.CompletionBuffer = 256
	}
	return c
}

// Status is the operator console's view of the scheduler.
type Status struct {
	Pools            []entity.PoolStat `json:"pools"`
	Ready            int               `json:"ready"`
	Running          int               `json:"running"`
	Blocked          int               `json:"blocked"`
	AwaitingApproval int               `json:"awaitingApproval"`
	Succeeded        int               `json:"succeeded"`
	Failed           int               `json:"failed"`
	RetryPending     int               `json:"retryPending"`
	Videos           int               `json:"videos"`
	StartedAt        time.Time         `json:"startedAt"`
	UptimeSeconds    float64           `json:"uptimeSeconds"`
}

// counters is a video's task census, recomputed from its graph whenever
// anything about that video changes. Deriving it rather than maintaining it
// incrementally removes a whole class of drift bugs for a few hundred integer
// comparisons, which is nothing next to the transaction that follows.
type counters struct {
	total            int
	succeeded        int
	failed           int
	running          int
	ready            int
	blocked          int
	awaitingApproval int
	cancelled        int
	retryPending     int
	stale            int
	lastError        string
	state            entity.VideoState
	gate             entity.GateKind
}

type completion struct {
	taskID  entity.TaskID
	videoID entity.VideoID
	outcome entity.TaskOutcome
	at      time.Time
	// generation the task was dispatched under. A reset in the meantime bumps the
	// graph's counter and this answer is thrown away.
	generation uint64
}

// videoRun carries the cancellation scope of one video. Cancelling it stops
// every in-flight provider call for that video, which is how a cancelled video
// frees its slots within 100 ms.
type videoRun struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// Scheduler is the event-driven dispatch loop. Every task starts the moment its
// own dependencies are met, regardless of what other chapters are doing; there
// are no stage barriers.
//
// All mutable state below the ports is owned by the single loop goroutine.
// Public methods are thin: they hand a command to the loop and wait.
type Scheduler struct {
	pools     *Pools
	ready     *ReadySet
	store     TransitionStore
	runner    Runner
	lifecycle VideoLifecycle
	notifier  Notifier
	log       *slog.Logger
	cfg       Config

	cmds        chan command
	completions chan completion

	// Loop-owned state below. Never touched from another goroutine.
	graphs    map[entity.VideoID]*Graph
	counts    map[entity.VideoID]*counters
	runs      map[entity.VideoID]*videoRun
	taskIndex map[entity.TaskID]entity.VideoID
	touched   map[entity.VideoID]struct{}
	// pending and spare alternate: a batch handed to the store is never the buffer
	// the loop keeps appending to. Reusing a single slice would alias the store's
	// view of it, and swapping keeps the steady state allocation free either way.
	pending  []repository.TaskTransition
	spare    []repository.TaskTransition
	retries  retryQueue
	inFlight int
	dirty    bool

	workers sync.WaitGroup
	status  atomic.Pointer[Status]
	// startedAt is read by the console from other goroutines, so it is stored as
	// unix nanoseconds rather than a plain field.
	startedAt atomic.Int64
	running   atomic.Bool
}

func (s *Scheduler) started() time.Time { return time.Unix(0, s.startedAt.Load()) }

// New constructs a Scheduler. Every dependency is an explicit parameter: the
// signature is the whole dependency list.
func New(
	pools *Pools,
	store TransitionStore,
	runner Runner,
	lifecycle VideoLifecycle,
	notifier Notifier,
	log *slog.Logger,
	cfg Config,
) *Scheduler {
	cfg = cfg.withDefaults()
	s := &Scheduler{
		pools:       pools,
		ready:       NewReadySet(),
		store:       store,
		runner:      runner,
		lifecycle:   lifecycle,
		notifier:    notifier,
		log:         log,
		cfg:         cfg,
		cmds:        make(chan command),
		completions: make(chan completion, cfg.CompletionBuffer),
		graphs:      make(map[entity.VideoID]*Graph, 16),
		counts:      make(map[entity.VideoID]*counters, 16),
		runs:        make(map[entity.VideoID]*videoRun, 16),
		taskIndex:   make(map[entity.TaskID]entity.VideoID, 1024),
		touched:     make(map[entity.VideoID]struct{}, 16),
		pending:     make([]repository.TaskTransition, 0, 512),
		spare:       make([]repository.TaskTransition, 0, 512),
	}
	s.startedAt.Store(time.Now().UnixNano())
	s.dirty = true
	s.publishStatus()
	return s
}

// Run owns the dispatch loop and the pool reconcilers, and returns only after
// every goroutine it started has exited.
func (s *Scheduler) Run(ctx context.Context) error {
	s.startedAt.Store(time.Now().UnixNano())
	s.running.Store(true)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return s.pools.Run(gctx) })
	g.Go(func() error {
		defer s.running.Store(false)
		return s.loop(gctx)
	})
	err := g.Wait()
	s.workers.Wait()
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}

func (s *Scheduler) loop(ctx context.Context) error {
	safety := time.NewTicker(s.cfg.SafetyInterval)
	defer safety.Stop()
	retryTimer := time.NewTimer(time.Hour)
	if !retryTimer.Stop() {
		<-retryTimer.C
	}
	defer retryTimer.Stop()
	armed := false

	done := ctx.Done()
	draining := false

	for {
		select {
		case c := <-s.cmds:
			s.handleCommand(ctx, c)
		case c := <-s.completions:
			s.handleCompletion(c)
			s.drainCompletions()
		case <-retryTimer.C:
			armed = false
			s.releaseDueRetries(time.Now())
		case <-safety.C:
			s.sweep()
		case <-done:
			// ctx.Done stays readable forever; disarm it so the loop cannot spin.
			done = nil
			draining = true
			s.log.Info("scheduler draining", slog.Int("in_flight", s.inFlight))
		}

		if !draining {
			s.pump(ctx)
		}
		s.settle(ctx)

		if !armed {
			if when, ok := s.retries.earliest(); ok {
				d := time.Until(when)
				if d < 0 {
					d = 0
				}
				retryTimer.Reset(d)
				armed = true
			}
		}

		if draining && s.inFlight == 0 {
			s.settle(context.WithoutCancel(ctx))
			return ctx.Err()
		}
	}
}

// settle recomputes derived state for every video touched this iteration,
// commits the batch in one transaction and publishes the console snapshot.
func (s *Scheduler) settle(ctx context.Context) {
	// Task rows commit first. The video row is derived from them, so writing it
	// first would let an API read observe a video parked on a gate whose task
	// still claims to be running.
	s.flush(ctx)
	for videoID := range s.touched {
		s.recount(videoID)
		s.updateVideoState(ctx, videoID)
		delete(s.touched, videoID)
	}
	s.publishStatus()
}

// drainCompletions batches everything already reported so that N tasks
// finishing together commit in one transaction.
func (s *Scheduler) drainCompletions() {
	const maxBatch = 128
	for range maxBatch {
		select {
		case c := <-s.completions:
			s.handleCompletion(c)
		default:
			return
		}
	}
}

// pump is the dispatch decision: for every pool, start as many ready tasks as
// there are free slots. Selection is greedy-with-skip and cannot deadlock — a
// task holds at most one slot and never acquires a second, so there is no
// hold-and-wait.
func (s *Scheduler) pump(ctx context.Context) {
	for _, pool := range entity.AllPools {
		for {
			t := s.ready.Next(pool)
			if t == nil {
				break
			}
			if !s.pools.TryAcquire(pool) {
				break
			}
			s.ready.Pop(pool)
			s.startTask(ctx, t)
		}
	}
}

func (s *Scheduler) startTask(ctx context.Context, t *entity.Task) {
	now := time.Now()
	t.State = entity.TaskStateRunning
	t.Attempt++
	t.StartedAt = &now
	t.UpdatedAt = now
	t.Error = ""
	t.NotBefore = nil
	s.record(t)
	s.inFlight++

	vctx := s.videoContext(ctx, t.VideoID)
	snapshot := *t // by value: the loop keeps mutating its own copy
	id, videoID := t.ID, t.VideoID

	// Captured at dispatch: whatever happens to the task while the provider call
	// is in flight, the completion can be matched against the state it was started
	// from.
	var generation uint64
	if g, ok := s.graphs[videoID]; ok {
		if idx, found := g.IndexOf(id); found {
			generation = g.Generation(idx)
		}
	}

	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		outcome := runSafely(vctx, s.runner, snapshot)
		s.pools.Release(snapshot.Pool)
		s.completions <- completion{
			taskID:     id,
			videoID:    videoID,
			outcome:    outcome,
			at:         time.Now(),
			generation: generation,
		}
	}()
}

// runSafely turns a panicking Runner into a permanent task failure instead of a
// dead server. A provider is replaceable third-party code by design.
func runSafely(ctx context.Context, r Runner, t entity.Task) (outcome entity.TaskOutcome) {
	defer func() {
		if rec := recover(); rec != nil {
			outcome = entity.Failed{Err: fmt.Errorf("runner panicked: %v", rec), Retryable: false}
		}
	}()
	return r.Run(ctx, t)
}

func (s *Scheduler) handleCompletion(c completion) {
	// Every completion corresponds to exactly one startTask, so the in-flight
	// count is decremented before any early return.
	s.inFlight--

	g, ok := s.graphs[c.videoID]
	if !ok {
		return
	}
	t, ok := g.TaskByID(c.taskID)
	if !ok {
		return
	}
	if idx, found := g.IndexOf(c.taskID); found && g.Generation(idx) != c.generation {
		// Reset while in flight. The answer is to a question that has since changed,
		// so it is discarded; resetFrom has already re-queued the task.
		return
	}
	if t.State != entity.TaskStateRunning {
		// Cancelled while in flight; the result is discarded.
		return
	}
	t.FinishedAt = &c.at
	t.UpdatedAt = c.at

	switch o := c.outcome.(type) {
	case entity.Success:
		if t.Gate != entity.GateNone {
			s.parkOnGate(t, t.Gate)
			break
		}
		s.markSucceeded(g, t)
	case entity.AwaitingApproval:
		gate := o.Gate
		if gate == entity.GateNone {
			gate = t.Gate
		}
		if gate == entity.GateNone {
			s.markSucceeded(g, t)
			break
		}
		s.parkOnGate(t, gate)
	case entity.Failed:
		s.handleFailure(t, o)
	default:
		panic(fmt.Sprintf("unhandled outcome %T", o))
	}

	s.record(t)
	s.touch(c.videoID)
}

func (s *Scheduler) parkOnGate(t *entity.Task, gate entity.GateKind) {
	t.State = entity.TaskStateAwaitingApproval
	t.Gate = gate
	t.Error = ""
	s.log.Info("task awaiting approval",
		slog.String("video_id", t.VideoID.String()),
		slog.String("task", t.ID.String()),
		slog.String("gate", string(gate)))
}

func (s *Scheduler) markSucceeded(g *Graph, t *entity.Task) {
	t.State = entity.TaskStateSucceeded
	t.Error = ""
	idx, _ := g.IndexOf(t.ID)
	s.releaseDependents(g, idx)
}

// releaseDependents is the whole of dependency propagation: a completed task
// signals its dependents directly. The task table is never rescanned to find
// work.
func (s *Scheduler) releaseDependents(g *Graph, idx int32) {
	now := time.Now()
	for _, d := range g.Dependents(idx) {
		dep := g.Task(int(d))
		if dep.State != entity.TaskStateBlocked || dep.NotBefore != nil {
			continue
		}
		if dep.DepsRemaining > 0 {
			dep.DepsRemaining--
		}
		if dep.DepsRemaining > 0 {
			s.record(dep)
			continue
		}
		dep.State = entity.TaskStateReady
		dep.UpdatedAt = now
		s.ready.Push(dep)
		s.record(dep)
	}
}

func (s *Scheduler) handleFailure(t *entity.Task, o entity.Failed) {
	msg := "unknown error"
	if o.Err != nil {
		msg = o.Err.Error()
	}
	t.Error = msg
	if o.Retryable && t.Retryable() {
		delay := retryDelay(s.cfg.RetryBase, s.cfg.RetryMax, t.Attempt)
		when := time.Now().Add(delay)
		t.State = entity.TaskStateBlocked
		t.NotBefore = &when
		s.retries.push(retryItem{taskID: t.ID, videoID: t.VideoID, when: when})
		s.log.Warn("task failed, retrying",
			slog.String("video_id", t.VideoID.String()),
			slog.String("task", t.ID.String()),
			slog.Int("attempt", t.Attempt),
			slog.Duration("in", delay),
			slog.String("error", msg))
		return
	}
	t.State = entity.TaskStateFailed
	t.NotBefore = nil
	s.log.Error("task failed permanently",
		slog.String("video_id", t.VideoID.String()),
		slog.String("task", t.ID.String()),
		slog.Int("attempt", t.Attempt),
		slog.String("error", msg))
}

// retryDelay uses cenkalti/backoff's exponential schedule rather than a
// hand-rolled one. A fresh instance advanced to the current attempt keeps the
// scheduler free of per-task retry state.
func retryDelay(base, maxDelay time.Duration, attempt int) time.Duration {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = base
	b.MaxInterval = maxDelay
	b.MaxElapsedTime = 0
	b.Reset()
	d := b.NextBackOff()
	for i := 1; i < attempt; i++ {
		next := b.NextBackOff()
		if next == backoff.Stop {
			break
		}
		d = next
	}
	if d == backoff.Stop || d > maxDelay {
		return maxDelay
	}
	return d
}

func (s *Scheduler) releaseDueRetries(now time.Time) {
	for {
		item, ok := s.retries.popDue(now)
		if !ok {
			return
		}
		g, ok := s.graphs[item.videoID]
		if !ok {
			continue
		}
		t, ok := g.TaskByID(item.taskID)
		if !ok || t.State != entity.TaskStateBlocked || t.NotBefore == nil {
			continue
		}
		t.State = entity.TaskStateReady
		t.NotBefore = nil
		t.UpdatedAt = now
		s.ready.Push(t)
		s.record(t)
		s.touch(item.videoID)
	}
}

// sweep is the safety net: it re-derives readiness in memory for any task whose
// dependencies are satisfied but which is not queued. It never touches the
// database and never rescans the task table.
func (s *Scheduler) sweep() {
	now := time.Now()
	for videoID, g := range s.graphs {
		for i := range g.tasks {
			t := &g.tasks[i]
			if t.State != entity.TaskStateBlocked || t.DepsRemaining != 0 || t.NotBefore != nil {
				continue
			}
			t.State = entity.TaskStateReady
			t.UpdatedAt = now
			s.ready.Push(t)
			s.record(t)
			s.touch(videoID)
		}
	}
}

func (s *Scheduler) videoContext(ctx context.Context, videoID entity.VideoID) context.Context {
	run, ok := s.runs[videoID]
	if !ok {
		vctx, cancel := context.WithCancel(ctx)
		run = &videoRun{ctx: vctx, cancel: cancel}
		s.runs[videoID] = run
	}
	return run.ctx
}

func (s *Scheduler) touch(videoID entity.VideoID) { s.touched[videoID] = struct{}{} }

// record queues a durable transition and notifies listeners.
func (s *Scheduler) record(t *entity.Task) {
	s.pending = append(s.pending, repository.TaskTransition{
		ID:            t.ID,
		State:         t.State,
		Attempt:       t.Attempt,
		DepsRemaining: t.DepsRemaining,
		Stale:         t.Stale,
		Error:         t.Error,
		StartedAt:     t.StartedAt,
		FinishedAt:    t.FinishedAt,
		NotBefore:     t.NotBefore,
		UpdatedAt:     t.UpdatedAt,
	})
	s.dirty = true
	s.touch(t.VideoID)
	if s.notifier != nil {
		s.notifier.NotifyTask(t.Delta())
	}
}

func (s *Scheduler) flush(ctx context.Context) {
	if len(s.pending) == 0 {
		return
	}
	batch := s.pending
	s.pending, s.spare = s.spare[:0], s.pending
	if err := s.store.ApplyTransitions(ctx, batch); err != nil {
		s.log.Error("failed to persist task transitions",
			slog.Int("count", len(batch)),
			slog.String("error", err.Error()))
	}
}

func (s *Scheduler) recount(videoID entity.VideoID) {
	g, ok := s.graphs[videoID]
	if !ok {
		return
	}
	c, ok := s.counts[videoID]
	if !ok {
		c = &counters{}
		s.counts[videoID] = c
	}
	*c = counters{total: len(g.tasks)}
	for i := range g.tasks {
		t := &g.tasks[i]
		// Staleness cuts across the states below; it does not partition with them.
		if t.Stale {
			c.stale++
		}
		switch t.State {
		case entity.TaskStateSucceeded:
			c.succeeded++
		case entity.TaskStateFailed:
			c.failed++
			if c.lastError == "" {
				c.lastError = t.Error
			}
		case entity.TaskStateRunning:
			c.running++
		case entity.TaskStateReady:
			c.ready++
		case entity.TaskStateAwaitingApproval:
			c.awaitingApproval++
			if c.gate == entity.GateNone {
				c.gate = t.Gate
			}
		case entity.TaskStateCancelled:
			c.cancelled++
		case entity.TaskStateBlocked:
			if t.NotBefore != nil {
				c.retryPending++
			} else {
				c.blocked++
			}
		default:
			c.blocked++
		}
	}
	c.state = deriveVideoState(c)
}

// deriveVideoState is the whole video-level state machine, derived from the
// task census rather than tracked separately.
func deriveVideoState(c *counters) entity.VideoState {
	switch {
	case c.total > 0 && c.succeeded == c.total:
		return entity.VideoStateCompleted
	case c.running > 0 || c.ready > 0 || c.retryPending > 0:
		return entity.VideoStateRunning
	case c.awaitingApproval > 0:
		return entity.VideoStateAwaitingApproval
	case c.cancelled > 0:
		return entity.VideoStateCancelled
	case c.failed > 0:
		return entity.VideoStateFailed
	case c.blocked > 0:
		return entity.VideoStateBlocked
	default:
		return entity.VideoStateCompleted
	}
}

func (s *Scheduler) updateVideoState(ctx context.Context, videoID entity.VideoID) {
	c, ok := s.counts[videoID]
	if !ok {
		return
	}
	if s.lifecycle != nil {
		if err := s.lifecycle.SetVideoState(ctx, videoID, c.state, c.lastError); err != nil {
			s.log.Error("failed to persist video state",
				slog.String("video_id", videoID.String()),
				slog.String("state", string(c.state)),
				slog.String("error", err.Error()))
		}
	}
	if s.notifier != nil {
		s.notifier.NotifyVideo(entity.VideoDelta{
			ID:        videoID,
			State:     c.state,
			Done:      c.succeeded,
			Total:     c.total,
			Failed:    c.failed,
			Running:   c.running,
			Error:     c.lastError,
			UpdatedAt: time.Now(),
		})
	}
	// A finished video keeps no cancellation scope alive.
	if c.state.Terminal() && c.running == 0 {
		if run, ok := s.runs[videoID]; ok {
			run.cancel()
			delete(s.runs, videoID)
		}
	}
}

func (s *Scheduler) publishStatus() {
	if !s.dirty {
		return
	}
	s.dirty = false
	st := &Status{
		Pools:     make([]entity.PoolStat, 0, entity.NumPools),
		Videos:    len(s.graphs),
		StartedAt: s.started(),
	}
	for _, pool := range entity.AllPools {
		st.Pools = append(st.Pools, entity.PoolStat{
			Pool:     pool,
			Limit:    s.pools.Limit(pool),
			InFlight: s.pools.InFlight(pool),
			Queued:   s.ready.Len(pool),
		})
	}
	for _, c := range s.counts {
		st.Ready += c.ready
		st.Running += c.running
		st.Blocked += c.blocked
		st.AwaitingApproval += c.awaitingApproval
		st.Succeeded += c.succeeded
		st.Failed += c.failed
		st.RetryPending += c.retryPending
	}
	st.UptimeSeconds = time.Since(s.started()).Seconds()
	s.status.Store(st)
	if s.notifier != nil {
		s.notifier.NotifyScheduler(entity.SchedulerDelta{
			Pools:   st.Pools,
			Ready:   st.Ready,
			Running: st.Running,
			Blocked: st.Blocked,
			Videos:  st.Videos,
			Uptime:  st.UptimeSeconds,
		})
	}
}

// Snapshot returns the current console view without disturbing the loop.
//
// Task counters come from the snapshot the loop publishes; pool limits and
// occupancy are read live, because a limit change is applied asynchronously by
// the pool reconciler and would otherwise appear stale until the next event.
func (s *Scheduler) Snapshot() Status {
	st := s.status.Load()
	if st == nil {
		st = &Status{StartedAt: s.started()}
	}
	out := *st
	out.Pools = make([]entity.PoolStat, 0, entity.NumPools)
	for i, pool := range entity.AllPools {
		queued := 0
		if i < len(st.Pools) {
			queued = st.Pools[i].Queued
		}
		out.Pools = append(out.Pools, entity.PoolStat{
			Pool:     pool,
			Limit:    s.pools.Limit(pool),
			InFlight: s.pools.InFlight(pool),
			Queued:   queued,
		})
	}
	out.UptimeSeconds = time.Since(s.started()).Seconds()
	return out
}
