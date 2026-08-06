package tts_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/tbui/yt-studio/adapters/assetstore"
	"github.com/tbui/yt-studio/adapters/provider/tts"
	"github.com/tbui/yt-studio/domain/provider"
)

// server is a stand-in AllTalk: it records what was asked of it and answers the
// two-request shape the real one uses.
type server struct {
	*httptest.Server

	mu     sync.Mutex
	forms  []map[string]string
	ready  string
	status int
	// body, when set, replaces the generate reply entirely.
	body string
	// audio is what the file endpoint serves.
	audio []byte
	// absoluteURL makes the reply carry a fully-qualified output_file_url, which
	// some builds do.
	absoluteURL bool
}

func newServer(t *testing.T) *server {
	t.Helper()
	s := &server{ready: "Ready", status: http.StatusOK, audio: sampleWAV()}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/ready", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.status != http.StatusOK {
			w.WriteHeader(s.status)
		}
		fmt.Fprint(w, s.ready)
	})
	mux.HandleFunc("/api/tts-generate", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		captured := map[string]string{}
		for k := range r.PostForm {
			captured[k] = r.PostForm.Get(k)
		}
		s.mu.Lock()
		s.forms = append(s.forms, captured)
		status, body, absolute := s.status, s.body, s.absoluteURL
		s.mu.Unlock()

		if status != http.StatusOK {
			w.WriteHeader(status)
			fmt.Fprint(w, "the server said no")
			return
		}
		if body != "" {
			fmt.Fprint(w, body)
			return
		}
		out := "/audio/out.wav"
		if absolute {
			out = s.URL + "/audio/out.wav"
		}
		fmt.Fprintf(w, `{"status":"generate-success","output_file_url":%q}`, out)
	})
	mux.HandleFunc("/audio/out.wav", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		audio := s.audio
		s.mu.Unlock()
		_, _ = w.Write(audio)
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func (s *server) submitted() []map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.forms
}

// sampleWAV is one short mono 16-bit WAV, enough to decode and concatenate.
func sampleWAV() []byte {
	const frames = 400
	header := []byte("RIFF")
	payload := make([]byte, frames*2)
	for i := range payload {
		payload[i] = 0x20
	}
	le32 := func(n int) []byte {
		return []byte{byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24)}
	}
	le16 := func(n int) []byte { return []byte{byte(n), byte(n >> 8)} }
	out := append([]byte{}, header...)
	out = append(out, le32(36+len(payload))...)
	out = append(out, []byte("WAVEfmt ")...)
	out = append(out, le32(16)...)
	out = append(out, le16(1)...)     // PCM
	out = append(out, le16(1)...)     // mono
	out = append(out, le32(22050)...) // sample rate
	out = append(out, le32(22050*2)...)
	out = append(out, le16(2)...)
	out = append(out, le16(16)...)
	out = append(out, []byte("data")...)
	out = append(out, le32(len(payload))...)
	return append(out, payload...)
}

func newClient(t *testing.T, s *server, opts tts.Options) (*tts.Client, provider.AssetStore) {
	t.Helper()
	store, err := assetstore.New(t.TempDir())
	if err != nil {
		t.Fatalf("assetstore: %v", err)
	}
	c, err := tts.New(tts.Config{
		BaseURL: s.URL,
		Options: func() tts.Options { return opts },
	}, store)
	if err != nil {
		t.Fatalf("tts.New: %v", err)
	}
	return c, store
}

func defaultOptions() tts.Options {
	return tts.Options{Voice: "female_01.wav", Language: "en", Speed: 1, ChunkMinChars: 250, ChunkSilenceMillis: 200}
}

func TestNewRejectsBadBaseURL(t *testing.T) {
	t.Parallel()
	store, err := assetstore.New(t.TempDir())
	if err != nil {
		t.Fatalf("assetstore: %v", err)
	}
	tests := []struct {
		name string
		url  string
	}{
		{"empty", ""},
		{"relative", "127.0.0.1:7851"},
		{"no host", "http://"},
		// The trap the Python's config walked into: its XTTS_URL was the full
		// endpoint, and pasting that value here would double the path.
		{"carries the endpoint path", "http://127.0.0.1:7851/api/tts-generate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := tts.New(tts.Config{BaseURL: tt.url}, store)
			if err == nil {
				t.Fatalf("New(%q) succeeded; a bad URL must fail at startup", tt.url)
			}
			if !errors.Is(err, provider.ErrUnavailable) {
				t.Errorf("New(%q) error = %v, want provider.ErrUnavailable", tt.url, err)
			}
		})
	}
}

func TestCheckReportsReady(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	c, _ := newClient(t, s, defaultOptions())
	if err := c.Check(context.Background()); err != nil {
		t.Errorf("Check: %v", err)
	}
}

func TestCheckRejectsNotReady(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	s.ready = "Model not loaded"
	c, _ := newClient(t, s, defaultOptions())
	err := c.Check(context.Background())
	if err == nil {
		t.Fatal("Check accepted a server that is not ready")
	}
	if !errors.Is(err, provider.ErrUnavailable) {
		t.Errorf("Check error = %v, want provider.ErrUnavailable", err)
	}
}

