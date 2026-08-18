// Package kokoro is the narration backend for a Kokoro-FastAPI server.
//
// It is the short one. Kokoro speaks a whole chapter in a single request and
// answers with the audio itself, so there is no chunking, no rejoining and no
// second call to fetch what a generation reported — the three things that make
// the XTTS backend beside it four times this size. The chapter is not split
// because it does not need to be: the server does its own phoneme-level
// splitting, and 2,700 characters come back in nine seconds.
//
// The wire format is OpenAI's /v1/audio/speech, which is why the address is
// also OpenAI-shaped. Nothing here imports an OpenAI SDK for it: one endpoint
// that answers with bytes does not earn a dependency.
package kokoro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/tbui/yt-studio/adapters/provider/tts"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// ErrUnavailable reports a server that cannot serve until someone changes
// something — an unreachable host, a voice that does not exist. Wrapping the
// port's sentinel makes app.classify fail the task once rather than retrying.
var ErrUnavailable = fmt.Errorf("kokoro: %w", provider.ErrUnavailable)

// The endpoints this package uses, relative to Config.BaseURL. The address is
// configured as the server root rather than as an OpenAI base URL ending in
// /v1, so that a narration server is configured the same way the gateway in
// adapters/provider/ninerouter is.
const (
	endpointSpeech = "/v1/audio/speech"
	endpointVoices = "/v1/audio/voices"
	endpointHealth = "/health"
)

// The defaults a half-configured client falls back to, so a missing row costs a
// chapter its tuning rather than its narration.
const (
	defaultModel = "kokoro"
	defaultSpeed = 1.0
)

// audioFormat is the only format this backend asks for. It is not a setting:
// the chapter's WAV is trimmed and faded by tts.CleanTail, which reads a RIFF
// header, and the web UI names the download .wav. A compressed format would
// pass silently through both and be wrong in neither.
const audioFormat = "wav"

// The tail cleanup constants, shared with the XTTS backend because they
// describe a narrator's last breath rather than a particular model's.
const (
	defaultFadeMillis       = 40
	defaultSilenceThreshold = 0.005
)

// defaultTimeout bounds one request, which here is also one chapter. Generous
// against a cold model or a CPU-only host, but far below the twenty minutes the
// XTTS backend allows: this server runs at roughly nineteen times real time, so
// a chapter that has not answered in five minutes is a chapter that is stuck.
const defaultTimeout = 5 * time.Minute

// Config is everything needed to reach one Kokoro-FastAPI instance.
type Config struct {
	// BaseURL resolves the server root, e.g. http://127.0.0.1:8880. A function,
	// so the server can be moved on the settings screen without a restart.
	BaseURL func() string
	// APIKey resolves the bearer token, empty when the server wants none — which
	// is the usual case for one running locally.
	APIKey func() string
	// Model resolves which of the server's model ids to ask for; empty means
	// defaultModel.
	Model func() string
	// Timeout bounds one request; zero means defaultTimeout.
	Timeout time.Duration
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
// row, so it is not known at wiring time and can change afterwards.
func New(cfg Config, store provider.AssetStore) (*Client, error) {
	if cfg.BaseURL == nil {
		return nil, fmt.Errorf("%w: no base URL resolver was given", ErrUnavailable)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	return &Client{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}, store: store}, nil
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

// Model returns the model id as currently configured, for log lines.
func (c *Client) Model() string {
	if c.cfg.Model == nil {
		return defaultModel
	}
	if model := strings.TrimSpace(c.cfg.Model()); model != "" {
		return model
	}
	return defaultModel
}

// baseURL resolves and checks the server root. The checks live here rather than
// in New because the value is a settings row, correctable while the server
// runs. The path has to be rejected rather than tolerated: an OpenAI-shaped
// address is usually written with /v1 on the end, and pasting one here would
// otherwise be requested as /v1/v1/audio/speech.
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
		return "", fmt.Errorf("%w: base URL %q must be absolute, e.g. http://127.0.0.1:8880",
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
//
// It takes the voice rather than resolving one, because which voice a chapter
// is read in belongs to the video and not to this backend. Passing it is worth
// the parameter: the voice list is what the probe reads anyway, and a name that
// is not on it fails every chapter of every video until it is corrected.
func (c *Client) Check(ctx context.Context, voice string) error {
	voices, err := c.voices(ctx)
	if err != nil {
		return err
	}
	// A build without the listing is still a working server; it just cannot
	// answer the second question.
	if voices == nil {
		return nil
	}
	if voice = strings.TrimSpace(voice); voice == "" {
		// The server crashes on an empty voice rather than choosing one, so this
		// is not the "let it pick a default" that it is for the XTTS backend.
		return fmt.Errorf("%w: no voice is set; the server offers %s",
			ErrUnavailable, tts.Snippet(strings.Join(voices, ", ")))
	}
	if !slices.Contains(voices, voice) {
		return fmt.Errorf("%w: voice %q is not on the server; it offers %s",
			ErrUnavailable, voice, tts.Snippet(strings.Join(voices, ", ")))
	}
	return nil
}

// voicesResponse is the half of the reply this package reads.
type voicesResponse struct {
	Voices []string `json:"voices"`
}

// voices lists what the server can speak as, and reports reachability on the
// way. A nil list with a nil error means the server is up but does not offer
// the listing — older builds answer only /health.
func (c *Client) voices(ctx context.Context) ([]string, error) {
	base, err := c.baseURL()
	if err != nil {
		return nil, err
	}
	body, code, err := c.get(ctx, base+endpointVoices)
	if err != nil {
		return nil, err
	}
	if code == http.StatusNotFound {
		if _, healthCode, healthErr := c.get(ctx, base+endpointHealth); healthErr != nil {
			return nil, healthErr
		} else if healthCode != http.StatusOK {
			return nil, fmt.Errorf("%w: %s answered %d", ErrUnavailable, base+endpointHealth, healthCode)
		}
		return nil, nil
	}
	if code != http.StatusOK {
		return nil, tts.StatusError(ErrUnavailable, "kokoro", code, http.StatusText(code), body)
	}
	var decoded voicesResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("%w: %s returned %q, which is not the expected JSON: %w",
			ErrUnavailable, base+endpointVoices, tts.Snippet(string(body)), err)
	}
	if len(decoded.Voices) == 0 {
		return nil, fmt.Errorf("%w: %s lists no voices", ErrUnavailable, base+endpointVoices)
	}
	return decoded.Voices, nil
}

// get performs one probe request, returning the body and the status so the
// caller can tell a missing endpoint from a broken one.
func (c *Client) get(ctx context.Context, target string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		return nil, 0, fmt.Errorf("kokoro: build request for %s: %w", target, err)
	}
	c.authorize(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %s is unreachable: %w", ErrUnavailable, target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, tts.ReadyBodyLimit))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("kokoro: read response from %s: %w", target, err)
	}
	return body, resp.StatusCode, nil
}

// speechRequest is what /v1/audio/speech is asked for. No language field: which
// language Kokoro reads in is carried by the voice's prefix — af_ is American
// English, jf_ Japanese, zf_ Mandarin — so a language setting beside the voice
// would be a control that either agrees with it or breaks it.
type speechRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format"`
	Speed          float64 `json:"speed"`
}

