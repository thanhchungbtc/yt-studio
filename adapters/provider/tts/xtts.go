// Package tts is the narration backend for an AllTalk/XTTS server.
//
// A chapter is spoken in pieces: the script is split on sentence boundaries
// into chunks of at least xtts.chunk.min_chars, each chunk is synthesised on its
// own, and the WAVs are joined with a short silence between them. The splitting
// is not an optimisation — XTTS degrades on long inputs, and a chapter is
// thousands of words.
//
// The server answers a generation with a URL rather than audio, so every chunk
// costs two requests: generate, then fetch what the generation reported. The
// text chunking, WAV concatenation and the tail trim and fade are ported one
// for one from the Python this replaces, because the output has to keep sounding
// the same.
package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// ErrUnavailable reports a server that cannot serve this request until someone
// changes something: an unreachable host, a voice that does not exist, a
// rejected language. It wraps the port's sentinel, so app.classify fails the
// task once and says why rather than spending its retries on an answer that
// will not change.
var ErrUnavailable = fmt.Errorf("xtts: %w", provider.ErrUnavailable)

// The two endpoints this package uses, relative to Config.BaseURL. They are
// named here rather than inline at their call sites, so the surface this
// package needs from a server is one list rather than a search.
const (
	endpointGenerate = "/api/tts-generate"
	endpointReady    = "/api/ready"
)

// The defaults a half-configured client falls back to, so a missing settings
// row costs a chapter its tuning rather than its narration. They are the values
// the Python this replaces shipped with.
const (
	defaultLanguage           = "en"
	defaultSpeed              = 1.0
	defaultChunkMinChars      = 250
	defaultChunkSilenceMillis = 200
)

// introOrdinal is the first chapter. Chapters are 1-based here (the Python this
// is ported from was 0-based), and the intro is the one chapter whose title is
// not announced — it has no topic to read out.
const introOrdinal = 1

// The tail cleanup constants, carried over unchanged: trim trailing samples
// below the threshold, then ramp the last fade to zero so whatever artefact
// survives the trim is attenuated below audibility.
const (
	defaultFadeMillis       = 40
	defaultSilenceThreshold = 0.005
)

// defaultTimeout bounds one HTTP request, not one chapter.
//
// It is generous because a chunk on a CPU-only server genuinely takes minutes,
// and cutting it off early costs the whole chapter rather than the chunk. The
// cost of the generosity is that a hung call holds its TTS pool slot for the
// duration; cancelling the video is the faster way out, since the per-video
// context aborts an in-flight call promptly.
const defaultTimeout = 20 * time.Minute

// Options are this backend's own knobs, read per call so an edit on the
// settings screen applies to the next chapter rather than the next restart —
// the same reason the registry resolves its backend per call.
//
// How a chapter should sound is not here: voice, language and speed arrive on
// the request, because they belong to the video being narrated rather than to
// the server narrating it. What is left is the chunking, which exists only
// because this server degrades on long inputs.
type Options struct {
	// ChunkMinChars is the floor on a chunk's length in characters. The chunk
	// count follows from it (len(text) / ChunkMinChars), so it sets the size of
	// the pieces rather than their number.
	ChunkMinChars int
	// ChunkSilenceMillis is the pause inserted between chunks when they are
	// rejoined, so a sentence boundary does not become a splice.
	ChunkSilenceMillis int
}

// voice is how one chapter should sound, after the request's blanks have been
// filled in. It is a type of its own so the request's three fields travel
// together down to the one call that sends them.
type voice struct {
	name     string
	language string
	speed    float64
}

// voiceOf fills a request's blanks, field by field rather than wholesale: a
// missing value should cost a chapter its tuning, not its narration.
//
// The name is the deliberate exception — empty is passed through, because the
// server picking its own default is better than this end guessing a filename it
// cannot verify.
func voiceOf(req provider.SpeakRequest) voice {
	v := voice{name: req.Voice, language: req.Language, speed: req.Speed}
	if v.language == "" {
		v.language = defaultLanguage
	}
	if v.speed <= 0 {
		v.speed = defaultSpeed
	}
	return v
}

// Config is everything needed to reach one AllTalk instance.
type Config struct {
	// BaseURL is the server root, e.g. http://127.0.0.1:7851. The endpoints are
	// appended here rather than configured, and the audio URL a generation
	// returns is resolved against it.
	BaseURL string
	// Timeout bounds one request; zero means defaultTimeout.
	Timeout time.Duration
	// Options resolves the settings-sourced knobs. It is a function for the
	// reason given on Options itself.
	Options func() Options
}

