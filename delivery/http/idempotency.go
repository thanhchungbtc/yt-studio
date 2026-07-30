package http

import (
	"bytes"
	"net/http"
	"sync"
	"time"
)

// idempotencyTTL bounds how long a replayed request key is remembered.
const idempotencyTTL = 10 * time.Minute

// idempotency makes mutations idempotent by request key.
//
// Most mutations in this API are already idempotent by construction — task
// ids are deterministic and a gate is a row update — but creating a video
// mints a ref from a counter, so a retried request must not produce a second
// video. Replaying a stored response is the only way to make that safe from the
// client's side.
func idempotency(ttl time.Duration) func(http.Handler) http.Handler {
	store := &idempotencyStore{
		entries: make(map[string]idempotencyEntry, 64),
		ttl:     ttl,
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("Idempotency-Key")
			if key == "" || (r.Method != http.MethodPost && r.Method != http.MethodPut) {
				next.ServeHTTP(w, r)
				return
			}
			scoped := r.Method + " " + r.URL.Path + " " + key
			if cached, ok := store.get(scoped); ok {
				for k, values := range cached.header {
					for _, v := range values {
						w.Header().Add(k, v)
					}
				}
				w.Header().Set("Idempotent-Replay", "true")
				w.WriteHeader(cached.status)
				_, _ = w.Write(cached.body)
				return
			}
			rec := &recordingWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			if rec.status < 500 {
				// The captured body is pre-compression, and the replay is written back
				// through the same middleware stack, so transfer headers from the original
				// response must not be replayed with it.
				header := w.Header().Clone()
				header.Del("Content-Encoding")
				header.Del("Content-Length")
				header.Del("Vary")
				store.put(scoped, idempotencyEntry{
					status: rec.status,
					header: header,
					body:   rec.body.Bytes(),
				})
			}
		})
	}
}

type idempotencyEntry struct {
	status   int
	header   http.Header
	body     []byte
	storedAt time.Time
}

type idempotencyStore struct {
	mu      sync.Mutex
	entries map[string]idempotencyEntry
	ttl     time.Duration
}

func (s *idempotencyStore) get(key string) (idempotencyEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	if !ok {
		return idempotencyEntry{}, false
	}
	if time.Since(e.storedAt) > s.ttl {
		delete(s.entries, key)
		return idempotencyEntry{}, false
	}
	return e, true
}

func (s *idempotencyStore) put(key string, e idempotencyEntry) {
	e.storedAt = time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.entries) > 1024 {
		for k, v := range s.entries {
			if time.Since(v.storedAt) > s.ttl {
				delete(s.entries, k)
			}
		}
	}
	s.entries[key] = e
}

// recordingWriter captures a response so it can be replayed for a repeated key.
type recordingWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (w *recordingWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	w.body.Write(p)
	return w.ResponseWriter.Write(p)
}
