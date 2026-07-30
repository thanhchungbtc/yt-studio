// Package ninerouter is the LLM backend that talks to a 9router gateway.
//
// 9router (github.com/decolua/9router) fronts many upstream providers behind
// one OpenAI-shaped REST surface, selected by a namespaced model id such as
// `ag/gemini-3-flash`. It is normally run locally and its auth can be turned
// off, so the key is optional here.
//
// Two things about that surface are not what the OpenAI shape implies, and both
// are load-bearing:
//
//   - Responses stream unless `stream` is explicitly false. Omitting the field
//     yields server-sent events rather than a JSON object, so this package
//     always sends it.
//   - `response_format` is accepted and silently ignored — both `json_object`
//     and a strict `json_schema` come back as prose. It fails open, with no
//     error to catch, so this package never sends it and puts the output
//     contract in the prompt instead.
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
	"time"

	"github.com/tbui/yt-studio/domain/provider"
)

// ErrUnavailable reports a gateway that cannot serve this request until someone
// changes something: a missing key, an expired upstream credential, a model id
// that does not exist. It wraps the port's sentinel, so a task fails once and
// says why rather than spending its retries on an answer that will not change.
var ErrUnavailable = fmt.Errorf("9router: %w", provider.ErrUnavailable)

// defaultTimeout bounds one request. A fifty-chapter outline from a reasoning
// model is a slow call; the per-video context cancels it sooner when an
// operator cancels the video.
const defaultTimeout = 3 * time.Minute

// Config is everything needed to reach a 9router instance.
type Config struct {
	// BaseURL is the gateway root, e.g. http://localhost:20128.
	BaseURL string
	// APIKey is sent as a bearer token when set. A local gateway may run with
	// auth disabled, in which case it is empty and no header is sent.
	APIKey string
	// Model is the namespaced upstream id, e.g. ag/gemini-3-flash.
	Model string
	// Timeout bounds one request; zero means defaultTimeout.
	Timeout time.Duration
}

// Client is the LLM backend. Every generation step funnels through chat, so the
// wire format, the auth header and the error taxonomy are written once.
type Client struct {
	cfg   Config
	http  *http.Client
	store provider.AssetStore
}

var _ provider.LLMProvider = (*Client)(nil)

// New validates the configuration and wires the client. It touches no network:
// wiring cannot fail because a gateway is down, and Check is what reports that.
func New(cfg Config, store provider.AssetStore) (*Client, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("%w: base url must not be empty", ErrUnavailable)
	}
	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%w: base url %q is not an absolute http url", ErrUnavailable, cfg.BaseURL)
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("%w: model must not be empty", ErrUnavailable)
	}
	if store == nil {
		return nil, fmt.Errorf("%w: asset store must not be nil", ErrUnavailable)
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	return &Client{
		cfg:   cfg,
		http:  &http.Client{Timeout: cfg.Timeout},
		store: store,
	}, nil
}

// Model returns the configured upstream id, for the startup log line.
func (c *Client) Model() string { return c.cfg.Model }

// Check probes the gateway, so an operator learns it is unreachable at startup
// rather than from the first chapter of a fifty-chapter video.
//
// The result is deliberately not cached: a gateway that was down when the
// daemon booted may be up now, and a remembered failure would keep saying
// otherwise.
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
// uses. They are deliberately not exhaustive: a field nothing reads is a field
// that can drift from the gateway without anyone noticing.

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	// Stream carries no omitempty on purpose. False is the value that matters,
	// and omitting it is exactly what makes the gateway stream.
	Stream bool `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// The two roles this package sends. There is no assistant turn: every call is a
// single instruction, and the DAG rather than a conversation carries the state.
const (
	roleSystem = "system"
	roleUser   = "user"
)

// chat sends one completion and returns the assistant's text.
//
// It is the single seam every generation step goes through, so a change in the
// gateway's shape is a change in one place.
func (c *Client) chat(ctx context.Context, system, user string) (string, error) {
	payload, err := json.Marshal(chatRequest{
		Model: c.cfg.Model,
		Messages: []chatMessage{
			{Role: roleSystem, Content: system},
			{Role: roleUser, Content: user},
		},
		Stream: false,
	})
	if err != nil {
		return "", fmt.Errorf("encode chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.BaseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.authorize(req)

	resp, err := c.http.Do(req)
	if err != nil {
		// A transport failure is the gateway being unreachable, which no number of
		// attempts fixes. A cancelled context arrives here too, and app.classify
		// recognises it as the cancelled video it is.
		return "", fmt.Errorf("%w: %s: %w", ErrUnavailable, c.cfg.BaseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read chat response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", statusError(resp.StatusCode, resp.Status, body)
	}

	var decoded chatResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("chat response is not JSON: %w (%s)", err, snippet(string(body)))
	}
	// An upstream failure arrives with a matching status today. This is the belt
	// to that braces, and costs one nil check.
	if decoded.Error != nil && decoded.Error.Message != "" {
		return "", fmt.Errorf("9router: %s", decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("9router returned no choices (%s)", snippet(string(body)))
	}
	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("9router returned an empty completion (finish_reason %q)",
			decoded.Choices[0].FinishReason)
	}
	return content, nil
}

func (c *Client) authorize(req *http.Request) {
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
}

// statusError turns a non-200 into an error of the right retry class.
//
// The distinction is whether another attempt could land differently. A rejected
// credential or an unknown model cannot, and three attempts would only take
// three times as long to say so; a rate limit or an upstream outage is exactly
// what backoff exists for.
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

// gatewayMessage digs the human-readable half out of an error body, falling
// back to the raw bytes when the body is not the shape we expect.
func gatewayMessage(body []byte) string {
	var decoded chatResponse
	if err := json.Unmarshal(body, &decoded); err == nil && decoded.Error != nil && decoded.Error.Message != "" {
		return decoded.Error.Message
	}
	return string(body)
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