// Client is the narration backend.
type Client struct {
	cfg   Config
	http  *http.Client
	store provider.AssetStore
}

var _ provider.TTS = (*Client)(nil)

// New validates the configuration and wires the client. It touches no network:
// wiring cannot fail because a server is down, and Check is what reports that.
//
// A bad BaseURL fails here rather than at the first chapter of fifty, which is
// the whole value of checking it at startup.
func New(cfg Config, store provider.AssetStore) (*Client, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("%w: base URL must not be empty", ErrUnavailable)
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("%w: base URL %q: %w", ErrUnavailable, cfg.BaseURL, err)
	}
	if !parsed.IsAbs() || parsed.Host == "" {
		return nil, fmt.Errorf("%w: base URL %q must be absolute, e.g. http://127.0.0.1:7851",
			ErrUnavailable, cfg.BaseURL)
	}
	// The server root, not the generate endpoint. The Python this replaces
	// configured the full endpoint and re-derived the root to resolve the audio
	// URL against; taking the root means a path here is a mistake to catch, not a
	// convention to remember.
	if parsed.Path != "" {
		return nil, fmt.Errorf("%w: base URL %q must be the server root, without %q",
			ErrUnavailable, cfg.BaseURL, parsed.Path)
	}
	cfg.BaseURL = base
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	return &Client{
		cfg:   cfg,
		http:  &http.Client{Timeout: cfg.Timeout},
		store: store,
	}, nil
}

// Check probes the server so an operator learns it is unreachable at startup
// rather than from the first chapter of a fifty-chapter video.
//
// The result is deliberately not cached: a server that was down at boot may be
// up now, and a remembered failure would keep saying otherwise.
func (c *Client) Check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.BaseURL+endpointReady, http.NoBody)
	if err != nil {
		return fmt.Errorf("xtts: build ready request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s is unreachable: %w", ErrUnavailable, c.cfg.BaseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, readyBodyLimit))
	if err != nil {
		return fmt.Errorf("xtts: read ready response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return statusError(resp.StatusCode, resp.Status, body)
	}
	// AllTalk answers this endpoint with the bare word "Ready". Anything else
	// means the process is up but the model is not loaded, which is exactly the
	// state worth catching before fifty chapters queue behind it.
	if !strings.EqualFold(strings.TrimSpace(string(body)), "Ready") {
		return fmt.Errorf("%w: %s answered %q rather than Ready",
			ErrUnavailable, c.cfg.BaseURL+endpointReady, snippet(string(body)))
	}
	return nil
}

// Speak narrates exactly one chapter and returns the audio's content address.
//
// The body below is the order of operations, ported from the Python. Every step
// but synthesize is written; that one call is what stands between this and a
// working backend.
func (c *Client) Speak(ctx context.Context, req provider.SpeakRequest) (entity.AssetID, error) {
	opts := c.options()
	v := voiceOf(req)

	text := normalize(req.Text)

	text = prependChapterTitle(text, req.ChapterTitle, req.Ordinal == introOrdinal)

	chunks := chunkTextBySentence(text, opts.ChunkMinChars)
	if len(chunks) == 0 {
		chunks = []string{text}
	}

	parts := make([][]byte, 0, len(chunks))
	for _, chunk := range chunks {
		part, err := c.synthesize(ctx, chunk, v)
		if err != nil {
			return "", err
		}
		parts = append(parts, part)
	}

	joined, err := concatWavs(parts, opts.ChunkSilenceMillis)
	if err != nil {
		return "", err
	}
	audio := cleanTail(joined, defaultFadeMillis, defaultSilenceThreshold)

	stored, err := c.store.Put(ctx, entity.AssetKindAudio, bytes.NewReader(audio))
	if err != nil {
		return "", fmt.Errorf("store narration: %w", err)
	}
	return stored.ID, nil
}

// generateResponse is the half of the generate reply this package reads. The
// server returns more; the audio's location is the only part that matters here.
type generateResponse struct {
	OutputFileURL string `json:"output_file_url"`
}

