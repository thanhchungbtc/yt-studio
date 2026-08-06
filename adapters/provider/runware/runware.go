// Package runware is the image backend that talks to the Runware inference API.
//
// One REST call generates one image and a second downloads it: Runware answers
// an inference request with a URL rather than with bytes. Both halves are here
// because a task is not finished until the file is in the asset store, and a
// URL that expires is not an artifact.
//
// Two backends are built on the same call — slides (provider.SlideGenerator) and
// thumbnail icons (provider.IconGenerator) — because they differ only
// in geometry and in which asset kind the bytes land under. They stay separate
// types because the ports are selected independently, so the two can be pointed
// at different backends without this package changing shape.
//
// Images are requested as PNG rather than JPEG because entity.AssetKind pins the
// extension and the content type per kind, and an image is served as image/png.
// JPEG bytes stored under a .png path and served as image/png would be a lie
// that nothing downstream is looking for.
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
// something: a missing key, an unselected model, a rejected credential, a size
// the checkpoint will not draw. It wraps the port's sentinel, so app.classify
// fails the task once with the reason rather than spending its retries
// re-asking a question that has already been answered.
var ErrUnavailable = fmt.Errorf("runware: %w", provider.ErrUnavailable)

// defaultBaseURL is the public API. Config.BaseURL overrides it, which is what
// the tests point at an httptest server.
const defaultBaseURL = "https://api.runware.ai/v1"

// Request timings. Inference is the slow half — a diffusion model at 1344x768
// takes tens of seconds under load — and the download is a CDN fetch that
// should never take a minute.
const (
	defaultInferenceTimeout = 120 * time.Second
	defaultDownloadTimeout  = 60 * time.Second
)

// outputFormat is fixed rather than configurable: see the package comment.
const outputFormat = "PNG"

// defaultNegativePrompt is what every generation is steered away from.
//
// It is a constant rather than a settings row because it is the other half of
// one style decision rather than a knob: it describes the register the slides
// and the icons are drawn in, and the positive half of that decision already
// lives in the prompts. Promoting it to a row is a one-line change if the style
// ever needs to differ per channel.
const defaultNegativePrompt = "human faces, readable text, logos, watermarks, UI elements, photorealistic, 3D render, CGI, " +
	"digital art, colorful, colors, bright, color tones, grey, gray, shading, shadows, gradients, " +
	"glow, lighting effects, daylight, outdoor, cheerful, stock photo, blurry, low quality, paint, " +
	"oil painting, watercolor, neon, chalk, textured background, noisy background, patterned background"

// defaultIconSize is the square edge used when a caller asks for no particular
// size, matching the sample backend so the two are interchangeable.
const defaultIconSize = 512

// Config is everything needed to reach the API.
type Config struct {
	// APIKey is the bearer token. There is no anonymous access, so an empty key
	// is a wiring error the server reports at startup rather than at first task.
	APIKey string
	// Model resolves the AIR identifier of the checkpoint to run, e.g.
	// runware:101@1.
	//
	// It is a function rather than a string for the same reason the registry
	// resolves its backend per call: a model picked on the settings screen has to
	// apply to the next generation instead of the next restart.
	Model func() string
	// SlideSize is the geometry slides are generated at. Icons are square by the
	// port's definition and carry their own size on the request, so only the
	// slide half needs this.
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

// New validates the configuration and wires the client. It touches no network:
// wiring cannot fail because an API is down, and Check is what reports that.
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

// Check reports whether a generation could run at all.
//
// It deliberately makes no request: the cheapest call to this API still costs a
// generation, and paying for one on every boot to learn what reading the
// configuration already says is not a trade worth making. What it catches is
// the failure that actually happens — the key was never set.
func (c *Client) Check() error {
	if strings.TrimSpace(c.cfg.APIKey) == "" {
		return fmt.Errorf("%w: no API key is set", ErrUnavailable)
	}
	if c.Model() == "" {
		return fmt.Errorf("%w: no model is selected", ErrUnavailable)
	}
	return nil
}

// The wire types below are the subset of the imageInference surface this
// package uses. They are deliberately not exhaustive: a field nothing reads is
// a field that can drift from the API without anyone noticing.

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

// generate runs one inference and returns the image bytes.
//
// The negative prompt is a parameter rather than read from the constant inside,
// so a caller that must not carry the house style can pass none without this
// function growing a flag.
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

	// The API also constrains sizes to a grid and a range. Those rules are not
	// repeated here: they vary by checkpoint, and a stale copy of them would
	// refuse a size the model would have drawn. A rejected size comes back as a
	// 400 carrying the real reason, which is non-retryable and says more than a
	// guess made here would.
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
	// The body is an array: the API takes a batch of tasks even when it is one.
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
		// A transport failure is the API being unreachable, which no number of
		// attempts fixes quickly. A cancelled context arrives here too, and
		// app.classify recognises it as the cancelled video it is.
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
	// An upstream failure arrives with a matching status today. This is the belt
	// to that braces, and costs one loop over an empty slice.
	if msg := firstError(decoded.Errors); msg != "" {
		return "", fmt.Errorf("runware: %s", msg)
	}
	if len(decoded.Data) == 0 || decoded.Data[0].ImageURL == "" {
		return "", fmt.Errorf("runware returned no image (%s)", snippet(string(body)))
	}
	return decoded.Data[0].ImageURL, nil
}

// fetch downloads the generated image.
//
// A failure here is retryable even though the generation behind it is already
// paid for: the URL is not persisted, so the only way back to the image is to
// ask for it again. Losing one generation to a flaky download is the cheaper
// half of that trade against parking the whole video.
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

// statusError turns a non-200 into an error of the right retry class.
//
// The distinction is whether another attempt could land differently. A rejected
// key, an unknown model or a size the checkpoint will not draw cannot, and
// three attempts would only take three times as long to say so; a rate limit or
// an outage is exactly what backoff exists for.
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

// apiMessage digs the human-readable half out of an error body, falling back to
// the raw bytes when the body is not the shape we expect.
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
