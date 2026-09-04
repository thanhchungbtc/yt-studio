// Package ninerouter is the LLM backend that talks to a 9router gateway.
//
// 9router (github.com/decolua/9router) fronts many upstream providers behind
// one OpenAI-shaped REST surface, selected by a namespaced model id such as
// `ag/gemini-3-flash`. It is normally run locally and its auth can be turned
// off, so the key is optional here.
//
// Two things about that surface are load-bearing:
//
//   - Streaming is its native mode — `stream: false` is what suppresses it —
//     so every completion is streamed and assembled here. The assembled text is
//     byte for byte what a buffered response would have returned, and one
//     request shape is one code path to keep right. It is also what lets an
//     exchange be watched while it runs, which is the only thing that
//     distinguishes a model working from a model stuck.
//   - `response_format` is accepted and silently ignored, so it is never sent
//     and the output contract goes in the prompt instead.
package ninerouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
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

const (
	// streamDone is the sentinel the gateway ends a completion with, in place
	// of JSON.
	streamDone = "[DONE]"

	// streamBufferBytes is the scanner's starting line buffer. One frame is a
	// token or two of JSON, so this is already generous.
	streamBufferBytes = 8 << 10

	// maxStreamLineBytes bounds one frame. Far past any legitimate one, and the
	// difference between a malformed stream reported as an error and a malformed
	// stream read until the process runs out of memory.
	maxStreamLineBytes = 1 << 20

	// maxErrorBody bounds how much of a rejection is read back for the message.
	// It is quoted into an error, and an error is a sentence.
	maxErrorBody = 1 << 13
)

// Config is everything needed to reach a 9router instance.
type Config struct {
	// BaseURL resolves the gateway root, e.g. http://localhost:20128. A function
	// for the same reason Model is one: the gateway can be moved on the settings
	// screen, and a malformed one is reported by Check rather than refusing to
	// start a server whose other backends are fine.
	BaseURL func() string
	// APIKey resolves the bearer token, sent only when non-empty. A local gateway
	// may run with auth disabled, which is the usual case.
	APIKey func() string
	// Model resolves the namespaced upstream id, e.g. ag/gemini-3-flash. A
	// function, so a model picked on the settings screen applies to the next
	// generation rather than the next restart.
	Model func() string
	// Timeout bounds one request; zero means defaultTimeout.
	Timeout time.Duration
	// TranscriptDir is where each exchange with the model is written for
	// reading afterwards. Empty switches it off.
	TranscriptDir string
	// Observe watches exchanges as they are produced, for a live console. It is
	// the counterpart to TranscriptDir: the same material, while it happens
	// rather than once it is over. Nil is the feature off, and it costs one nil
	// check per exchange — never per token.
	Observe provider.LLMObserver
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

// New wires the client, touching no network.
//
// The gateway address is resolved per call rather than checked here: it is a
// settings row, so it is not known at wiring time and can change afterwards.
// A missing or malformed one surfaces through Check and through the first
// request, both as ErrUnavailable.
//
// lookup resolves a video id into the plan its slides illustrate. Only
// SlidePrompts needs it, being handed an id and nothing else, so a nil lookup
// leaves that one method unavailable and the rest working.
func New(cfg Config, store provider.AssetStore, lookup ContextLookup) (*Client, error) {
	if cfg.BaseURL == nil {
		return nil, fmt.Errorf("%w: no base url resolver was given", ErrUnavailable)
	}
	if cfg.APIKey == nil {
		return nil, fmt.Errorf("%w: no API key resolver was given", ErrUnavailable)
	}
	if cfg.Model == nil {
		return nil, fmt.Errorf("%w: no model resolver was given", ErrUnavailable)
	}
	if store == nil {
		return nil, fmt.Errorf("%w: asset store must not be nil", ErrUnavailable)
	}
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

// llmRunSeq numbers exchanges for the observer. Process-local, monotonic, and
// never persisted: it groups one run's frames for as long as a console is
// looking at them and means nothing afterwards.
var llmRunSeq atomic.Uint64

// watcher reports one exchange to the configured observer.
//
// A nil watcher is the no-observer case, and every method tolerates one, so the
// request path never asks whether anybody is watching. That is the whole reason
// it is a type rather than a callback threaded through by hand.
type watcher struct {
	observe provider.LLMObserver
	// frame is the run's identity, copied and completed for each report.
	frame provider.LLMFrame
}

// watch opens an exchange for the observer, and returns nil when there is none.
func (c *Client) watch(of call, model string, started time.Time) *watcher {
	if c.cfg.Observe == nil {
		return nil
	}
	w := &watcher{
		observe: c.cfg.Observe,
		frame: provider.LLMFrame{
			Run:       llmRunSeq.Add(1),
			Video:     of.Video,
			Label:     of.Label,
			Model:     model,
			StartedAt: started,
		},
	}
	// Announced before the request goes out rather than at the first token, so
	// a console shows the exchange for the whole of the wait — which on a slow
	// upstream is most of it, and is exactly the part worth reporting.
	w.observe(w.frame)
	return w
}

// delta reports text that has just arrived.
func (w *watcher) delta(text string) {
	if w == nil || text == "" {
		return
	}
	f := w.frame
	f.Text = text
	w.observe(f)
}

// close reports the exchange as over, whether or not it succeeded. Exactly one
// of these follows every watch.
func (w *watcher) close(err error) {
	if w == nil {
		return
	}
	f := w.frame
	f.Done = true
	f.Err = err
	w.observe(f)
}

// Model returns the currently selected upstream id, for the startup log line.
func (c *Client) Model() string { return c.cfg.Model() }

// BaseURL returns the gateway root as currently configured, for log lines and
// error messages. Empty when the row is unset or unusable.
func (c *Client) BaseURL() string {
	base, err := c.baseURL()
	if err != nil {
		return ""
	}
	return base
}

// baseURL resolves and checks the gateway root. The check lives here rather
// than in New because the value is a settings row: it is empty at wiring time
// and may be corrected while the server runs.
func (c *Client) baseURL() (string, error) {
	raw := strings.TrimSpace(c.cfg.BaseURL())
	if raw == "" {
		return "", fmt.Errorf("%w: no gateway address is set", ErrUnavailable)
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%w: %q is not an absolute http url", ErrUnavailable, raw)
	}
	return strings.TrimRight(raw, "/"), nil
}

// Check probes the gateway, so an unreachable one is known at startup rather
// than at the first chapter. The result is not cached: a gateway that was down
// at boot may be up now.
func (c *Client) Check(ctx context.Context) error {
	base, err := c.baseURL()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/health", http.NoBody)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	c.authorize(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrUnavailable, base, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s: health returned %s: %s",
			ErrUnavailable, base, resp.Status, snippet(string(body)))
	}
	var health struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(body, &health); err != nil || !health.OK {
		return fmt.Errorf("%w: %s: health is not ok: %s",
			ErrUnavailable, base, snippet(string(body)))
	}
	return nil
}

