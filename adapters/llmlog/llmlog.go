// Package llmlog retains the live text of recent language-model exchanges and
// fans it out to whoever is watching.
//
// It exists because a blueprint can take minutes and says nothing at all while
// it does. The scheduler already reports that a task is running; this reports
// what it is producing, which is the only thing that distinguishes a model
// working from a model stuck.
//
// Three properties are the whole design:
//
//   - It retains. A browser that opens the console halfway through a
//     generation is served the same text as one that was watching from the
//     start, because the exchange so far is replayed as one frame in the shape
//     of a live one.
//
//   - It coalesces. Tokens arrive a few hundred times a second and no display
//     benefits from a frame per token, so writes accumulate and go out at most
//     once per window — the same bargain adapters/eventbus makes, for the same
//     reason.
//
//   - It bounds. A fifty-chapter render is hundreds of exchanges and megabytes
//     of text, so the oldest runs are evicted and a long one keeps its tail.
//     What a log costs must not depend on how long the machine has been up.
//
// Nothing here is on the path of any task. A dropped frame costs a console its
// text and costs the pipeline nothing, which is the trade a log should make.
package llmlog

import (
	"sync"
	"time"
	"unicode/utf8"

	"github.com/tbui/yt-studio/domain/provider"
)

const (
	// maxRuns is how many exchanges are retained. Well past what a console
	// shows, so scrolling back reaches something, and far short of what a long
	// render produces.
	maxRuns = 32

	// maxRunBytes bounds one retained exchange. A slide-prompt batch for fifty
	// chapters is the largest thing an LLM here returns and lands under this;
	// anything above it is a model that has started repeating, and the tail is
	// the half worth keeping.
	maxRunBytes = 64 << 10

	// subscriberBuffer is generous enough that a browser doing a little work
	// between reads never drops a frame at the coalesced rate.
	subscriberBuffer = 256

	// defaultWindow is the coalescing window. Ten frames a second reads as
	// continuous text and costs a hundredth of a frame per token.
	defaultWindow = 100 * time.Millisecond
)

// run is one exchange, as retained.
type run struct {
	frame provider.LLMFrame

	// text is everything kept, oldest bytes dropped once past maxRunBytes.
	text []byte
	// pending is what has arrived since the last fan-out.
	pending []byte
	// announced reports that subscribers have been told this run exists, so
	// the frame that opens one goes out without waiting for the window.
	announced bool
	lastFlush time.Time
}

type subscriber struct {
	id uint64
	ch chan provider.LLMFrame
}

// Broker retains recent exchanges and fans new text out to subscribers.
//
// One mutex covers both halves on purpose. Subscribing takes a snapshot and
// registers for what follows, and those two have to be one step: a frame that
// landed between them would be missing from the snapshot and never delivered,
// which in append-only text is not a stale reading but a hole. Serialising
// fan-out through the same lock also keeps a run's frames in order, which the
// receiver depends on and a state-based bus would not care about.
type Broker struct {
	window time.Duration

	mu sync.Mutex
	// runs holds every retained exchange by id, and order is their arrival
	// sequence — the order a console shows them in, and the order they are
	// evicted in.
	runs  map[uint64]*run
	order []uint64

	subs    map[uint64]*subscriber
	nextSub uint64
}

// New constructs a broker. A window of zero means defaultWindow.
func New(window time.Duration) *Broker {
	if window <= 0 {
		window = defaultWindow
	}
	return &Broker{
		window: window,
		runs:   make(map[uint64]*run, maxRuns),
		order:  make([]uint64, 0, maxRuns),
		subs:   make(map[uint64]*subscriber, 4),
	}
}

// Observe records a frame and fans out what is due. It satisfies
// provider.LLMObserver and is called from whichever goroutine is driving the
// exchange, so it takes the lock briefly and never blocks on a subscriber.
func (b *Broker) Observe(f provider.LLMFrame) {
	if f.Run == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	r := b.runFor(f)
	if f.Text != "" {
		r.append(f.Text)
	}
	if f.Done {
		r.frame.Done = true
		r.frame.Err = f.Err
		r.frame.Duration = time.Since(r.frame.StartedAt)
	}
	// The first frame and the last one are the two a console cannot wait for:
	// one is the only announcement that an exchange has begun, and the other is
	// the only report of how it ended. Everything between them is text, and
	// text is what the window is for.
	if r.announced && !f.Done && time.Since(r.lastFlush) < b.window {
		return
	}
	b.flushLocked(r)
}

// runFor returns the retained run a frame belongs to, starting one when the
// frame is the first of its exchange.
func (b *Broker) runFor(f provider.LLMFrame) *run {
	if r, ok := b.runs[f.Run]; ok {
		return r
	}
	started := f.StartedAt
	if started.IsZero() {
		started = time.Now()
	}
	r := &run{
		frame: provider.LLMFrame{
			Run:       f.Run,
			Video:     f.Video,
			Label:     f.Label,
			Model:     f.Model,
			StartedAt: started,
		},
	}
	b.runs[f.Run] = r
	b.order = append(b.order, f.Run)
	b.evictLocked()
	return r
}

// evictLocked drops the oldest runs once past maxRuns. A console holding an
// evicted run simply stops seeing it move, which is what an old log line does.
func (b *Broker) evictLocked() {
	for len(b.order) > maxRuns {
		delete(b.runs, b.order[0])
		b.order = b.order[1:]
	}
}

// flushLocked sends everything accumulated since the last fan-out.
func (b *Broker) flushLocked(r *run) {
	out := r.frame
	out.Text = string(r.pending)
	r.pending = r.pending[:0]
	r.announced = true
	r.lastFlush = time.Now()

	for _, s := range b.subs {
		select {
		case s.ch <- out:
		default:
			// A console that cannot keep up loses text rather than slowing a
			// generation down. It is a log.
		}
	}
}

// Subscribe registers a watcher and hands back everything retained so far.
//
// The backlog is ordinary frames, each carrying one run's whole accumulated
// text, so a client applies it with the same code it applies live frames with.
// The returned cancel must be called when the watcher goes away.
func (b *Broker) Subscribe() ([]provider.LLMFrame, <-chan provider.LLMFrame, func()) {
	b.mu.Lock()
	backlog := make([]provider.LLMFrame, 0, len(b.order))
	for _, id := range b.order {
		r, ok := b.runs[id]
		if !ok {
			continue
		}
		snapshot := r.frame
		snapshot.Text = string(r.text)
		backlog = append(backlog, snapshot)
	}

	b.nextSub++
	s := &subscriber{id: b.nextSub, ch: make(chan provider.LLMFrame, subscriberBuffer)}
	b.subs[s.id] = s
	b.mu.Unlock()

	return backlog, s.ch, func() {
		b.mu.Lock()
		delete(b.subs, s.id)
		b.mu.Unlock()
		close(s.ch)
	}
}

// append adds text to a run, keeping the tail once past maxRunBytes.
func (r *run) append(s string) {
	r.pending = append(r.pending, s...)
	r.text = append(r.text, s...)
	if len(r.text) <= maxRunBytes {
		return
	}
	// Cut on a rune boundary. The join is only ever seen at the top of a
	// truncated run, and half a character there would be a bug report about
	// encoding rather than about the length.
	drop := len(r.text) - maxRunBytes
	for drop < len(r.text) && !utf8.RuneStart(r.text[drop]) {
		drop++
	}
	r.text = append(r.text[:0], r.text[drop:]...)
	r.frame.Truncated = true
}
