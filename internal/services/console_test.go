package services

import (
	"sync"
	"testing"
	"time"

	"github.com/tbui/yt-studio/internal/domain/provider"
)

// fakeConsole is an LLM log with a fixed backlog.
type fakeConsole struct {
	mu      sync.Mutex
	backlog []provider.LLMFrame
	subs    []chan provider.LLMFrame
}

func (f *fakeConsole) Subscribe() ([]provider.LLMFrame, <-chan provider.LLMFrame, func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := make(chan provider.LLMFrame, 16)
	f.subs = append(f.subs, ch)
	var once sync.Once
	return f.backlog, ch, func() { once.Do(func() { close(ch) }) }
}

func (f *fakeConsole) send(fr provider.LLMFrame) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subs[len(f.subs)-1] <- fr
}

func batches(emit *syncEmitter, session int) []ConsoleBatch {
	var out []ConsoleBatch
	for _, ev := range emit.all() {
		if b, ok := ev.data.(ConsoleBatch); ok && ev.name == ConsoleEvent && b.Session == session {
			out = append(out, b)
		}
	}
	return out
}

func TestConsoleSendsBacklogThenLiveFrames(t *testing.T) {
	source := &fakeConsole{backlog: []provider.LLMFrame{{Run: 1, Text: "so far"}}}
	emit := &syncEmitter{}
	s := NewConsoleService(source, emit)

	s.Open(10)
	first := emit.waitFor(t, func(e recordedEvent) bool { return e.name == ConsoleEvent }).data.(ConsoleBatch)
	if !first.Reset || len(first.Frames) != 1 || first.Frames[0].Text != "so far" {
		t.Fatalf("backlog = %+v", first)
	}
	source.send(provider.LLMFrame{Run: 1, Text: " and more"})
	emit.waitFor(t, func(e recordedEvent) bool {
		b, ok := e.data.(ConsoleBatch)
		return ok && !b.Reset && len(b.Frames) == 1 && b.Frames[0].Text == " and more"
	})
	s.Close(10)
}

func TestConsoleIgnoresLateCalls(t *testing.T) {
	source := &fakeConsole{}
	emit := &syncEmitter{}
	s := NewConsoleService(source, emit)

	// The UI opened 2 after closing 1, but the calls arrived out of order.
	s.Open(2)
	s.Close(1)
	s.Open(1)
	emit.waitFor(t, func(e recordedEvent) bool { return e.name == ConsoleEvent })
	time.Sleep(20 * time.Millisecond)
	if len(batches(emit, 1)) != 0 || len(batches(emit, 2)) != 1 {
		t.Fatalf("events = %+v", emit.all())
	}
	if len(source.subs) != 1 {
		t.Fatalf("subscriptions = %d, want 1", len(source.subs))
	}

	// Closed before it opened: stays closed.
	s.Close(3)
	s.Open(3)
	time.Sleep(20 * time.Millisecond)
	if len(batches(emit, 3)) != 0 {
		t.Fatal("a closed session opened")
	}
}