// The wire types below are the subset of the OpenAI chat surface this package
// uses. Not exhaustive: a field nothing reads can drift unnoticed.

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// streamOptions asks for the token counts a streamed exchange would otherwise
// never carry. Without it usage arrives only in a buffered response, and the
// transcript would quietly lose the one number that says what a generation
// cost.
type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	// Always true, and no omitempty: streaming is the gateway's native mode and
	// `stream: false` is what suppresses it. One request shape is one code path
	// to keep correct, and the text assembled from the frames is byte for byte
	// what a buffered response would have returned — the only difference is
	// that it can be watched while it arrives.
	Stream        bool           `json:"stream"`
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
}

// chatChunk is one frame of a streamed completion. The content arrives in
// `delta` rather than `message`, and the last frames carry no choice at all —
// one for the finish reason, one for usage — which is why every field here is
// checked for presence rather than assumed.
type chatChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *chatUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
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
//
// It is also where an exchange is bracketed for the two things that record it:
// the transcript written on the way out, and the observer told as it happens.
// Both are best-effort and neither can fail the call.
func (c *Client) chat(ctx context.Context, of call, system, user string) (string, error) {
	started := time.Now()
	// Resolved once, here, for the whole exchange. The request, the transcript
	// and the console must all name the same model, and a settings edit landing
	// mid-generation would otherwise label output a model did not produce.
	model := strings.TrimSpace(c.cfg.Model())
	record := transcript{call: of, Model: model, System: system, User: user, StartedAt: started}
	defer func() {
		record.Duration = time.Since(started)
		c.transcripts.write(record)
	}()

	watch := c.watch(of, model, started)
	text, usage, err := c.complete(ctx, watch, model, system, user)
	watch.close(err)

	// Partial text on a failure is deliberate: a truncated answer is the most
	// useful thing a failed exchange leaves behind, and the transcript exists to
	// be read afterwards. Callers check the error, never the string.
	record.Response, record.Usage, record.Err = text, usage, err
	return text, err
}