// Speak narrates exactly one chapter and returns the audio's content address.
func (c *Client) Speak(ctx context.Context, req provider.SpeakRequest) (entity.AssetID, error) {
	base, err := c.baseURL()
	if err != nil {
		return "", err
	}
	voice := strings.TrimSpace(req.Voice)
	if voice == "" {
		// Refused here rather than sent: the server answers an empty voice with a
		// 500 and "string index out of range", which reads as a server fault and
		// would be retried as one.
		return "", fmt.Errorf("%w: no voice is set", ErrUnavailable)
	}
	speed := req.Speed
	if speed <= 0 {
		speed = defaultSpeed
	}

	text := tts.Normalize(req.Text)
	text = tts.PrependChapterTitle(text, req.ChapterTitle, req.Ordinal == tts.IntroOrdinal)

	audio, err := c.synthesize(ctx, base, speechRequest{
		Model:          c.Model(),
		Input:          text,
		Voice:          voice,
		ResponseFormat: audioFormat,
		Speed:          speed,
	})
	if err != nil {
		return "", err
	}
	// The server writes a streaming RIFF header, with 0xFFFFFFFF where both
	// lengths belong. CleanTail re-encodes what it decodes, so trimming the tail
	// is also what leaves a well-formed file behind for ffmpeg.
	audio = tts.CleanTail(audio, defaultFadeMillis, defaultSilenceThreshold)

	stored, err := c.store.Put(ctx, entity.AssetKindAudio, bytes.NewReader(audio))
	if err != nil {
		return "", fmt.Errorf("store narration: %w", err)
	}
	return stored.ID, nil
}

// synthesize speaks one chapter and returns its WAV bytes.
func (c *Client) synthesize(ctx context.Context, base string, body speechRequest) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("kokoro: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+endpointSpeech, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("kokoro: build speech request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.authorize(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kokoro: speech: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// Only the error body is bounded. The audio is not: a chapter is megabytes
		// and truncating it would store as a narration that stops mid-sentence.
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, tts.ReplyBodyLimit))
		return nil, tts.StatusError(ErrUnavailable, "kokoro", resp.StatusCode, resp.Status, detail)
	}
	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("kokoro: read audio: %w", err)
	}
	if len(audio) == 0 {
		// Empty bytes would store, address and compose as silence — the one
		// failure nobody notices until the render.
		return nil, fmt.Errorf("%w: %s returned no audio", ErrUnavailable, base+endpointSpeech)
	}
	return audio, nil
}

// authorize adds the bearer token when one is configured. A local server wants
// none, so an empty key sends no header rather than an empty one.
func (c *Client) authorize(req *http.Request) {
	if c.cfg.APIKey == nil {
		return
	}
	if key := strings.TrimSpace(c.cfg.APIKey()); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
}
