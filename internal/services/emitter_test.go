package services

import (
	"sync"
	"testing"
	"time"
)

type recordedEvent struct {
	name string
	data any
}

// syncEmitter records what's emitted.
type syncEmitter struct {
	mu     sync.Mutex
	events []recordedEvent
}

func (e *syncEmitter) Emit(name string, data any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, recordedEvent{name, data})
}

func (e *syncEmitter) all() []recordedEvent {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]recordedEvent(nil), e.events...)
}

// waitFor waits up to 5s for an event match accepts.
func (e *syncEmitter) waitFor(t *testing.T, match func(recordedEvent) bool) recordedEvent {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, ev := range e.all() {
			if match(ev) {
				return ev
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no matching event in %v", e.all())
	return recordedEvent{}
}
