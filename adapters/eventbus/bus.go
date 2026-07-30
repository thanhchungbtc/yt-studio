// Package eventbus is the in-process fan-out behind the SSE stream.
//
// One stream per client multiplexes every video. Bursts are coalesced per video
// — at most one message per window — so a 50-chapter render does not emit
// hundreds of events per second, while a lone state change still goes out
// immediately and meets the 20 ms budget.
package eventbus

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
)

const (
	// subscriberBuffer is generous enough that a browser doing a little work
	// between reads never drops a message.
	subscriberBuffer = 512
	// historySize bounds the replay buffer a reconnecting client resumes from.
	historySize = 1024
)

// schedulerBucket is the pseudo-video key the scheduler console's own deltas
// are coalesced under.
const schedulerBucket entity.VideoID = ""

type bucket struct {
	tasks     map[entity.TaskID]entity.TaskDelta
	chapters  map[entity.ChapterID]entity.ChapterDelta
	video     *entity.VideoDelta
	scheduler *entity.SchedulerDelta
	lastFlush time.Time
	dirty     bool
}

type subscriber struct {
	id uint64
	ch chan *entity.Event
	// dropped counts messages a slow client missed; it is reported once the client
	// catches up so the UI can force a refetch instead of showing stale state
	// forever.
	dropped atomic.Uint64
}

// Broker fans events out to every connected client.
type Broker struct {
	log      *slog.Logger
	coalesce time.Duration

	mu      sync.Mutex
	buckets map[entity.VideoID]*bucket

	subMu   sync.RWMutex
	subs    map[uint64]*subscriber
	nextSub uint64

	seq     atomic.Uint64
	histMu  sync.RWMutex
	history []entity.Event
	histAt  int
	histLen int
}

// New constructs a broker. coalesce is the per-video burst window.
func New(coalesce time.Duration, log *slog.Logger) *Broker {
	if coalesce <= 0 {
		coalesce = 50 * time.Millisecond
	}
	return &Broker{
		log:      log,
		coalesce: coalesce,
		buckets:  make(map[entity.VideoID]*bucket, 16),
		subs:     make(map[uint64]*subscriber, 8),
		history:  make([]entity.Event, historySize),
	}
}

// SetCoalesce applies a settings change without a restart.
func (b *Broker) SetCoalesce(d time.Duration) {
	if d <= 0 {
		return
	}
	b.mu.Lock()
	b.coalesce = d
	b.mu.Unlock()
}

// NotifyTask records a task delta. It is called from the scheduler's dispatch
// goroutine and must never block.
func (b *Broker) NotifyTask(d entity.TaskDelta) {
	b.mu.Lock()
	bk := b.bucketFor(d.VideoID)
	bk.tasks[d.ID] = d
	bk.dirty = true
	ev := b.maybeFlushLocked(d.VideoID, bk)
	b.mu.Unlock()
	b.publish(ev)
}

// NotifyVideo records a video delta.
func (b *Broker) NotifyVideo(d entity.VideoDelta) {
	b.mu.Lock()
	bk := b.bucketFor(d.ID)
	copied := d
	bk.video = &copied
	bk.dirty = true
	ev := b.maybeFlushLocked(d.ID, bk)
	b.mu.Unlock()
	b.publish(ev)
}

// NotifyChapter records a chapter delta, emitted by use cases as artifacts
// land.
func (b *Broker) NotifyChapter(d entity.ChapterDelta) {
	b.mu.Lock()
	bk := b.bucketFor(d.VideoID)
	bk.chapters[d.ID] = d
	bk.dirty = true
	ev := b.maybeFlushLocked(d.VideoID, bk)
	b.mu.Unlock()
	b.publish(ev)
}

// NotifyScheduler records pool utilisation for the operator console.
func (b *Broker) NotifyScheduler(d entity.SchedulerDelta) {
	b.mu.Lock()
	bk := b.bucketFor(schedulerBucket)
	copied := d
	bk.scheduler = &copied
	bk.dirty = true
	ev := b.maybeFlushLocked(schedulerBucket, bk)
	b.mu.Unlock()
	b.publish(ev)
}

func (b *Broker) bucketFor(videoID entity.VideoID) *bucket {
	bk, ok := b.buckets[videoID]
	if !ok {
		bk = &bucket{
			tasks:    make(map[entity.TaskID]entity.TaskDelta, 32),
			chapters: make(map[entity.ChapterID]entity.ChapterDelta, 8),
		}
		b.buckets[videoID] = bk
	}
	return bk
}

// maybeFlushLocked emits immediately when the window has elapsed, so a single
// change is not delayed, and otherwise leaves the bucket for the ticker.
func (b *Broker) maybeFlushLocked(videoID entity.VideoID, bk *bucket) *entity.Event {
	if time.Since(bk.lastFlush) < b.coalesce {
		return nil
	}
	return b.drainLocked(videoID, bk)
}