// synthesize speaks one chunk and returns its WAV bytes.
//
// It is two requests, because the server answers with a URL rather than audio:
// generate, then fetch what the generation reported.
func (c *Client) synthesize(ctx context.Context, chunk string, v voice) ([]byte, error) {
	form := url.Values{
		"text_input":          {chunk},
		"character_voice_gen": {v.name},
		"language":            {v.language},
		// Formatted without a trailing zero so 1.0 goes over as "1", which is what
		// the Python sent and what the server's own examples use.
		"speed": {strconv.FormatFloat(v.speed, 'g', -1, 64)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.BaseURL+endpointGenerate, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("xtts: build generate request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xtts: generate: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, replyBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("xtts: read generate response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, statusError(resp.StatusCode, resp.Status, body)
	}

	var decoded generateResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("xtts: generate returned %q, which is not the expected JSON: %w",
			snippet(string(body)), err)
	}
	if decoded.OutputFileURL == "" {
		// A 200 with no URL is the server telling us it did nothing. Retrying is
		// pointless until whatever refused the request is changed.
		return nil, fmt.Errorf("%w: generate returned no output_file_url: %s",
			ErrUnavailable, snippet(string(body)))
	}

	audioURL, err := c.audioURL(decoded.OutputFileURL)
	if err != nil {
		return nil, err
	}
	return c.fetchAudio(ctx, audioURL)
}

// fetchAudio downloads one generated file.
func (c *Client) fetchAudio(ctx context.Context, audioURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, audioURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("xtts: build audio request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xtts: fetch audio: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, replyBodyLimit))
		return nil, statusError(resp.StatusCode, resp.Status, body)
	}
	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("xtts: read audio: %w", err)
	}
	if len(audio) == 0 {
		// Empty bytes would store, content-address and compose into a video as
		// silence, which is the one failure nobody notices until the render.
		return nil, fmt.Errorf("%w: %s returned no audio", ErrUnavailable, audioURL)
	}
	return audio, nil
}

// audioURL resolves what a generation reported into something fetchable.
//
// The builds this was written against answer with a server-root-relative path,
// but others return an absolute URL. Parsing tells the two apart; concatenation
// works until the day it silently does not.
func (c *Client) audioURL(reported string) (string, error) {
	parsed, err := url.Parse(reported)
	if err != nil {
		return "", fmt.Errorf("%w: output_file_url %q is not a URL: %w", ErrUnavailable, reported, err)
	}
	if parsed.IsAbs() {
		return parsed.String(), nil
	}
	base, err := url.Parse(c.cfg.BaseURL)
	if err != nil {
		return "", fmt.Errorf("xtts: base URL %q: %w", c.cfg.BaseURL, err)
	}
	return base.ResolveReference(parsed).String(), nil
}

// options reads the current settings, falling back per field rather than
// wholesale: a missing row should cost a chapter its tuning, not its narration.
func (c *Client) options() Options {
	opts := Options{}
	if c.cfg.Options != nil {
		opts = c.cfg.Options()
	}
	if opts.ChunkMinChars <= 0 {
		opts.ChunkMinChars = defaultChunkMinChars
	}
	if opts.ChunkSilenceMillis < 0 {
		opts.ChunkSilenceMillis = defaultChunkSilenceMillis
	}
	return opts
}

// statusError turns a non-200 into an error of the right retry class.
//
// The distinction is whether another attempt could land differently. A voice
// that does not exist or a rejected language cannot, and three attempts would
// only take three times as long to say so; a rate limit or a model still
// loading is exactly what backoff exists for.
func statusError(code int, status string, body []byte) error {
	detail := snippet(string(body))
	switch {
	case code == http.StatusTooManyRequests, code >= 500:
		return fmt.Errorf("xtts: %s: %s", status, detail)
	case code >= 400:
		return fmt.Errorf("%w: %s: %s", ErrUnavailable, status, detail)
	default:
		return fmt.Errorf("xtts: unexpected %s: %s", status, detail)
	}
}

// The response-body ceilings. An error body is read to describe the failure,
// not to be kept, and the ready probe answers with a single word.
const (
	replyBodyLimit = 64 << 10
	readyBodyLimit = 1 << 10
)

// snippetLimit is how much of a response an error carries: enough to recognise
// what came back, not so much that a log line becomes a transcript.
const snippetLimit = 240

// snippet flattens and truncates text for an error message.
func snippet(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= snippetLimit {
		return s
	}
	return s[:snippetLimit] + "…"
}

// normalize is the only tidying done to a script before it is spoken: leading
// and trailing whitespace, nothing else. What the model was told to write is
// what the narrator reads.
func normalize(text string) string {
	return strings.TrimSpace(text)
}
