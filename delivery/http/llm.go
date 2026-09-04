package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/tbui/yt-studio/domain/provider"
)

// LLMStreamSource is the narrow half of the LLM log this handler needs.
//
// Subscribe hands back the backlog and the live channel in one call rather than
// two, and that is not a convenience. Taken separately, a frame landing between
// them would be absent from the snapshot and never delivered — in append-only
// text that is not a stale reading but a hole in the middle of a sentence.
type LLMStreamSource interface {
	Subscribe() ([]provider.LLMFrame, <-chan provider.LLMFrame, func())
}

// llmFrame is the wire shape of one moment in an exchange.
//
// Text is what has arrived since the previous frame, so a client appends rather
// than replaces, and a backlog frame carrying the whole exchange so far appends
// into an empty run to the same effect. That is the point: one client-side code
// path whether the console was open when the generation started or not.
type llmFrame struct {
	Run     uint64 `json:"run"`
	VideoID string `json:"videoId"`
	Label   string `json:"label"`
	Model   string `json:"model"`
	Text    string `json:"text,omitempty"`
	Done    bool   `json:"done,omitempty"`
	// Error is the reason an exchange ended, present only alongside done. A
	// string rather than a problem document: this is a log, and what it is
	// logging has already been reported as a task failure through the API.
	Error string `json:"error,omitempty"`
	// Truncated says the text is a tail — output was dropped from the front to
	// bound what the server retains.
	Truncated bool      `json:"truncated,omitempty"`
	StartedAt time.Time `json:"startedAt"`
	// Millis is how long the exchange ran, present only once done.
	Millis int64 `json:"ms,omitempty"`
}

func toLLMFrame(f provider.LLMFrame) llmFrame {
	out := llmFrame{
		Run:       f.Run,
		VideoID:   f.Video.String(),
		Label:     f.Label,
		Model:     f.Model,
		Text:      f.Text,
		Done:      f.Done,
		Truncated: f.Truncated,
		StartedAt: f.StartedAt,
		Millis:    f.Duration.Milliseconds(),
	}
	if f.Err != nil {
		out.Error = f.Err.Error()
	}
	return out
}

// llmHandler streams what the language models are producing, right now.
//
// A second SSE endpoint rather than a kind on /events, and the split is the
// design. The event stream carries *state*: deltas are coalesced per video and
// merged last-wins, which is exactly right for a task that moved and exactly
// wrong for text, where merging two frames loses the first one's words. This
// carries an append-only log, so it gets its own connection with its own
// lifetime — opened when the console is shown and closed when it is hidden,
// costing nothing at all the rest of the time.
//
// There is no Last-Event-ID resume here for the same reason. The server retains
// the recent exchanges, so a reconnecting client is served by the ordinary
// backlog every client gets; an id to resume from would be a second answer to a
// question already answered.
func llmHandler(source LLMStreamSource, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if source == nil {
			http.Error(w, "no llm log is configured", http.StatusServiceUnavailable)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache, no-transform")
		h.Set("Connection", "keep-alive")
		// Disables proxy buffering where anything sits in front of the server.
		h.Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		backlog, frames, cancel := source.Subscribe()
		defer cancel()

		for _, f := range backlog {
			if err := writeLLMFrame(w, f); err != nil {
				return
			}
		}
		flusher.Flush()

		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case f, open := <-frames:
				if !open {
					return
				}
				if err := writeLLMFrame(w, f); err != nil {
					log.Debug("llm console went away", slog.String("error", err.Error()))
					return
				}
				// Drain before flushing, so a burst costs one flush, not N.
			drain:
				for range 64 {
					select {
					case more, stillOpen := <-frames:
						if !stillOpen {
							flusher.Flush()
							return
						}
						if err := writeLLMFrame(w, more); err != nil {
							return
						}
					default:
						break drain
					}
				}
				flusher.Flush()
			case <-ticker.C:
				writeRaw(w, ": heartbeat\n\n")
				flusher.Flush()
			}
		}
	}
}

// writeLLMFrame emits one frame. No id field: nothing resumes from one here.
func writeLLMFrame(w http.ResponseWriter, f provider.LLMFrame) error {
	payload, err := json.Marshal(toLLMFrame(f))
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte("event: llm\ndata: ")); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	_, err = w.Write([]byte("\n\n"))
	return err
}