func (b *Broker) drainLocked(videoID entity.VideoID, bk *bucket) *entity.Event {
	if !bk.dirty {
		return nil
	}
	ev := &entity.Event{
		ID:      b.seq.Add(1),
		Kind:    entity.EventKindBatch,
		VideoID: videoID,
		At:      time.Now().UTC(),
	}
	if videoID == schedulerBucket {
		ev.Kind = entity.EventKindScheduler
	}
	if len(bk.tasks) > 0 {
		ev.Tasks = make([]entity.TaskDelta, 0, len(bk.tasks))
		for _, d := range bk.tasks {
			ev.Tasks = append(ev.Tasks, d)
		}
		clear(bk.tasks)
	}
	if len(bk.chapters) > 0 {
		ev.Chapters = make([]entity.ChapterDelta, 0, len(bk.chapters))
		for _, d := range bk.chapters {
			ev.Chapters = append(ev.Chapters, d)
		}
		clear(bk.chapters)
	}
	ev.Video, bk.video = bk.video, nil
	ev.Scheduler, bk.scheduler = bk.scheduler, nil
	bk.dirty = false
	bk.lastFlush = time.Now()
	return ev
}

func (b *Broker) publish(ev *entity.Event) {
	if ev == nil {
		return
	}
	b.remember(*ev)

	b.subMu.RLock()
	for _, s := range b.subs {
		select {
		case s.ch <- ev:
		default:
			s.dropped.Add(1)
		}
	}
	b.subMu.RUnlock()
}

func (b *Broker) remember(ev entity.Event) {
	b.histMu.Lock()
	b.history[b.histAt] = ev
	b.histAt = (b.histAt + 1) % len(b.history)
	if b.histLen < len(b.history) {
		b.histLen++
	}
	b.histMu.Unlock()
}

// Since returns the buffered events after id, oldest first, for a client
// resuming from Last-Event-ID. The second result reports whether the requested
// id was still inside the buffer; a false means the client must refetch.
func (b *Broker) Since(id uint64) ([]entity.Event, bool) {
	b.histMu.RLock()
	defer b.histMu.RUnlock()
	if b.histLen == 0 {
		return nil, true
	}
	out := make([]entity.Event, 0, b.histLen)
	start := (b.histAt - b.histLen + len(b.history)) % len(b.history)
	oldest := b.history[start].ID
	for i := range b.histLen {
		ev := b.history[(start+i)%len(b.history)]
		if ev.ID > id {
			out = append(out, ev)
		}
	}
	return out, id == 0 || id >= oldest-1
}

// Subscribe registers a client. The returned cancel must be called when the
// connection ends.
func (b *Broker) Subscribe() (<-chan *entity.Event, func()) {
	b.subMu.Lock()
	b.nextSub++
	s := &subscriber{id: b.nextSub, ch: make(chan *entity.Event, subscriberBuffer)}
	b.subs[s.id] = s
	count := len(b.subs)
	b.subMu.Unlock()

	b.log.Debug("sse client subscribed", slog.Uint64("client", s.id), slog.Int("clients", count))
	return s.ch, func() {
		b.subMu.Lock()
		delete(b.subs, s.id)
		remaining := len(b.subs)
		b.subMu.Unlock()
		close(s.ch)
		b.log.Debug("sse client unsubscribed", slog.Uint64("client", s.id), slog.Int("clients", remaining))
	}
}

// Subscribers reports the number of connected clients.
func (b *Broker) Subscribers() int {
	b.subMu.RLock()
	defer b.subMu.RUnlock()
	return len(b.subs)
}

// Run owns the coalescing ticker and returns when ctx is done.
func (b *Broker) Run(ctx context.Context) error {
	b.mu.Lock()
	interval := b.coalesce / 2
	b.mu.Unlock()
	if interval <= 0 {
		interval = 25 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			b.flushAll()
			return nil
		case <-ticker.C:
			b.flushDue()
		}
	}
}

func (b *Broker) flushDue() {
	b.mu.Lock()
	var pending []*entity.Event
	for videoID, bk := range b.buckets {
		if !bk.dirty || time.Since(bk.lastFlush) < b.coalesce {
			continue
		}
		if ev := b.drainLocked(videoID, bk); ev != nil {
			pending = append(pending, ev)
		}
	}
	b.mu.Unlock()
	for _, ev := range pending {
		b.publish(ev)
	}
}

func (b *Broker) flushAll() {
	b.mu.Lock()
	var pending []*entity.Event
	for videoID, bk := range b.buckets {
		if ev := b.drainLocked(videoID, bk); ev != nil {
			pending = append(pending, ev)
		}
	}
	b.mu.Unlock()
	for _, ev := range pending {
		b.publish(ev)
	}
}
