// Package ninerouter is the LLM backend that talks to a 9router gateway.
//
// 9router (github.com/decolua/9router) fronts many upstream providers behind
// one OpenAI-shaped REST surface, selected by a namespaced model id such as
// `ag/gemini-3-flash`. It is normally run locally and its auth can be turned
// off, so the key is optional here.
//
// Two things about that surface are load-bearing:
//
//   - Responses stream unless `stream` is explicitly false, so it is always
//     sent.
//   - `response_format` is accepted and silently ignored, so it is never sent
//     and the output contract goes in the prompt instead.
package ninerouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// ErrUnavailable reports a gateway that cannot serve until someone changes
// something — a missing key, an expired credential, an unknown model id.
// Wrapping the port's sentinel makes the task fail once rather than retry.
var ErrUnavailable = fmt.Errorf("9router: %w", provider.ErrUnavailable)

// defaultTimeout bounds one request. It is generous because a fifty-chapter
// outline and a hundred-prompt batch are genuinely slow; the cost is that a
// hung call holds one of two LLM slots, and cancelling is the way out.
const defaultTimeout = 20 * time.Minute

// Config is everything needed to reach a 9router instance.
type Config struct {
	// BaseURL is the gateway root, e.g. http://localhost:20128.
	BaseURL string
	// APIKey is sent as a bearer token when set. A local gateway may run with
	// auth disabled, in which case it is empty and no header is sent.
	APIKey string
	// Model resolves the namespaced upstream id, e.g. ag/gemini-3-flash. A
	// function, so a model picked on the settings screen applies to the next
	// generation rather than the next restart.
	Model func() string
	// Timeout bounds one request; zero means defaultTimeout.
	Timeout time.Duration
	// TranscriptDir is where each exchange with the model is written for
	// reading afterwards. Empty switches it off.
	TranscriptDir string
}

// Client is the LLM backend. Every generation step funnels through chat, so the
// wire format, the auth header and the error taxonomy are written once.
type Client struct {
	cfg         Config
	http        *http.Client
	store       provider.AssetStore
	transcripts *transcriptWriter
	lookup      ContextLookup

	// Slide prompts are produced once per video and served to N callers.
	// Singleflight collapses the ones that overlap, the cache answers the rest.
	inflight singleflight.Group
	cacheMu  sync.RWMutex
	cache    map[entity.VideoID][]provider.SlidePrompt
}

var _ provider.LLM = (*Client)(nil)

// New validates the configuration and wires the client, touching no network.
//
// lookup resolves a video id into the plan its slides illustrate. Only
// SlidePrompts needs it, being handed an id and nothing else, so a nil lookup
// leaves that one method unavailable and the rest working.
func New(cfg Config, store provider.AssetStore, lookup ContextLookup) (*Client, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("%w: base url must not be empty", ErrUnavailable)
	}
	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%w: base url %q is not an absolute http url", ErrUnavailable, cfg.BaseURL)
	}
	if cfg.Model == nil {
		return nil, fmt.Errorf("%w: no model resolver was given", ErrUnavailable)
	}
	if store == nil {
		return nil, fmt.Errorf("%w: asset store must not be nil", ErrUnavailable)
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	// Eagerly, so an unusable path is a wiring error rather than a discovery
	// halfway through a fifty-chapter video.
	transcripts, err := newTranscriptWriter(cfg.TranscriptDir)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return &Client{
		cfg:         cfg,
		http:        &http.Client{Timeout: cfg.Timeout},
		store:       store,
		transcripts: transcripts,
		lookup:      lookup,
		cache:       make(map[entity.VideoID][]provider.SlidePrompt, 4),
	}, nil
}

// Model returns the currently selected upstream id, for the startup log line.
func (c *Client) Model() string { return c.cfg.Model() }

