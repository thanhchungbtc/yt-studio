package services

import (
	"context"

	"github.com/tbui/yt-studio/internal/domain/entity"
)

// Events the UI follows.
const (
	// PipelineEvent carries the scheduler's deltas and pool table.
	PipelineEvent = "pipeline:event"
	// PipelineResyncEvent says deltas were missed: refetch.
	PipelineResyncEvent = "pipeline:resync"
	// ConsoleEvent carries LLM console frames, while the console is open.
	ConsoleEvent = "console:frames"
)

// Resync is the payload of PipelineResyncEvent.
type Resync struct{}

// Emitter publishes an event to the UI.
type Emitter interface {
	Emit(name string, data any)
}

// PipelineSource is the event broker's subscription side.
type PipelineSource interface {
	Subscribe() (events <-chan *entity.Event, dropped func() uint64, cancel func())
}

// ForwardPipeline emits broker events to the UI; dropped events become a resync.
func ForwardPipeline(ctx context.Context, source PipelineSource, emit Emitter) {
	events, dropped, cancel := source.Subscribe()
	defer cancel()
	var missed uint64
	for {
		select {
		case <-ctx.Done():
			return
		case ev, open := <-events:
			if !open {
				return
			}
			if n := dropped(); n != missed {
				missed = n
				emit.Emit(PipelineResyncEvent, Resync{})
			}
			emit.Emit(PipelineEvent, *ev)
		}
	}
}
