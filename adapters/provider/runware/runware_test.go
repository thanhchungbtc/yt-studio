package runware_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tbui/yt-studio/adapters/assetstore"
	"github.com/tbui/yt-studio/adapters/provider/runware"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// Nothing here talks to the real API. A generation costs money and produces a
// different image every time, neither of which a test can assert against; what
// these hold is the request that goes out and the classification of what comes
// back.

const (
	testKey   = "rw-test-key"
	testModel = "runware:100@1"
)

// pixel is a one-pixel PNG, so what the store ingests is a real image rather
// than a string that happens to be bytes.
var pixel = []byte{
	0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89,
	0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T',
	0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05, 0x00, 0x01,
	0x0d, 0x0a, 0x2d, 0xb4,
	0x00, 0x00, 0x00, 0x00, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82,
}

// api is a stand-in Runware. It answers the inference POST at / and serves the
// image it advertised at /image, because the real API hands back a URL rather
// than bytes and the adapter's second half is the fetch.
type api struct {
	server *httptest.Server

	// inference is what the POST replies with. Zero status means 200 and the
	// canned success body pointing at this server's own image route.
	status int
	reply  string
	// imageStatus and image are what the download route serves.
	imageStatus int
	image       []byte

	auth     string
	tasks    []map[string]any
	inferred int
	fetched  int
}

func newAPI(t *testing.T) *api {
	t.Helper()
	a := &api{image: pixel}
	mux := http.NewServeMux()
	mux.HandleFunc("/image", func(w http.ResponseWriter, _ *http.Request) {
		a.fetched++
		if a.imageStatus != 0 && a.imageStatus != http.StatusOK {
			w.WriteHeader(a.imageStatus)
			_, _ = io.WriteString(w, "gone")
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(a.image)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		a.inferred++
		a.auth = r.Header.Get("Authorization")
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		// The body is an array of tasks even when it carries one; decoding it as
		// anything else is the assertion that it stays that way.
		a.tasks = nil
		if err := json.Unmarshal(raw, &a.tasks); err != nil {
			t.Errorf("request body is not a JSON array of tasks: %v (%s)", err, raw)
		}
		w.Header().Set("Content-Type", "application/json")
		if a.status != 0 {
			w.WriteHeader(a.status)
			_, _ = io.WriteString(w, a.reply)
			return
		}
		reply := a.reply
		if reply == "" {
			reply = `{"data":[{"taskUUID":"x","imageURL":"` + a.server.URL + `/image"}]}`
		}
		_, _ = io.WriteString(w, reply)
	})
	a.server = httptest.NewServer(mux)
	t.Cleanup(a.server.Close)
	return a
}

// task returns the single task the last request carried.
func (a *api) task(t *testing.T) map[string]any {
	t.Helper()
	if len(a.tasks) != 1 {
		t.Fatalf("expected exactly one task, got %d", len(a.tasks))
	}
	return a.tasks[0]
}

// newStore builds the real content-addressed store over a temporary directory,
// so the addresses these tests see are the ones production would produce.
func newStore(t *testing.T) *assetstore.FS {
	t.Helper()
	store, err := assetstore.New(filepath.Join(t.TempDir(), "assets"))
	if err != nil {
		t.Fatalf("assetstore.New: %v", err)
	}
	return store
}

func newClient(t *testing.T, a *api, key string, width, height int) *runware.Client {
	t.Helper()
	client, err := runware.New(runware.Config{
		APIKey:           key,
		Model:            func() string { return testModel },
		SlideSize:        func() (int, int) { return width, height },
		BaseURL:          a.server.URL,
		InferenceTimeout: 5 * time.Second,
		DownloadTimeout:  5 * time.Second,
	}, newStore(t), nil)
	if err != nil {
		t.Fatalf("runware.New: %v", err)
	}
	return client
}

func TestNewRejectsIncompleteConfig(t *testing.T) {
	t.Parallel()
	model := func() string { return testModel }
	size := func() (int, int) { return 512, 512 }
	store, err := assetstore.New(filepath.Join(t.TempDir(), "assets"))
	if err != nil {
		t.Fatalf("assetstore.New: %v", err)
	}

	for name, cfg := range map[string]runware.Config{
		"no model resolver": {APIKey: testKey, SlideSize: size},
		"no size resolver":  {APIKey: testKey, Model: model},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := runware.New(cfg, store, nil); !errors.Is(err, provider.ErrUnavailable) {
				t.Fatalf("expected ErrUnavailable, got %v", err)
			}
		})
	}

	t.Run("no store", func(t *testing.T) {
		t.Parallel()
		cfg := runware.Config{APIKey: testKey, Model: model, SlideSize: size}
		if _, err := runware.New(cfg, nil, nil); !errors.Is(err, provider.ErrUnavailable) {
			t.Fatalf("expected ErrUnavailable, got %v", err)
		}
	})
}

