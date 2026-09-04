package provider

import (
	"time"

	"github.com/tbui/yt-studio/domain/entity"
)

// LLMFrame is one moment in an exchange with a language model, for watching a
// generation happen rather than reading it afterwards.
//
// It is deliberately one flat struct rather than a begin/delta/end trio of
// types. A single shape is a single code path on both sides of the wire: an
// observer that has not seen Run before is looking at the start of an exchange,
// one that has appends Text to what it already holds, and Done ends it. A
// backlog frame carrying everything accumulated so far is then the same thing
// as a live one, which is what lets a client that connects halfway through be
// served by the code that serves a client that was there from the beginning.
//
// The fields divide by who fills them in. The producer — an LLM adapter —
// owns Run, Video, Label, Model, Text, Done and Err. Whoever collects the
// frames owns StartedAt, Duration and Truncated, because those are properties
// of the accumulated exchange rather than of the moment being reported.
type LLMFrame struct {
	// Run groups the frames of one exchange. Unique within a process run, and
	// never zero: a zero Run is how a producer says it has no observer.
	Run uint64
	// Video is the video the exchange belongs to, so a client can filter.
	Video entity.VideoID
	// Label names the generation, e.g. "blueprint" or "script-ch12". It is the
	// same label the transcript file is named after, so the live view and the
	// file on disk can be matched up by eye.
	Label string
	// Model is the upstream id the exchange was sent to, resolved once when it
	// began. On the frame rather than read from settings by the reader, so a
	// model changed mid-generation cannot relabel output it did not produce.
	Model string
	// Text is what has arrived since the previous frame of this run, or on a
	// backlog frame everything that has arrived so far. Empty on the frame that
	// opens an exchange and usually empty on the one that closes it.
	Text string
	// Done marks the last frame of a run. Exactly one frame per run carries it,
	// whether the exchange succeeded or failed.
	Done bool
	// Err is the reason the exchange ended, set only alongside Done.
	Err error

	// StartedAt is when the exchange began.
	StartedAt time.Time
	// Duration is how long it ran, set only once Done.
	Duration time.Duration
	// Truncated reports that output has been dropped from the front of this
	// run to bound what is retained. It says the text is a tail, not a whole.
	Truncated bool
}

// LLMObserver receives frames as an exchange produces them.
//
// It is called from the goroutine driving the request, so it must not block:
// what it is watching is a task that would otherwise be making progress, and an
// observer is never a reason for one to run slower. It must also tolerate being
// called concurrently, since more than one exchange runs at a time.
//
// A nil observer is the feature switched off, and every producer checks for one
// rather than calling through a no-op — an adapter with nobody watching should
// not pay for the frames it would have sent.
type LLMObserver func(LLMFrame)
