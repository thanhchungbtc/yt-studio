package services

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/tbui/yt-studio/internal/domain/entity"
)

type fakePipeline struct {
	events  chan *entity.Event
	dropped atomic.Uint64
}

func (f *fakePipeline) Subscribe() (<-chan *entity.Event, func() uint64, func()) {
	return f.events, f.dropped.Load, func() {}
}

func TestForwardPipelineResyncsAfterDrops(t *testing.T) {
	source := &fakePipeline{events: make(chan *entity.Event, 4)}
	emit := &syncEmitter{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ForwardPipeline(ctx, source, emit)

	source.events <- &entity.Event{ID: 1, Kind: entity.EventKindBatch}
	emit.waitFor(t, func(e recordedEvent) bool { return e.name == PipelineEvent })

	source.dropped.Store(3)
	source.events <- &entity.Event{ID: 5, Kind: entity.EventKindBatch}
	emit.waitFor(t, func(e recordedEvent) bool {
		ev, ok := e.data.(entity.Event)
		return ok && ev.ID == 5
	})

	var names []string
	for _, ev := range emit.all() {
		names = append(names, ev.name)
	}
	want := []string{PipelineEvent, PipelineResyncEvent, PipelineEvent}
	if len(names) != len(want) {
		t.Fatalf("events = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("events = %v, want %v", names, want)
		}
	}
}
