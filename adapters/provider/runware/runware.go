// Package runware is the image backend that talks to the Runware inference API.
//
// One call generates an image and a second downloads it, because Runware
// answers with a URL rather than bytes and a URL that expires is not an
// artifact. Slides and thumbnail icons are separate types over that one call:
// they differ only in geometry and asset kind, but the ports are selected
// independently, so the two can be pointed at different backends.
//
// PNG rather than JPEG, because entity.AssetKind pins the extension and MIME
// per kind and JPEG bytes served as image/png would be a lie nothing checks.
package runware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tbui/yt-studio/domain/provider"
)

// ErrUnavailable reports a request that cannot succeed until someone changes
// something — a missing key, an unselected model, a size the checkpoint will
// not draw. Wrapping the port's sentinel makes app.classify fail it once.
var ErrUnavailable = fmt.Errorf("runware: %w", provider.ErrUnavailable)

// Model is a checkpoint this backend is known to draw with: the AIR identifier
// the API wants, and the name a human uses for it.
type Model struct {
	AIR  string
	Name string
}

// Models is the shortlist offered on the settings screen, not a catalogue:
// Runware hosts thousands and a copy here would be stale on the day it shipped.
// The field still takes any AIR.
func Models() []Model {
	return []Model{
		{AIR: "runware:100@1", Name: "FLUX.1 Dev"},
		{AIR: "runware:101@1", Name: "FLUX.1 Schnell"},
	}
}

// defaultBaseURL is the public API; Config.BaseURL overrides it.
const defaultBaseURL = "https://api.runware.ai/v1"

// Request timings. Inference is the slow half — a diffusion model at 1344x768
// takes tens of seconds under load — where the download is a CDN fetch.
const (
	defaultInferenceTimeout = 120 * time.Second
	defaultDownloadTimeout  = 60 * time.Second
)

// outputFormat is fixed rather than configurable: see the package comment.
const outputFormat = "PNG"

// defaultNegativePrompt is what every generation is steered away from. A
// constant rather than a row because it is the other half of one style
// decision, whose positive half already lives in the prompts.
const defaultNegativePrompt = "human faces, readable text, logos, watermarks, UI elements, photorealistic, 3D render, CGI, " +
	"digital art, colorful, colors, bright, color tones, grey, gray, shading, shadows, gradients, " +
	"glow, lighting effects, daylight, outdoor, cheerful, stock photo, blurry, low quality, paint, " +
	"oil painting, watercolor, neon, chalk, textured background, noisy background, patterned background"

// defaultIconSize is the square edge when a caller asks for none, matching the
// sample backend so the two are interchangeable.
const defaultIconSize = 512

// Config is everything needed to reach the API.
type Config struct {
	// APIKey is the bearer token; there is no anonymous access, so an empty one
	// is a wiring error reported at startup rather than at the first task.
	APIKey string
	// Model resolves the checkpoint's AIR identifier, e.g. runware:101@1. A
	// function, so a model picked on the settings screen applies to the next
	// generation rather than the next restart.
	Model func() string
	// SlideSize is the geometry slides are generated at. Icons are square by the
	// port's definition and carry their own size on the request.
	SlideSize func() (width, height int)
	// BaseURL overrides the public endpoint. Empty means defaultBaseURL.
	BaseURL string
	// InferenceTimeout and DownloadTimeout bound the two halves of one
	// generation; zero means the defaults above.
	InferenceTimeout time.Duration
	DownloadTimeout  time.Duration
}

// Client is the transport every generation goes through, so the wire format,
// the auth header and the error taxonomy are written once.
type Client struct {
	cfg Config
	// Two clients rather than one: the timeouts differ by an order of magnitude
	// and a single client cannot carry both.
	inference *http.Client
	download  *http.Client
	store     provider.AssetStore
	log       *slog.Logger
}

