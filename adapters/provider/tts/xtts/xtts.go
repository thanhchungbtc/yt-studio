// Package xtts is the narration backend for an AllTalk/XTTS server.
//
// A chapter is split on sentence boundaries into chunks of at least
// xtts.chunk.min_chars, each synthesised alone and rejoined with a short
// silence. The splitting is not an optimisation: XTTS degrades on long inputs.
// Every chunk costs two requests, because a generation answers with a URL
// rather than audio. The chunking, concatenation and tail cleanup are ported
// one for one from the Python this replaces, so the output sounds the same.
package xtts

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

	"github.com/tbui/yt-studio/adapters/provider/tts"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// ErrUnavailable reports a server that cannot serve until someone changes
// something — an unreachable host, a voice that does not exist. Wrapping the
// port's sentinel makes app.classify fail the task once rather than retrying.
var ErrUnavailable = fmt.Errorf("xtts: %w", provider.ErrUnavailable)

// The endpoints this package uses, relative to Config.BaseURL, listed here so
// the surface it needs from a server is one list rather than a search.
const (
	endpointGenerate = "/api/tts-generate"
	endpointReady    = "/api/ready"
)

// The defaults a half-configured client falls back to, so a missing row costs a
// chapter its tuning rather than its narration. They are the Python's values.
const (
	defaultLanguage           = "en"
	defaultSpeed              = 1.0
	defaultChunkMinChars      = 250
	defaultChunkSilenceMillis = 200
)

// The tail cleanup constants, carried over unchanged: trim trailing samples
// below the threshold, then fade what survives below audibility.
const (
	defaultFadeMillis       = 40
	defaultSilenceThreshold = 0.005
)

// defaultTimeout bounds one request, not one chapter. It is generous because a
// chunk on a CPU-only server takes minutes; the cost is that a hung call holds
// its pool slot, and cancelling the video is the way out.
const defaultTimeout = 20 * time.Minute

// Options are this backend's own knobs, read per call so an edit applies to the
// next chapter rather than the next restart. How a chapter should *sound* is
// not here — that arrives on the request, because it belongs to the video.
type Options struct {
	// ChunkMinChars floors a chunk's length, so it sets the size of the pieces a
	// chapter is spoken in rather than their number.
	ChunkMinChars int
	// ChunkSilenceMillis pads the joins so a sentence boundary is not a splice.
	ChunkSilenceMillis int
}

// voice is how one chapter should sound, once the request's blanks are filled.
// Its own type so the three fields travel together to the call that sends them.
type voice struct {
	name     string
	language string
	speed    float64
}

// voiceOf fills a request's blanks field by field. The name is the exception —
// empty is passed through, because the server picking its own default beats
// this end guessing a filename it cannot verify.
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
	// BaseURL resolves the server root, e.g. http://127.0.0.1:7851. Endpoints are
	// appended to it and a generation's audio URL is resolved against it. A
	// function, so the server can be moved on the settings screen without a
	// restart.
	BaseURL func() string
	// Timeout bounds one request; zero means defaultTimeout.
	Timeout time.Duration
	// Options resolves the settings-sourced knobs, per call.
	Options func() Options
}

// Client is the narration backend.
type Client struct {
	cfg   Config
	http  *http.Client
	store provider.AssetStore
}

var _ provider.TTS = (*Client)(nil)