func TestCheckReportsMissingKey(t *testing.T) {
	t.Parallel()
	a := newAPI(t)

	if err := newClient(t, a, testKey, 1344, 768).Check(); err != nil {
		t.Fatalf("a configured client should check out: %v", err)
	}
	err := newClient(t, a, "", 1344, 768).Check()
	if !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable for a missing key, got %v", err)
	}
	// Check costs a generation if it probes, so it must not.
	if a.inferred != 0 {
		t.Fatalf("Check made %d requests; it must make none", a.inferred)
	}
}

func TestGenerateSlideSendsTheConfiguredTask(t *testing.T) {
	t.Parallel()
	a := newAPI(t)
	client := newClient(t, a, testKey, 1344, 768)

	id, err := runware.NewSlide(client).Generate(context.Background(), provider.SlideRequest{
		Prompt: "a chalk diagram of a lever",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if id == "" {
		t.Fatal("Generate returned no asset id")
	}
	if a.inferred != 1 || a.fetched != 1 {
		t.Fatalf("expected one inference and one download, got %d and %d", a.inferred, a.fetched)
	}
	if a.auth != "Bearer "+testKey {
		t.Fatalf("Authorization header is %q", a.auth)
	}

	task := a.task(t)
	for field, want := range map[string]any{
		"taskType":       "imageInference",
		"positivePrompt": "a chalk diagram of a lever",
		"model":          testModel,
		"width":          float64(1344),
		"height":         float64(768),
		"numberResults":  float64(1),
		// PNG rather than JPG: AssetKind pins the extension and the MIME type per
		// kind, and an image is served as image/png.
		"outputFormat": "PNG",
	} {
		if got := task[field]; got != want {
			t.Errorf("task[%q] = %v, want %v", field, got, want)
		}
	}
	if negative, _ := task["negativePrompt"].(string); !strings.Contains(negative, "watermarks") {
		t.Errorf("the house negative prompt did not go out: %q", negative)
	}
	if uuid, _ := task["taskUUID"].(string); len(uuid) != 36 {
		t.Errorf("taskUUID = %q, want a UUID", uuid)
	}
}

func TestGenerateSlidePrefersTheRequestedSize(t *testing.T) {
	t.Parallel()
	a := newAPI(t)
	client := newClient(t, a, testKey, 1344, 768)

	// The port declares the fields; a use case that starts filling them must be
	// obeyed rather than overridden by the settings row.
	if _, err := runware.NewSlide(client).Generate(context.Background(), provider.SlideRequest{
		Prompt: "a lever",
		Width:  1024,
		Height: 1024,
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	task := a.task(t)
	if task["width"] != float64(1024) || task["height"] != float64(1024) {
		t.Fatalf("sent %vx%v, want 1024x1024", task["width"], task["height"])
	}
}

func TestGenerateIconIsSquare(t *testing.T) {
	t.Parallel()
	a := newAPI(t)
	client := newClient(t, a, testKey, 1344, 768)

	id, err := runware.NewIcon(client).Generate(context.Background(), provider.IconRequest{
		Index:  3,
		Prompt: "a lever, thick-stroke white line art",
		Size:   768,
	})
	if err != nil {
		t.Fatalf("Icon: %v", err)
	}
	if id == "" {
		t.Fatal("Icon returned no asset id")
	}
	task := a.task(t)
	if task["width"] != float64(768) || task["height"] != float64(768) {
		t.Fatalf("sent %vx%v, want a 768 square", task["width"], task["height"])
	}
}

func TestGenerateIconFallsBackToADefaultSize(t *testing.T) {
	t.Parallel()
	a := newAPI(t)
	client := newClient(t, a, testKey, 1344, 768)

	if _, err := runware.NewIcon(client).Generate(context.Background(), provider.IconRequest{
		Prompt: "a lever",
	}); err != nil {
		t.Fatalf("Icon: %v", err)
	}
	task := a.task(t)
	if task["width"] != float64(512) || task["height"] != float64(512) {
		t.Fatalf("sent %vx%v, want the 512 default", task["width"], task["height"])
	}
}

func TestStoredAssetsAreAddressedByKind(t *testing.T) {
	t.Parallel()
	a := newAPI(t)
	store := newStore(t)
	client, err := runware.New(runware.Config{
		APIKey:    testKey,
		Model:     func() string { return testModel },
		SlideSize: func() (int, int) { return 512, 512 },
		BaseURL:   a.server.URL,
	}, store, nil)
	if err != nil {
		t.Fatalf("runware.New: %v", err)
	}

	slide, err := runware.NewSlide(client).Generate(context.Background(),
		provider.SlideRequest{Prompt: "a lever"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	icon, err := runware.NewIcon(client).Generate(context.Background(),
		provider.IconRequest{Prompt: "a lever", Size: 512})
	if err != nil {
		t.Fatalf("Icon: %v", err)
	}
	// The same bytes: the content address is the hash, so the two differ only in
	// the kind they were filed under, and each must be readable as that kind.
	if slide != icon {
		t.Fatalf("identical bytes produced different addresses: %s and %s", slide, icon)
	}
	for kind, id := range map[entity.AssetKind]entity.AssetID{
		entity.AssetKindImage:         slide,
		entity.AssetKindThumbnailIcon: icon,
	} {
		if _, err := store.Stat(context.Background(), id, kind); err != nil {
			t.Errorf("stat %s as %s: %v", id, kind, err)
		}
	}
}

// TestFailureClassification is the whole point of the adapter's error handling:
// app.classify asks only whether another attempt could land differently.
func TestFailureClassification(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		status      int
		reply       string
		imageStatus int
		retryable   bool
		contains    string
	}{
		"rejected key": {
			status:   http.StatusUnauthorized,
			reply:    `{"errors":[{"code":"unauthorized","message":"invalid api key"}]}`,
			contains: "invalid api key",
		},
		"unknown model": {
			status:   http.StatusBadRequest,
			reply:    `{"errors":[{"code":"invalidModel","message":"model not found"}]}`,
			contains: "model not found",
		},
		"rate limited": {
			status:    http.StatusTooManyRequests,
			reply:     `{"errors":[{"message":"slow down"}]}`,
			retryable: true,
			contains:  "slow down",
		},
		"outage": {
			status:    http.StatusBadGateway,
			reply:     "upstream is down",
			retryable: true,
			contains:  "upstream is down",
		},
		"errors on a 200": {
			reply:     `{"errors":[{"code":"inferenceFailed","message":"the sampler diverged"}]}`,
			retryable: true,
			contains:  "the sampler diverged",
		},
		"no image in the reply": {
			reply:     `{"data":[]}`,
			retryable: true,
			contains:  "no image",
		},
		"reply is not JSON": {
			reply:     "<html>maintenance</html>",
			retryable: true,
			contains:  "not JSON",
		},
		"download failed": {
			imageStatus: http.StatusNotFound,
			retryable:   true,
			contains:    "download image",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			a := newAPI(t)
			a.status, a.reply, a.imageStatus = tc.status, tc.reply, tc.imageStatus
			client := newClient(t, a, testKey, 512, 512)

			_, err := runware.NewSlide(client).Generate(context.Background(),
				provider.SlideRequest{Prompt: "a lever"})
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Errorf("error %q does not carry %q", err, tc.contains)
			}
			if retryable := !errors.Is(err, provider.ErrUnavailable); retryable != tc.retryable {
				t.Errorf("retryable = %v, want %v (%v)", retryable, tc.retryable, err)
			}
		})
	}
}

func TestGenerateWithoutAKeyNeverLeaves(t *testing.T) {
	t.Parallel()
	a := newAPI(t)
	client := newClient(t, a, "", 512, 512)

	_, err := runware.NewSlide(client).Generate(context.Background(),
		provider.SlideRequest{Prompt: "a lever"})
	if !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if a.inferred != 0 {
		t.Fatalf("a keyless client sent %d requests; it must send none", a.inferred)
	}
}

func TestGenerateRejectsAnImpossibleSize(t *testing.T) {
	t.Parallel()
	a := newAPI(t)
	client := newClient(t, a, testKey, 0, 0)

	// The API's own grid and range rules are left to the API, but a zero is not a
	// size under any of them and is worth neither the round trip nor a retry.
	_, err := runware.NewSlide(client).Generate(context.Background(),
		provider.SlideRequest{Prompt: "a lever"})
	if !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if a.inferred != 0 {
		t.Fatalf("sent %d requests for a 0x0 image; it must send none", a.inferred)
	}
}

func TestCancellationAbortsInFlight(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(func() {
		close(release)
		server.Close()
	})
	client, err := runware.New(runware.Config{
		APIKey:    testKey,
		Model:     func() string { return testModel },
		SlideSize: func() (int, int) { return 512, 512 },
		BaseURL:   server.URL,
	}, newStore(t), nil)
	if err != nil {
		t.Fatalf("runware.New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runware.NewSlide(client).Generate(ctx,
		provider.SlideRequest{Prompt: "a lever"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the cancellation to surface, got %v", err)
	}
}
