package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
)

// EventSource is the narrow half of the broker this handler needs.
type EventSource interface {
	Subscribe() (<-chan *entity.Event, func())
	Since(id uint64) ([]entity.Event, bool)
}

// heartbeatInterval keeps intermediaries from closing an idle stream.
const heartbeatInterval = 20 * time.Second

// eventsHandler is the single multiplexed SSE stream.
//
// Task state flows one way — daemon to browser — so EventSource gives
// reconnection, event ids and resume-from-last-id for free over plain HTTP. A
// WebSocket would add a bidirectional protocol with no use here.
func eventsHandler(source EventSource, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache, no-transform")
		h.Set("Connection", "keep-alive")
		// Disables proxy buffering where anything sits in front of the daemon.
		h.Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		lastID := parseLastEventID(r)
		events, cancel := source.Subscribe()
		defer cancel()

		// Replay whatever the client missed before streaming live deltas, so a
		// reconnect never needs a full reload.
		replay, complete := source.Since(lastID)
		if !complete {
			// The client was away longer than the buffer: tell it to refetch.
			writeRaw(w, "event: resync\ndata: {}\n\n")
		}
		for i := range replay {
			if err := writeEvent(w, &replay[i]); err != nil {
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
			case ev, open := <-events:
				if !open {
					return
				}
				if err := writeEvent(w, ev); err != nil {
					log.Debug("sse client went away", slog.String("error", err.Error()))
					return
				}
				// Drain anything already queued before flushing, so a burst costs one flush
				// rather than one per message.
				drained := 0
				for drained < 64 {
					select {
					case more, stillOpen := <-events:
						if !stillOpen {
							flusher.Flush()
							return
						}
						if err := writeEvent(w, more); err != nil {
							return
						}
						drained++
					default:
						drained = 64
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

func writeEvent(w http.ResponseWriter, ev *entity.Event) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte("id: ")); err != nil {
		return err
	}
	if _, err := w.Write([]byte(strconv.FormatUint(ev.ID, 10))); err != nil {
		return err
	}
	if _, err := w.Write([]byte("\nevent: ")); err != nil {
		return err
	}
	if _, err := w.Write([]byte(ev.Kind)); err != nil {
		return err
	}
	if _, err := w.Write([]byte("\ndata: ")); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	_, err = w.Write([]byte("\n\n"))
	return err
}

func writeRaw(w http.ResponseWriter, s string) {
	_, _ = w.Write([]byte(s))
}

func parseLastEventID(r *http.Request) uint64 {
	raw := r.Header.Get("Last-Event-ID")
	if raw == "" {
		raw = r.URL.Query().Get("lastEventId")
	}
	if raw == "" {
		return 0
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0
	}
	return id
}