// Check probes the gateway, so an unreachable one is known at startup rather
// than at the first chapter. The result is not cached: a gateway that was down
// at boot may be up now.
func (c *Client) Check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.BaseURL+"/api/health", http.NoBody)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	c.authorize(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrUnavailable, c.cfg.BaseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s: health returned %s: %s",
			ErrUnavailable, c.cfg.BaseURL, resp.Status, snippet(string(body)))
	}
	var health struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(body, &health); err != nil || !health.OK {
		return fmt.Errorf("%w: %s: health is not ok: %s",
			ErrUnavailable, c.cfg.BaseURL, snippet(string(body)))
	}
	return nil
}

// The wire types below are the subset of the OpenAI chat surface this package
// uses. Not exhaustive: a field nothing reads can drift unnoticed.

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	// No omitempty: false is the value that matters, and omitting it is what
	// makes the gateway stream.
	Stream bool `json:"stream"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type chatResponse struct {
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage *chatUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// The two roles this package sends. There is no assistant turn: every call is
// one instruction, and the DAG rather than a conversation carries the state.
const (
	roleSystem = "system"
	roleUser   = "user"
)

// chat sends one completion and returns the assistant's text. Every generation
// step goes through it, so the gateway's shape is described in one place.
func (c *Client) chat(ctx context.Context, of call, system, user string) (string, error) {
	started := time.Now()
	record := transcript{call: of, Model: c.cfg.Model(), System: system, User: user, StartedAt: started}
	defer func() {
		record.Duration = time.Since(started)
		c.transcripts.write(record)
	}()

	text, usage, err := c.complete(ctx, system, user)
	record.Response, record.Usage, record.Err = text, usage, err
	return text, err
}

// complete is the request itself, split out so chat is only the recording.
func (c *Client) complete(ctx context.Context, system, user string) (string, *chatUsage, error) {
	model := strings.TrimSpace(c.cfg.Model())
	if model == "" {
		return "", nil, fmt.Errorf("%w: no model is selected", ErrUnavailable)
	}
	payload, err := json.Marshal(chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: roleSystem, Content: system},
			{Role: roleUser, Content: user},
		},
		Stream: false,
	})
	if err != nil {
		return "", nil, fmt.Errorf("encode chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.BaseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.authorize(req)

	resp, err := c.http.Do(req)
	if err != nil {
		// An unreachable gateway is not fixed by attempts. A cancelled context
		// lands here too, and app.classify recognises it for what it is.
		return "", nil, fmt.Errorf("%w: %s: %w", ErrUnavailable, c.cfg.BaseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("read chat response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil, statusError(resp.StatusCode, resp.Status, body)
	}

	var decoded chatResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", nil, fmt.Errorf("chat response is not JSON: %w (%s)", err, snippet(string(body)))
	}
	// An upstream failure arrives with a matching status today; this is the
	// belt to that braces.
	if decoded.Error != nil && decoded.Error.Message != "" {
		return "", decoded.Usage, fmt.Errorf("9router: %s", decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 {
		return "", decoded.Usage, fmt.Errorf("9router returned no choices (%s)", snippet(string(body)))
	}
	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if content == "" {
		return "", decoded.Usage, fmt.Errorf("9router returned an empty completion (finish_reason %q)",
			decoded.Choices[0].FinishReason)
	}
	return content, decoded.Usage, nil
}

func (c *Client) authorize(req *http.Request) {
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
}

// statusError turns a non-200 into an error of the right retry class: a
// rejected credential or unknown model cannot land differently, a rate limit
// or an upstream outage can.
func statusError(code int, status string, body []byte) error {
	detail := snippet(gatewayMessage(body))
	switch {
	case code == http.StatusTooManyRequests, code >= 500:
		return fmt.Errorf("9router: %s: %s", status, detail)
	case code >= 400:
		return fmt.Errorf("%w: %s: %s", ErrUnavailable, status, detail)
	default:
		return fmt.Errorf("9router: unexpected %s: %s", status, detail)
	}
}

// gatewayMessage digs the readable half out of an error body, falling back to
// the raw bytes.
func gatewayMessage(body []byte) string {
	var decoded chatResponse
	if err := json.Unmarshal(body, &decoded); err == nil && decoded.Error != nil && decoded.Error.Message != "" {
		return decoded.Error.Message
	}
	return string(body)
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