func TestSpeakSendsTheDocumentedForm(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	c, _ := newClient(t, s, defaultOptions())

	if _, err := c.Speak(context.Background(), provider.SpeakRequest{
		Ordinal: 2, ChapterTitle: "The Long Winter", Text: "Body text.",
	}); err != nil {
		t.Fatalf("Speak: %v", err)
	}

	forms := s.submitted()
	if len(forms) != 1 {
		t.Fatalf("submitted %d generations, want 1", len(forms))
	}
	got := forms[0]
	// These four field names are the contract three separate legacy clients
	// agreed on; renaming one silently produces a chapter of nothing.
	for field, want := range map[string]string{
		"character_voice_gen": "female_01.wav",
		"language":            "en",
		"speed":               "1",
	} {
		if got[field] != want {
			t.Errorf("form[%q] = %q, want %q", field, got[field], want)
		}
	}
	if !strings.Contains(got["text_input"], "Body text.") {
		t.Errorf("text_input = %q, want it to carry the script", got["text_input"])
	}
	// Ordinal 2 is not the intro, so the title is announced.
	if !strings.HasPrefix(got["text_input"], "The Long Winter.") {
		t.Errorf("text_input = %q, want the chapter title announced first", got["text_input"])
	}
}

func TestSpeakStoresTheAudio(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	c, store := newClient(t, s, defaultOptions())

	id, err := c.Speak(context.Background(), provider.SpeakRequest{Ordinal: 1, Text: "Body text."})
	if err != nil {
		t.Fatalf("Speak: %v", err)
	}
	if id == "" {
		t.Fatal("Speak returned an empty asset id")
	}
	if _, err := store.Stat(context.Background(), id, "audio"); err != nil {
		t.Errorf("the narration was not stored: %v", err)
	}
}

func TestSpeakChunksLongScripts(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	c, _ := newClient(t, s, tts.Options{Voice: "v", Language: "en", Speed: 1, ChunkMinChars: 40})

	var script strings.Builder
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&script, "This is sentence number %d of the chapter. ", i)
	}
	if _, err := c.Speak(context.Background(), provider.SpeakRequest{Ordinal: 1, Text: script.String()}); err != nil {
		t.Fatalf("Speak: %v", err)
	}
	if n := len(s.submitted()); n < 2 {
		t.Errorf("submitted %d generations; a long script must be chunked", n)
	}
}

func TestSpeakResolvesAnAbsoluteAudioURL(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	s.absoluteURL = true
	c, _ := newClient(t, s, defaultOptions())
	if _, err := c.Speak(context.Background(), provider.SpeakRequest{Ordinal: 1, Text: "Body."}); err != nil {
		t.Errorf("Speak with an absolute output_file_url: %v", err)
	}
}

func TestSpeakClassifiesFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		status      int
		body        string
		unavailable bool
	}{
		// A rejected voice or language cannot be fixed by trying again, so the
		// task fails once instead of spending its retries.
		{"400 is not retryable", http.StatusBadRequest, "", true},
		{"404 is not retryable", http.StatusNotFound, "", true},
		// A rate limit or a model still loading is exactly what backoff is for.
		{"429 is retryable", http.StatusTooManyRequests, "", false},
		{"500 is retryable", http.StatusInternalServerError, "", false},
		// A 200 that promises no audio is the server declining quietly.
		{"200 with no url is not retryable", http.StatusOK, `{"status":"generate-failure"}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newServer(t)
			s.status = tt.status
			s.body = tt.body
			c, _ := newClient(t, s, defaultOptions())

			_, err := c.Speak(context.Background(), provider.SpeakRequest{Ordinal: 1, Text: "Body."})
			if err == nil {
				t.Fatal("Speak succeeded; it should have failed")
			}
			if got := errors.Is(err, provider.ErrUnavailable); got != tt.unavailable {
				t.Errorf("errors.Is(err, ErrUnavailable) = %v, want %v (err: %v)", got, tt.unavailable, err)
			}
		})
	}
}

func TestSpeakRefusesEmptyAudio(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	s.audio = nil
	c, _ := newClient(t, s, defaultOptions())

	// Storing zero bytes would content-address and compose into the video as
	// silence, which nobody notices until the render.
	_, err := c.Speak(context.Background(), provider.SpeakRequest{Ordinal: 1, Text: "Body."})
	if err == nil {
		t.Fatal("Speak accepted an empty audio response")
	}
	if !errors.Is(err, provider.ErrUnavailable) {
		t.Errorf("error = %v, want provider.ErrUnavailable", err)
	}
}

func TestSpeakFallsBackWhenSettingsAreMissing(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	// A zero Options is what a client with no settings rows sees. It must still
	// speak: a missing row costs the tuning, not the narration.
	c, _ := newClient(t, s, tts.Options{})

	if _, err := c.Speak(context.Background(), provider.SpeakRequest{Ordinal: 1, Text: "Body."}); err != nil {
		t.Fatalf("Speak with zero options: %v", err)
	}
	got := s.submitted()[0]
	if got["language"] != "en" {
		t.Errorf("language = %q, want the default %q", got["language"], "en")
	}
	if got["speed"] != "1" {
		t.Errorf("speed = %q, want the default %q", got["speed"], "1")
	}
}
