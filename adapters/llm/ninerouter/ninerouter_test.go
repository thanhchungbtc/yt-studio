package ninerouter_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/tbui/yt-studio/adapters/assetstore"
	"github.com/tbui/yt-studio/adapters/llm/ninerouter"
	"github.com/tbui/yt-studio/domain/provider"
)

// Nothing here talks to a real gateway. The contract these tests hold is the
// one 9router was observed to have, which is not quite the one the OpenAI shape
// implies — that gap is the whole reason the adapter exists.

const testModel = "ag/gemini-3-flash"

// staticModel is the resolver a test uses when the model never changes. In
// production it reads the settings row, so a model picked on the settings
// screen applies to the next generation.
func staticModel(id string) func() string { return func() string { return id } }

// gateway is a stand-in 9router. It records the last request so a test can
// assert what went out, and replies with whatever the test handed it.
type gateway struct {
	server *httptest.Server

	path   string
	auth   string
	body   map[string]any
	called int
}

func newGateway(t *testing.T, status int, reply string) *gateway {
	t.Helper()
	g := &gateway{}
	g.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		g.called++
		g.path = r.URL.Path
		g.auth = r.Header.Get("Authorization")
		g.body = nil
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &g.body); err != nil {
				t.Errorf("request body is not JSON: %v", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(g.server.Close)
	return g
}

// messages returns the roles and contents of the recorded chat request.
func (g *gateway) messages(t *testing.T) map[string]string {
	t.Helper()
	raw, ok := g.body["messages"].([]any)
	if !ok {
		t.Fatalf("request carried no messages: %#v", g.body)
	}
	out := make(map[string]string, len(raw))
	for _, m := range raw {
		msg, ok := m.(map[string]any)
		if !ok {
			t.Fatalf("message is not an object: %#v", m)
		}
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)
		out[role] = content
	}
	return out
}

// newStore is a real asset store in a temporary directory. The store is cheap
// and exercising it means the content addresses these tests assert are the ones
// production would produce.
func newStore(t *testing.T) *assetstore.FS {
	t.Helper()
	store, err := assetstore.New(filepath.Join(t.TempDir(), "assets"))
	if err != nil {
		t.Fatalf("assetstore.New: %v", err)
	}
	return store
}

func newClient(t *testing.T, g *gateway, key string) *ninerouter.Client {
	t.Helper()
	return newClientWithStore(t, g, key, newStore(t))
}

func newClientWithStore(t *testing.T, g *gateway, key string, store provider.AssetStore) *ninerouter.Client {
	t.Helper()
	c, err := ninerouter.New(ninerouter.Config{
		BaseURL: g.server.URL,
		APIKey:  key,
		Model:   staticModel(testModel),
		Timeout: 5 * time.Second,
	}, store, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// completion wraps content in the non-streaming response 9router returns when
// stream is explicitly false.
func completion(content string) string {
	body, err := json.Marshal(map[string]any{
		"id":     "chatcmpl-test",
		"object": "chat.completion",
		"choices": []any{map[string]any{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": content},
			"finish_reason": "stop",
		}},
	})
	if err != nil {
		panic(err)
	}
	return string(body)
}

func gatewayError(message string) string {
	return `{"error":{"message":` + strconv.Quote(message) + `}}`
}

func TestNewRejectsBadConfig(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	cases := []struct {
		name string
		cfg  ninerouter.Config
	}{
		{"no base url", ninerouter.Config{Model: staticModel(testModel)}},
		{"relative base url", ninerouter.Config{BaseURL: "localhost:20128", Model: staticModel(testModel)}},
		{"no model resolver", ninerouter.Config{BaseURL: "http://localhost:20128"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := ninerouter.New(tc.cfg, store, nil); !errors.Is(err, provider.ErrUnavailable) {
				t.Fatalf("New = %v, want ErrUnavailable", err)
			}
		})
	}
	if _, err := ninerouter.New(ninerouter.Config{
		BaseURL: "http://localhost:20128", Model: staticModel(testModel),
	}, nil, nil); !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("New with a nil store = %v, want ErrUnavailable", err)
	}
}

// The finding this adapter exists for: 9router streams unless stream is
// explicitly false, so the field must go out on every request and must never
// be omitempty.
func TestChatAlwaysSendsStreamFalse(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(outline(1)))
	c := newClient(t, g, "")

	if _, err := c.Blueprint(context.Background(), testRequest(3)); err != nil {
		t.Fatalf("Blueprint: %v", err)
	}
	stream, present := g.body["stream"]
	if !present {
		t.Fatal("the request omitted stream, which is what makes the gateway send SSE")
	}
	if stream != false {
		t.Fatalf("stream = %v, want false", stream)
	}
	if g.path != "/v1/chat/completions" {
		t.Fatalf("posted to %q, want /v1/chat/completions", g.path)
	}
	if g.body["model"] != testModel {
		t.Fatalf("model = %v, want %q", g.body["model"], testModel)
	}
}

// response_format is accepted by the gateway and silently ignored, so sending
// it would read like a guarantee the wire does not give.
func TestChatSendsNoResponseFormat(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(outline(1)))
	c := newClient(t, g, "")

	if _, err := c.Blueprint(context.Background(), testRequest(3)); err != nil {
		t.Fatalf("Blueprint: %v", err)
	}
	if _, present := g.body["response_format"]; present {
		t.Fatal("the request carried response_format, which 9router ignores")
	}
}