// New wires the client, touching no network — Check reports a server that is
// down.
//
// The address is resolved per call rather than checked here: it is a settings
// row, so it is not known at wiring time and can change afterwards. A malformed
// one surfaces through Check and through the first chapter, both as
// ErrUnavailable.
func New(cfg Config, store provider.AssetStore) (*Client, error) {
	if cfg.BaseURL == nil {
		return nil, fmt.Errorf("%w: no base URL resolver was given", ErrUnavailable)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	return &Client{
		cfg:   cfg,
		http:  &http.Client{Timeout: cfg.Timeout},
		store: store,
	}, nil
}

// BaseURL returns the server root as currently configured, for log lines.
// Empty when the row is unset or unusable.
func (c *Client) BaseURL() string {
	base, err := c.baseURL()
	if err != nil {
		return ""
	}
	return base
}

// baseURL resolves and checks the server root. The checks live here rather than
// in New because the value is a settings row, correctable while the server runs
// — which is also why the endpoint-not-root mistake still has to be caught: the
// Python this replaces configured the full endpoint, so pasting that value
// across is a mistake worth naming rather than a convention to remember.
func (c *Client) baseURL() (string, error) {
	raw := strings.TrimRight(strings.TrimSpace(c.cfg.BaseURL()), "/")
	if raw == "" {
		return "", fmt.Errorf("%w: no server address is set", ErrUnavailable)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%w: base URL %q: %w", ErrUnavailable, raw, err)
	}
	if !parsed.IsAbs() || parsed.Host == "" {
		return "", fmt.Errorf("%w: base URL %q must be absolute, e.g. http://127.0.0.1:7851",
			ErrUnavailable, raw)
	}
	if parsed.Path != "" {
		return "", fmt.Errorf("%w: base URL %q must be the server root, without %q",
			ErrUnavailable, raw, parsed.Path)
	}
	return raw, nil
}

// Check probes the server, so an unreachable one is known at startup rather
// than at the first chapter. The result is not cached: a server that was down
// at boot may be up now.
func (c *Client) Check(ctx context.Context) error {
	base, err := c.baseURL()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+endpointReady, http.NoBody)
	if err != nil {
		return fmt.Errorf("xtts: build ready request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s is unreachable: %w", ErrUnavailable, base, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, tts.ReadyBodyLimit))
	if err != nil {
		return fmt.Errorf("xtts: read ready response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return tts.StatusError(ErrUnavailable, "xtts", resp.StatusCode, resp.Status, body)
	}
	// AllTalk answers with the bare word "Ready". Anything else means the
	// process is up but the model is not loaded.
	if !strings.EqualFold(strings.TrimSpace(string(body)), "Ready") {
		return fmt.Errorf("%w: %s answered %q rather than Ready",
			ErrUnavailable, base+endpointReady, tts.Snippet(string(body)))
	}
	return nil
}

// Speak narrates exactly one chapter and returns the audio's content address.
// The order of operations below is the Python's.
func (c *Client) Speak(ctx context.Context, req provider.SpeakRequest) (entity.AssetID, error) {
	opts := c.options()
	v := voiceOf(req)

	text := tts.Normalize(req.Text)

	text = tts.PrependChapterTitle(text, req.ChapterTitle, req.Ordinal == tts.IntroOrdinal)

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

	joined, err := tts.ConcatWavs(parts, opts.ChunkSilenceMillis)
	if err != nil {
		return "", err
	}
	audio := tts.CleanTail(joined, defaultFadeMillis, defaultSilenceThreshold)

	stored, err := c.store.Put(ctx, entity.AssetKindAudio, bytes.NewReader(audio))
	if err != nil {
		return "", fmt.Errorf("store narration: %w", err)
	}
	return stored.ID, nil
}

// generateResponse is the half of the reply this package reads; the server
// returns more.
type generateResponse struct {
	OutputFileURL string `json:"output_file_url"`
}

// synthesize speaks one chunk and returns its WAV bytes: generate, then fetch
// what the generation reported.
func (c *Client) synthesize(ctx context.Context, chunk string, v voice) ([]byte, error) {
	base, err := c.baseURL()
	if err != nil {
		return nil, err
	}
	form := url.Values{
		"text_input":          {chunk},
		"character_voice_gen": {v.name},
		"language":            {v.language},
		// No trailing zero, so 1.0 goes over as "1" — what the Python sent.
		"speed": {strconv.FormatFloat(v.speed, 'g', -1, 64)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+endpointGenerate, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("xtts: build generate request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xtts: generate: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, tts.ReplyBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("xtts: read generate response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, tts.StatusError(ErrUnavailable, "xtts", resp.StatusCode, resp.Status, body)
	}

	var decoded generateResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("xtts: generate returned %q, which is not the expected JSON: %w",
			tts.Snippet(string(body)), err)
	}
	if decoded.OutputFileURL == "" {
		// A 200 with no URL means the server did nothing, and will keep doing
		// nothing until something changes.
		return nil, fmt.Errorf("%w: generate returned no output_file_url: %s",
			ErrUnavailable, tts.Snippet(string(body)))
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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, tts.ReplyBodyLimit))
		return nil, tts.StatusError(ErrUnavailable, "xtts", resp.StatusCode, resp.Status, body)
	}
	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("xtts: read audio: %w", err)
	}
	if len(audio) == 0 {
		// Empty bytes would store, address and compose as silence — the one
		// failure nobody notices until the render.
		return nil, fmt.Errorf("%w: %s returned no audio", ErrUnavailable, audioURL)
	}
	return audio, nil
}

// audioURL resolves what a generation reported into something fetchable. Some
// builds answer with a root-relative path and others with an absolute URL, and
// parsing tells the two apart where concatenation would not.
func (c *Client) audioURL(reported string) (string, error) {
	parsed, err := url.Parse(reported)
	if err != nil {
		return "", fmt.Errorf("%w: output_file_url %q is not a URL: %w", ErrUnavailable, reported, err)
	}
	if parsed.IsAbs() {
		return parsed.String(), nil
	}
	root, err := c.baseURL()
	if err != nil {
		return "", err
	}
	base, err := url.Parse(root)
	if err != nil {
		return "", fmt.Errorf("xtts: base URL %q: %w", root, err)
	}
	return base.ResolveReference(parsed).String(), nil
}

// options reads the current settings, falling back per field so a missing row
// costs a chapter its tuning rather than its narration.
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