// complete is the request itself, split out so chat is only the recording.
func (c *Client) complete(
	ctx context.Context, watch *watcher, model, system, user string,
) (string, *chatUsage, error) {
	if model == "" {
		return "", nil, fmt.Errorf("%w: no model is selected", ErrUnavailable)
	}
	base, err := c.baseURL()
	if err != nil {
		return "", nil, err
	}
	payload, err := json.Marshal(chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: roleSystem, Content: system},
			{Role: roleUser, Content: user},
		},
		Stream:        true,
		StreamOptions: &streamOptions{IncludeUsage: true},
	})
	if err != nil {
		return "", nil, fmt.Errorf("encode chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	c.authorize(req)

	resp, err := c.http.Do(req)
	if err != nil {
		// An unreachable gateway is not fixed by attempts. A cancelled context
		// lands here too, and app.classify recognises it for what it is.
		return "", nil, fmt.Errorf("%w: %s: %w", ErrUnavailable, base, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		return "", nil, statusError(resp.StatusCode, resp.Status, body)
	}
	// A gateway is entitled to answer a streaming request with a buffered body,
	// and an upstream that cannot stream is a reason to be slower rather than a
	// reason to fail. The content type is what says which arrived.
	if !isEventStream(resp.Header.Get("Content-Type")) {
		return buffered(resp.Body, watch)
	}
	return streamed(resp.Body, watch)
}

// streamed assembles a completion from its frames, reporting each one onward as
// it lands. The string it returns is what the same exchange would have returned
// buffered; nothing downstream can tell which path produced it.
func streamed(body io.Reader, watch *watcher) (string, *chatUsage, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, streamBufferBytes), maxStreamLineBytes)

	var text strings.Builder
	var usage *chatUsage
	finish := ""

	for scanner.Scan() {
		// Field lines are terminated CRLF or LF; everything that is not a data
		// field — comments, `event:`, the blank line between frames — is noise
		// to a client that only ever receives one kind of message.
		data, ok := strings.CutPrefix(strings.TrimSuffix(scanner.Text(), "\r"), "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "" {
			continue
		}
		if data == streamDone {
			break
		}

		var chunk chatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return text.String(), usage,
				fmt.Errorf("chat stream frame is not JSON: %w (%s)", err, snippet(data))
		}
		// An upstream failure arrives with a matching status today; this is the
		// belt to that braces, and mid-stream it is the only way one can arrive
		// at all — the status line is long gone by then.
		if chunk.Error != nil && chunk.Error.Message != "" {
			return text.String(), usage, fmt.Errorf("9router: %s", chunk.Error.Message)
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		if reason := chunk.Choices[0].FinishReason; reason != "" {
			finish = reason
		}
		if delta := chunk.Choices[0].Delta.Content; delta != "" {
			text.WriteString(delta)
			watch.delta(delta)
		}
	}
	if err := scanner.Err(); err != nil {
		return text.String(), usage, fmt.Errorf("read chat stream: %w", err)
	}

	content := strings.TrimSpace(text.String())
	if content == "" {
		return "", usage, fmt.Errorf("9router returned an empty completion (finish_reason %q)", finish)
	}
	return content, usage, nil
}

// buffered decodes a whole-body completion, for a gateway that ignored the
// streaming request. The observer is told once rather than not at all, so the
// console shows the answer arriving late rather than showing nothing.
func buffered(body io.Reader, watch *watcher) (string, *chatUsage, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return "", nil, fmt.Errorf("read chat response: %w", err)
	}
	var decoded chatResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", nil, fmt.Errorf("chat response is not JSON: %w (%s)", err, snippet(string(raw)))
	}
	if decoded.Error != nil && decoded.Error.Message != "" {
		return "", decoded.Usage, fmt.Errorf("9router: %s", decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 {
		return "", decoded.Usage, fmt.Errorf("9router returned no choices (%s)", snippet(string(raw)))
	}
	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if content == "" {
		return "", decoded.Usage, fmt.Errorf("9router returned an empty completion (finish_reason %q)",
			decoded.Choices[0].FinishReason)
	}
	watch.delta(content)
	return content, decoded.Usage, nil
}

// isEventStream reports whether a response is the stream that was asked for.
// Parsed rather than compared, because the header carries a charset.
func isEventStream(contentType string) bool {
	media, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return media == "text/event-stream"
}

func (c *Client) authorize(req *http.Request) {
	if key := strings.TrimSpace(c.cfg.APIKey()); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
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