// New validates the configuration and wires the client, touching no network.
func New(cfg Config, store provider.AssetStore, log *slog.Logger) (*Client, error) {
	if cfg.Model == nil {
		return nil, fmt.Errorf("%w: no model resolver was given", ErrUnavailable)
	}
	if cfg.SlideSize == nil {
		return nil, fmt.Errorf("%w: no slide size resolver was given", ErrUnavailable)
	}
	if store == nil {
		return nil, fmt.Errorf("%w: asset store must not be nil", ErrUnavailable)
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = defaultBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.InferenceTimeout <= 0 {
		cfg.InferenceTimeout = defaultInferenceTimeout
	}
	if cfg.DownloadTimeout <= 0 {
		cfg.DownloadTimeout = defaultDownloadTimeout
	}
	if log == nil {
		log = slog.Default()
	}
	return &Client{
		cfg:       cfg,
		inference: &http.Client{Timeout: cfg.InferenceTimeout},
		download:  &http.Client{Timeout: cfg.DownloadTimeout},
		store:     store,
		log:       log,
	}, nil
}

// Model returns the currently selected checkpoint, for the startup log line.
func (c *Client) Model() string { return strings.TrimSpace(c.cfg.Model()) }

// Check reports whether a generation could run at all. It makes no request —
// the cheapest probe still costs a generation — so it catches the failure that
// actually happens: the key was never set.
func (c *Client) Check() error {
	if strings.TrimSpace(c.cfg.APIKey) == "" {
		return fmt.Errorf("%w: no API key is set", ErrUnavailable)
	}
	if c.Model() == "" {
		return fmt.Errorf("%w: no model is selected", ErrUnavailable)
	}
	return nil
}

// The wire types below are the subset of imageInference this package uses. Not
// exhaustive: a field nothing reads can drift from the API unnoticed.

type inferenceTask struct {
	TaskType       string `json:"taskType"`
	TaskUUID       string `json:"taskUUID"`
	PositivePrompt string `json:"positivePrompt"`
	NegativePrompt string `json:"negativePrompt,omitempty"`
	Model          string `json:"model"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	NumberResults  int    `json:"numberResults"`
	OutputFormat   string `json:"outputFormat"`
}

type inferenceError struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	TaskUUID string `json:"taskUUID"`
}

type inferenceResponse struct {
	Data []struct {
		TaskUUID string `json:"taskUUID"`
		ImageURL string `json:"imageURL"`
	} `json:"data"`
	Errors []inferenceError `json:"errors"`
}

// generate runs one inference and returns the image bytes. The negative prompt
// is a parameter, so a caller that must not carry the house style passes none
// rather than this growing a flag.
func (c *Client) generate(ctx context.Context, prompt string, width, height int, negative string) ([]byte, error) {
	if strings.TrimSpace(c.cfg.APIKey) == "" {
		return nil, fmt.Errorf("%w: no API key is set", ErrUnavailable)
	}
	model := c.Model()
	if model == "" {
		return nil, fmt.Errorf("%w: no model is selected", ErrUnavailable)
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("%w: %dx%d is not a size", ErrUnavailable, width, height)
	}

	// The API's size grid is not duplicated here: it varies by checkpoint, and a
	// stale copy would refuse a size the model would have drawn. A rejected size
	// comes back as a 400 carrying the real reason.
	task := inferenceTask{
		TaskType:       "imageInference",
		TaskUUID:       uuid.NewString(),
		PositivePrompt: prompt,
		NegativePrompt: negative,
		Model:          model,
		Width:          width,
		Height:         height,
		NumberResults:  1,
		OutputFormat:   outputFormat,
	}
	c.log.Debug("runware inference",
		slog.String("model", model),
		slog.Int("width", width),
		slog.Int("height", height),
		slog.String("prompt", snippet(prompt)))

	started := time.Now()
	imageURL, err := c.infer(ctx, task)
	if err != nil {
		return nil, err
	}
	c.log.Debug("runware image ready", slog.Duration("took", time.Since(started)))

	image, err := c.fetch(ctx, imageURL)
	if err != nil {
		return nil, err
	}
	c.log.Debug("runware image downloaded",
		slog.Int("bytes", len(image)),
		slog.Duration("took", time.Since(started)))
	return image, nil
}

// infer posts one task and returns the URL of the image it produced.
func (c *Client) infer(ctx context.Context, task inferenceTask) (string, error) {
	// An array: the API takes a batch of tasks even when it is one.
	payload, err := json.Marshal([]inferenceTask{task})
	if err != nil {
		return "", fmt.Errorf("encode inference request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.inference.Do(req)
	if err != nil {
		// An unreachable API is not fixed by attempts. A cancelled context lands
		// here too, and app.classify recognises it for what it is.
		return "", fmt.Errorf("%w: %s: %w", ErrUnavailable, c.cfg.BaseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read inference response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", statusError(resp.StatusCode, resp.Status, body)
	}

	var decoded inferenceResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("runware response is not JSON: %w (%s)", err, snippet(string(body)))
	}
	// An upstream failure arrives with a matching status today; this is the
	// belt to that braces.
	if msg := firstError(decoded.Errors); msg != "" {
		return "", fmt.Errorf("runware: %s", msg)
	}
	if len(decoded.Data) == 0 || decoded.Data[0].ImageURL == "" {
		return "", fmt.Errorf("runware returned no image (%s)", snippet(string(body)))
	}
	return decoded.Data[0].ImageURL, nil
}

// fetch downloads the generated image. A failure is retryable even though the
// generation is already paid for: the URL is not persisted, so asking again is
// the only way back, and one wasted generation beats parking the video.
func (c *Client) fetch(ctx context.Context, imageURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("runware: image url %q: %w", imageURL, err)
	}
	resp, err := c.download.Do(req)
	if err != nil {
		return nil, fmt.Errorf("runware: download image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
		return nil, fmt.Errorf("runware: download image: %s: %s", resp.Status, snippet(string(body)))
	}
	image, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("runware: read image: %w", err)
	}
	if len(image) == 0 {
		return nil, fmt.Errorf("runware: downloaded an empty image from %s", imageURL)
	}
	return image, nil
}

// statusError turns a non-200 into an error of the right retry class: a
// rejected key or an unknown model cannot land differently, a rate limit can.
func statusError(code int, status string, body []byte) error {
	detail := snippet(apiMessage(body))
	switch {
	case code == http.StatusTooManyRequests, code >= 500:
		return fmt.Errorf("runware: %s: %s", status, detail)
	case code >= 400:
		return fmt.Errorf("%w: %s: %s", ErrUnavailable, status, detail)
	default:
		return fmt.Errorf("runware: unexpected %s: %s", status, detail)
	}
}

// apiMessage digs the readable half out of an error body, falling back to the
// raw bytes.
func apiMessage(body []byte) string {
	var decoded inferenceResponse
	if err := json.Unmarshal(body, &decoded); err == nil {
		if msg := firstError(decoded.Errors); msg != "" {
			return msg
		}
	}
	return string(body)
}

// firstError reports the first error the API returned that carries any text.
func firstError(errs []inferenceError) string {
	for _, e := range errs {
		switch {
		case e.Message != "":
			return e.Message
		case e.Code != "":
			return e.Code
		}
	}
	return ""
}

// snippetLimit is enough of a response to recognise what came back, without
// turning a log line into a transcript.
const snippetLimit = 240

// snippet flattens and truncates text for an error message.
func snippet(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= snippetLimit {
		return s
	}
	return s[:snippetLimit] + "…"
}