func TestChatSendsBearerOnlyWhenKeyIsSet(t *testing.T) {
	t.Parallel()
	reply := completion(outline(1))

	t.Run("with a key", func(t *testing.T) {
		t.Parallel()
		g := newGateway(t, http.StatusOK, reply)
		if _, err := newClient(t, g, "sk-test").Blueprint(context.Background(), testRequest(3)); err != nil {
			t.Fatalf("Blueprint: %v", err)
		}
		if g.auth != "Bearer sk-test" {
			t.Fatalf("Authorization = %q, want %q", g.auth, "Bearer sk-test")
		}
	})

	// A local gateway may run with auth off, and an empty bearer header is worse
	// than none at all.
	t.Run("without a key", func(t *testing.T) {
		t.Parallel()
		g := newGateway(t, http.StatusOK, reply)
		if _, err := newClient(t, g, "").Blueprint(context.Background(), testRequest(3)); err != nil {
			t.Fatalf("Blueprint: %v", err)
		}
		if g.auth != "" {
			t.Fatalf("Authorization = %q, want it absent", g.auth)
		}
	})
}

// The retry class of a failure is the adapter's most consequential output: it
// decides whether a video spends its attempts on an answer that cannot change.
func TestErrorRetryClasses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		status      int
		reply       string
		unavailable bool
	}{
		{"expired upstream credential", http.StatusUnauthorized,
			gatewayError("[cc/claude-opus-4-8] [401]: OAuth access token has expired."), true},
		{"upstream needs a subscription", http.StatusForbidden,
			gatewayError("[ollama/qwen3.5] [403]: this model requires a subscription"), true},
		{"unknown model", http.StatusBadRequest,
			gatewayError("Invalid model format"), true},
		{"rate limited", http.StatusTooManyRequests,
			gatewayError("slow down"), false},
		{"all accounts unavailable", http.StatusServiceUnavailable,
			gatewayError("All accounts unavailable (reset after 2m)"), false},
		{"gateway fault", http.StatusInternalServerError, `{}`, false},
		{"error in the body of a 200", http.StatusOK,
			gatewayError("[kimi/kimi-k2.5] [401]: The API Key appears to be invalid"), false},
		{"no choices", http.StatusOK, `{"choices":[]}`, false},
		{"blank completion", http.StatusOK, completion("   "), false},
		{"body is not JSON", http.StatusOK, `<html>bad gateway</html>`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := newGateway(t, tc.status, tc.reply)
			_, err := newClient(t, g, "").Blueprint(context.Background(), testRequest(3))
			if err == nil {
				t.Fatal("expected an error")
			}
			if got := errors.Is(err, provider.ErrUnavailable); got != tc.unavailable {
				t.Fatalf("ErrUnavailable = %v, want %v (err: %v)", got, tc.unavailable, err)
			}
		})
	}
}

// An unreachable gateway is not worth three attempts to discover.
func TestUnreachableGatewayIsUnavailable(t *testing.T) {
	t.Parallel()
	// A server that is closed the moment it exists: the port is nobody's.
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := dead.URL
	dead.Close()

	c, err := ninerouter.New(ninerouter.Config{BaseURL: url, Model: staticModel(testModel), Timeout: time.Second}, newStore(t), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.Blueprint(context.Background(), testRequest(3)); !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("Blueprint = %v, want ErrUnavailable", err)
	}
	if err := c.Check(context.Background()); !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("Check = %v, want ErrUnavailable", err)
	}
}

// Cancelling a video cancels the call inside it.
func TestChatHonoursContextCancellation(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.Close)

	c, err := ninerouter.New(ninerouter.Config{BaseURL: server.URL, Model: staticModel(testModel)}, newStore(t), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Blueprint(ctx, testRequest(3)); !errors.Is(err, context.Canceled) {
		t.Fatalf("Blueprint = %v, want a cancelled context", err)
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status int
		reply  string
		ok     bool
	}{
		{"healthy", http.StatusOK, `{"ok":true}`, true},
		{"not ok", http.StatusOK, `{"ok":false}`, false},
		{"not json", http.StatusOK, `nope`, false},
		{"unreachable path", http.StatusNotFound, `not found`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := newGateway(t, tc.status, tc.reply)
			err := newClient(t, g, "").Check(context.Background())
			if tc.ok {
				if err != nil {
					t.Fatalf("Check = %v, want nil", err)
				}
				if g.path != "/api/health" {
					t.Fatalf("probed %q, want /api/health", g.path)
				}
				return
			}
			if !errors.Is(err, provider.ErrUnavailable) {
				t.Fatalf("Check = %v, want ErrUnavailable", err)
			}
		})
	}
}

// A base URL with a trailing slash must not produce a doubled one.
func TestBaseURLTrailingSlashIsTrimmed(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, `{"ok":true}`)
	c, err := ninerouter.New(ninerouter.Config{
		BaseURL: g.server.URL + "/", Model: staticModel(testModel), Timeout: 5 * time.Second,
	}, newStore(t), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Check(context.Background()); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if g.path != "/api/health" {
		t.Fatalf("probed %q, want /api/health", g.path)
	}
}
