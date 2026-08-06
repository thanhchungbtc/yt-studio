package http_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tbui/yt-studio/adapters/assetstore"
	"github.com/tbui/yt-studio/adapters/eventbus"
	llmmock "github.com/tbui/yt-studio/adapters/provider/mock/llm"
	mediamock "github.com/tbui/yt-studio/adapters/provider/mock/media"
	"github.com/tbui/yt-studio/adapters/sqlite"
	"github.com/tbui/yt-studio/app"
	deliveryhttp "github.com/tbui/yt-studio/delivery/http"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/scheduler"
	"github.com/tbui/yt-studio/domain/service"
)

// harness wires the real stack — SQLite, the content-addressed store, the
// mock providers, the scheduler and the router — behind an httptest server.
// Nothing is stubbed except the clock and the id generator, so these tests
// exercise the same code path the binary does.
type harness struct {
	t      *testing.T
	server *httptest.Server
	store  *sqlite.Store
	assets *assetstore.FS
	sched  *scheduler.Scheduler
	broker *eventbus.Broker
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	store, err := sqlite.Open(ctx, sqlite.Options{Path: filepath.Join(t.TempDir(), "test.db")}, log)
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	// The writer outlives the request context so the scheduler's final transitions
	// still commit during shutdown; it is stopped explicitly last.
	writerCtx, stopWriter := context.WithCancel(context.Background())
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		_ = store.Run(writerCtx)
	}()

	if err := sqlite.SeedSettings(ctx, store); err != nil {
		t.Fatalf("SeedSettings: %v", err)
	}
	if err := sqlite.SeedChannels(ctx, store, time.Unix(0, 0).UTC()); err != nil {
		t.Fatalf("SeedChannels: %v", err)
	}
	settings := service.NewSettings(store, store)
	if err := settings.Load(ctx); err != nil {
		t.Fatalf("settings.Load: %v", err)
	}

	assets, err := assetstore.New(filepath.Join(t.TempDir(), "assets"))
	if err != nil {
		t.Fatalf("assetstore.New: %v", err)
	}
	broker := eventbus.New(10*time.Millisecond, log)

	llm := llmmock.NewLLM(assets, videoContext(store))
	// An ungated blueprint expands its own video's DAG, so the runner needs the
	// scheduler that is built from it. The reference is filled in below, before
	// the loop starts.
	expander := &lateExpander{}
	runner := app.NewTaskRunner(
		store, store, store, store, store, store, store, assets,
		llm,
		mediamock.NewTTS(assets),
		mediamock.NewSlide(assets),
		mediamock.NewComposer(assets),
		mediamock.NewThumbnail(assets),
		mediamock.NewIcon(assets),
		mediamock.NewUploader(assets, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }),
		broker,
		expander,
		func() app.BlueprintOptions {
			return app.BlueprintOptions{
				ChapterTolerancePercent: settings.Int(entity.SettingVideoChapterTolerancePercent),
				MaxAttempts:             settings.Int(entity.SettingTaskMaxAttempts),
				UploadGate:              settings.GateEnabled(entity.GateUpload),
			}
		},
		func() app.IconOptions {
			return app.IconOptions{
				Style: settings.String(entity.SettingThumbnailIconStyle),
				Size:  settings.Int(entity.SettingThumbnailIconSize),
			}
		},
		func() bool { return settings.Bool(entity.SettingUploadDryRun) },
		time.Now, log,
	)
	pools, err := scheduler.NewPools(settings.PoolLimits())
	if err != nil {
		t.Fatalf("NewPools: %v", err)
	}
	sched := scheduler.New(pools, store, runner, store, broker, log, scheduler.Config{
		RetryBase: 5 * time.Millisecond, RetryMax: 20 * time.Millisecond,
	})
	expander.sched = sched

	schedDone := make(chan struct{})
	go func() {
		defer close(schedDone)
		_ = sched.Run(ctx)
	}()
	brokerDone := make(chan struct{})
	go func() {
		defer close(brokerDone)
		_ = broker.Run(ctx)
	}()

	var counter atomic.Uint64
	level := &slog.LevelVar{}
	handler, _ := deliveryhttp.NewRouter(deliveryhttp.Deps{
		Channels: store, ChannelWriter: store, Videos: store, VideoWriter: store,
		VideoStates: store, VideoFields: store, Chapters: store, ChapterFields: store, Assets: store,
		Tasks: store, Store: assets, Settings: settings,
		Submitter: sched, Expander: sched, Canceller: sched, Approver: sched, Rejecter: sched,
		Forgetter: sched, TaskRetry: sched, ChapRetry: sched, Rerunner: sched, Pools: sched,
		Reporter: sched, Prompts: llm, Notifier: broker, Coalescer: broker,
		Events: broker, SSEClients: broker.Subscribers,
		LogLevel: level, Log: log,
		// Deterministic ids keep artifact hashes stable across runs.
		NewID:   func() string { return "vid-" + strconv.FormatUint(counter.Add(1), 10) },
		Now:     func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		Version: "test", Started: time.Unix(1_700_000_000, 0).UTC(),
	})

	server := httptest.NewServer(handler)
	h := &harness{t: t, server: server, store: store, assets: assets, sched: sched, broker: broker}

	t.Cleanup(func() {
		server.Close()
		cancel()
		<-schedDone
		<-brokerDone
		stopWriter()
		<-writerDone
		_ = store.Close()
	})
	return h
}

// lateExpander breaks the cycle between the task runner and the scheduler. It
// mirrors the production wiring in cmd/server.
type lateExpander struct{ sched *scheduler.Scheduler }

var _ app.GraphExpander = (*lateExpander)(nil)

func (e *lateExpander) Expand(ctx context.Context, videoID entity.VideoID, tail scheduler.Tail) error {
	return e.sched.Expand(ctx, videoID, tail)
}

func videoContext(store *sqlite.Store) llmmock.ContextLookup {
	return func(ctx context.Context, videoID entity.VideoID) (llmmock.VideoContext, error) {
		v, err := store.VideoByID(ctx, videoID)
		if err != nil {
			return llmmock.VideoContext{}, err
		}
		rows, err := store.ListChaptersByVideo(ctx, videoID)
		if err != nil {
			return llmmock.VideoContext{}, err
		}
		outline := make([]provider.BlueprintChapter, 0, len(rows))
		for _, c := range rows {
			outline = append(outline, provider.BlueprintChapter{Ordinal: c.Ordinal, Title: c.Title, Summary: c.Summary})
		}
		return llmmock.VideoContext{
			Ref: v.Ref, Title: v.Title, Topic: v.Topic,
			Chapters: outline, SlidesPerChapter: v.SlidesPerChapter,
		}, nil
	}
}

func (h *harness) do(method, path string, body any) (*http.Response, []byte) {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			h.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, h.server.URL+path, reader)
	if err != nil {
		h.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.server.Client().Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatal(err)
	}
	return resp, raw
}

func (h *harness) json(method, path string, body any, want int, out any) {
	h.t.Helper()
	resp, raw := h.do(method, path, body)
	if resp.StatusCode != want {
		h.t.Fatalf("%s %s = %d, want %d: %s", method, path, resp.StatusCode, want, raw)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			h.t.Fatalf("decode %s: %v (%s)", path, err, raw)
		}
	}
}

type videoBody struct {
	ID     string `json:"id"`
	Ref    string `json:"ref"`
	State  string `json:"state"`
	Counts struct {
		Total     int `json:"total"`
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
	} `json:"counts"`
	BlueprintAssetID string `json:"blueprintAssetId"`
	FinalAssetID     string `json:"finalAssetId"`
	ThumbnailAssetID string `json:"thumbnailAssetId"`
	Upload           *struct {
		URL    string `json:"url"`
		DryRun bool   `json:"dryRun"`
	} `json:"upload"`
	Metadata *struct {
		Title string `json:"title"`
	} `json:"metadata"`
}

func (h *harness) video(ref string) videoBody {
	h.t.Helper()
	var v videoBody
	h.json(http.MethodGet, "/api/videos/"+ref, nil, http.StatusOK, &v)
	return v
}

func (h *harness) waitForState(ref, state string, timeout time.Duration) videoBody {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	var last videoBody
	for time.Now().Before(deadline) {
		last = h.video(ref)
		if last.State == state {
			return last
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.t.Fatalf("video %s is %q after %s, want %q (counts %+v)", ref, last.State, timeout, state, last.Counts)
	return last
}

// waitForGate waits for a specific gate to open. Waiting on the video state
// alone is not enough: after one gate is approved the video row still reads
// awaiting_approval until the loop recomputes it, so a test could act on the
// gate it just closed.
func (h *harness) waitForGate(ref, gate string, timeout time.Duration) {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var tasks struct {
			Tasks []struct {
				State string `json:"state"`
				Gate  string `json:"gate"`
			} `json:"tasks"`
		}
		h.json(http.MethodGet, "/api/videos/"+ref+"/tasks", nil, http.StatusOK, &tasks)
		for _, t := range tasks.Tasks {
			if t.State == "awaiting_approval" && t.Gate == gate {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.t.Fatalf("the %s gate on %s never opened within %s", gate, ref, timeout)
}

// approve waits for a gate and releases it.
func (h *harness) approve(ref, gate string, timeout time.Duration) {
	h.t.Helper()
	h.waitForGate(ref, gate, timeout)
	h.json(http.MethodPost, "/api/videos/"+ref+"/approve", map[string]any{"gate": gate}, http.StatusOK, nil)
}

// testThumbnailCells is the grid width every test video asks for. Deliberately
// not the shipped default: a test that passes only at the default is a
// test that is reading the default.
const testThumbnailCells = 4

func createVideo(h *harness, chapters, slides int, start bool) videoBody {
	h.t.Helper()
	var v videoBody
	h.json(http.MethodPost, "/api/videos", map[string]any{
		"channel":          "deep-sleep-stories",
		"title":            "The Long Winter of the Harbour",
		"topic":            "a northern port town",
		"chapterCount":     chapters,
		"slidesPerChapter": slides,
		"thumbnailCells":   testThumbnailCells,
		"start":            start,
	}, http.StatusCreated, &v)
	return v
}

// The whole of phase 2: a full pipeline end to end against the mocks, through
// both approval gates, with real files on disk at the end.
func TestPipelineEndToEndThroughBothGates(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	v := createVideo(h, 6, 2, true)
	if v.Ref != "DSS-1" {
		t.Fatalf("ref = %q, want DSS-1", v.Ref)
	}
	// A video is enqueued as a lone blueprint. How many chapter branches it needs
	// is the blueprint's output, so the rest of the DAG cannot exist yet.
	if v.Counts.Total != 1 {
		t.Fatalf("tasks at enqueue = %d, want 1", v.Counts.Total)
	}

	// Gate 1: the pipeline parks after the blueprint.
	parked := h.waitForState("DSS-1", "awaiting_approval", 15*time.Second)
	if parked.BlueprintAssetID == "" {
		t.Fatal("no blueprint artifact was recorded")
	}
	if parked.Counts.Succeeded != 0 {
		t.Fatalf("%d tasks ran past the blueprint gate", parked.Counts.Succeeded)
	}
	if parked.Counts.Total != 1 {
		t.Fatalf("tasks at the blueprint gate = %d, want 1: the graph expands on approval",
			parked.Counts.Total)
	}

	var chapters struct {
		Chapters []struct {
			Ordinal int `json:"ordinal"`
			Title   string
		} `json:"chapters"`
	}
	h.json(http.MethodGet, "/api/videos/DSS-1/chapters", nil, http.StatusOK, &chapters)
	if len(chapters.Chapters) != 6 {
		t.Fatalf("chapters = %d, want 6", len(chapters.Chapters))
	}

	h.json(http.MethodPost, "/api/videos/DSS-1/approve", map[string]any{"gate": "blueprint"}, http.StatusOK, nil)

	// Approving the outline is what fixes the video's shape: the DAG is built for
	// the chapters that were just signed off, not for the number briefed.
	expanded := h.video("DSS-1")
	if expanded.Counts.Total != scheduler.NodeCountFor(len(chapters.Chapters), 2, testThumbnailCells) {
		t.Fatalf("tasks after approval = %d, want %d",
			expanded.Counts.Total, scheduler.NodeCountFor(len(chapters.Chapters), 2, testThumbnailCells))
	}

	// Gate 2: the pipeline parks again before upload.
	h.waitForGate("DSS-1", "upload", 30*time.Second)
	beforeUpload := h.video("DSS-1")
	if beforeUpload.FinalAssetID == "" {
		t.Fatal("the final render was not produced before the upload gate")
	}
	if beforeUpload.Metadata == nil || beforeUpload.Metadata.Title == "" {
		t.Fatal("metadata was not produced before the upload gate")
	}
	// The gate is worth nothing if it opens on a listing the operator cannot see
	// in full: the thumbnail is rendered before it, not after, and the video row
	// names it so the UI can show what is about to be published.
	if beforeUpload.ThumbnailAssetID == "" {
		t.Fatal("the thumbnail was not produced before the upload gate")
	}
	// And it was built from a full grid: one plan, one icon per tile, every slot
	// filled. An icon that never landed would render as a hole.
	var gridAssets struct {
		Assets []struct {
			Kind string `json:"kind"`
		} `json:"assets"`
	}
	h.json(http.MethodGet, "/api/videos/DSS-1/assets", nil, http.StatusOK, &gridAssets)
	byKind := map[string]int{}
	for _, a := range gridAssets.Assets {
		byKind[a.Kind]++
	}
	if byKind["thumbnail_plan"] != 1 {
		t.Errorf("thumbnail plan assets = %d, want 1", byKind["thumbnail_plan"])
	}
	// Icons are content-addressed, so two cells that asked for the same picture
	// share a file: the count is a ceiling, not an equality.
	if n := byKind["thumbnail_icon"]; n == 0 || n > testThumbnailCells {
		t.Errorf("thumbnail icon assets = %d, want 1..%d", n, testThumbnailCells)
	}
	if beforeUpload.Upload != nil {
		t.Fatal("the upload ran before its gate was approved")
	}

	h.json(http.MethodPost, "/api/videos/DSS-1/approve", map[string]any{"gate": "upload"}, http.StatusOK, nil)
	done := h.waitForState("DSS-1", "completed", 15*time.Second)

	if done.Counts.Succeeded != done.Counts.Total {
		t.Fatalf("succeeded %d of %d", done.Counts.Succeeded, done.Counts.Total)
	}
	if done.Upload == nil || !done.Upload.DryRun || done.Upload.URL == "" {
		t.Fatalf("upload receipt = %+v", done.Upload)
	}

	// Every chapter has its narration, both slides and a clip.
	var full struct {
		Chapters []struct {
			Ordinal       int      `json:"ordinal"`
			Script        string   `json:"script"`
			SlidePrompts  []string `json:"slidePrompts"`
			AudioAssetID  string   `json:"audioAssetId"`
			SlideAssetIDs []string `json:"slideAssetIds"`
			ClipAssetID   string   `json:"clipAssetId"`
		} `json:"chapters"`
	}
	h.json(http.MethodGet, "/api/videos/DSS-1/chapters", nil, http.StatusOK, &full)
	for _, c := range full.Chapters {
		if c.Script == "" {
			t.Errorf("chapter %d has no script", c.Ordinal)
		}
		if len(c.SlidePrompts) != 2 {
			t.Errorf("chapter %d has %d prompts, want 2", c.Ordinal, len(c.SlidePrompts))
		}
		if c.AudioAssetID == "" || c.ClipAssetID == "" {
			t.Errorf("chapter %d is missing audio or clip", c.Ordinal)
		}
		for j, id := range c.SlideAssetIDs {
			if id == "" {
				t.Errorf("chapter %d slide %d is missing", c.Ordinal, j)
			}
		}
	}

	// The artifacts are really on disk and really served.
	resp, body := h.do(http.MethodGet, "/assets/"+done.FinalAssetID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET final asset = %d", resp.StatusCode)
	}
	if len(body) < 1000 {
		t.Fatalf("final render is only %d bytes", len(body))
	}
	if got := resp.Header.Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Errorf("Cache-Control = %q, want an immutable directive", got)
	}
	if resp.Header.Get("Content-Type") != "video/mp4" {
		t.Errorf("Content-Type = %q", resp.Header.Get("Content-Type"))
	}
}

// The same seed and inputs must produce identical artifact hashes.
func TestArtifactHashesAreReproducible(t *testing.T) {
	t.Parallel()

	run := func() (string, string, []string) {
		h := newHarness(t)
		createVideo(h, 3, 2, true)
		h.approve("DSS-1", "blueprint", 15*time.Second)
		h.approve("DSS-1", "upload", 30*time.Second)
		v := h.waitForState("DSS-1", "completed", 15*time.Second)

		var assets struct {
			Assets []struct {
				ID   string `json:"id"`
				Kind string `json:"kind"`
			} `json:"assets"`
		}
		h.json(http.MethodGet, "/api/videos/DSS-1/assets", nil, http.StatusOK, &assets)
		// Assets are listed in completion order, which concurrency varies; the
		// reproducibility claim is about the set of artifacts, not their order.
		ids := make([]string, 0, len(assets.Assets))
		for _, a := range assets.Assets {
			ids = append(ids, a.Kind+":"+a.ID)
		}
		sort.Strings(ids)
		return v.BlueprintAssetID, v.FinalAssetID, ids
	}

	bp1, final1, ids1 := run()
	bp2, final2, ids2 := run()

	if bp1 != bp2 {
		t.Errorf("blueprint hash differs between runs:\n%s\n%s", bp1, bp2)
	}
	if final1 != final2 {
		t.Errorf("final render hash differs between runs:\n%s\n%s", final1, final2)
	}
	if len(ids1) != len(ids2) {
		t.Fatalf("asset count differs: %d vs %d", len(ids1), len(ids2))
	}
	for i := range ids1 {
		if ids1[i] != ids2[i] {
			t.Fatalf("asset %d differs:\n%s\n%s", i, ids1[i], ids2[i])
		}
	}
	t.Logf("%d artifacts reproduced byte-for-byte; final render %s", len(ids1), final1[:16])
}

// The dispatch sequence must be deterministic too, not just the artifacts.
func TestTaskSequenceIsDeterministic(t *testing.T) {
	t.Parallel()

	run := func() []string {
		h := newHarness(t)
		createVideo(h, 4, 2, false)
		h.json(http.MethodPost, "/api/videos/DSS-1/start", nil, http.StatusOK, nil)
		h.approve("DSS-1", "blueprint", 15*time.Second)
		h.waitForGate("DSS-1", "upload", 30*time.Second)

		var tasks struct {
			Tasks []struct {
				ID    string `json:"id"`
				Kind  string `json:"kind"`
				State string `json:"state"`
			} `json:"tasks"`
		}
		h.json(http.MethodGet, "/api/videos/DSS-1/tasks", nil, http.StatusOK, &tasks)
		out := make([]string, 0, len(tasks.Tasks))
		for _, t := range tasks.Tasks {
			out = append(out, t.ID+"="+t.State)
		}
		return out
	}

	a, b := run(), run()
	if len(a) != len(b) {
		t.Fatalf("task counts differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("task %d differs: %q vs %q", i, a[i], b[i])
		}
	}
}

func TestRejectingTheBlueprintFailsTheVideo(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	createVideo(h, 2, 1, true)
	h.waitForGate("DSS-1", "blueprint", 15*time.Second)

	h.json(http.MethodPost, "/api/videos/DSS-1/reject",
		map[string]any{"gate": "blueprint", "reason": "the outline misses the point"}, http.StatusOK, nil)
	failed := h.waitForState("DSS-1", "failed", 10*time.Second)
	if failed.Counts.Failed == 0 {
		t.Fatal("no task was recorded as failed")
	}

	// Retrying the blueprint task puts the video back in flight.
	var tasks struct {
		Tasks []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"tasks"`
	}
	h.json(http.MethodGet, "/api/videos/DSS-1/tasks", nil, http.StatusOK, &tasks)
	var blueprintID string
	for _, task := range tasks.Tasks {
		if task.Kind == "blueprint" {
			blueprintID = task.ID
		}
	}
	if blueprintID == "" {
		t.Fatal("no blueprint task")
	}
	h.json(http.MethodPost, "/api/tasks/"+blueprintID+"/retry", nil, http.StatusOK, nil)
	h.waitForGate("DSS-1", "blueprint", 15*time.Second)
}

func TestCancelStopsAVideo(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	createVideo(h, 40, 2, true)
	h.approve("DSS-1", "blueprint", 20*time.Second)

	h.json(http.MethodPost, "/api/videos/DSS-1/cancel", nil, http.StatusOK, nil)
	h.waitForState("DSS-1", "cancelled", 10*time.Second)
}

func TestChapterScriptEditAndRetry(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	createVideo(h, 3, 2, true)
	h.approve("DSS-1", "blueprint", 15*time.Second)
	h.waitForGate("DSS-1", "upload", 30*time.Second)

	var chapters struct {
		Chapters []struct {
			ID              string  `json:"id"`
			Ordinal         int     `json:"ordinal"`
			DurationSeconds float64 `json:"durationSeconds"`
		} `json:"chapters"`
	}
	h.json(http.MethodGet, "/api/videos/DSS-1/chapters", nil, http.StatusOK, &chapters)
	target := chapters.Chapters[1]

	var edited struct {
		Script          string  `json:"script"`
		DurationSeconds float64 `json:"durationSeconds"`
	}
	newScript := strings.Repeat("a rewritten sentence for the operator. ", 20)
	h.json(http.MethodPut, "/api/chapters/"+target.ID+"/script",
		map[string]any{"script": newScript}, http.StatusOK, &edited)
	if !strings.HasPrefix(edited.Script, "a rewritten sentence") {
		t.Fatalf("script was not saved: %q", edited.Script[:40])
	}
	if edited.DurationSeconds <= 0 {
		t.Fatal("the duration estimate was not recalculated")
	}

	// Re-running the chapter regenerates its narration from the edited script.
	h.json(http.MethodPost, "/api/videos/DSS-1/chapters/2/retry", nil, http.StatusNoContent, nil)
	h.waitForGate("DSS-1", "upload", 30*time.Second)

	// An empty script is rejected at the boundary.
	resp, _ := h.do(http.MethodPut, "/api/chapters/"+target.ID+"/script", map[string]any{"script": ""})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("empty script = %d, want 422", resp.StatusCode)
	}
}

// Editing a prompt and redrawing the slide it describes is one request. The
// mock painter seeds on the prompt text, so an artifact that changed is proof
// the edit reached the provider rather than just the row.
func TestEditingAPromptRedrawsOneStill(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	createVideo(h, 3, 2, true)
	h.approve("DSS-1", "blueprint", 15*time.Second)
	h.waitForGate("DSS-1", "upload", 30*time.Second)

	type chapterBody struct {
		ID            string   `json:"id"`
		Ordinal       int      `json:"ordinal"`
		SlidePrompts  []string `json:"slidePrompts"`
		SlideAssetIDs []string `json:"slideAssetIds"`
	}
	var chapters struct {
		Chapters []chapterBody `json:"chapters"`
	}
	h.json(http.MethodGet, "/api/videos/DSS-1/chapters", nil, http.StatusOK, &chapters)
	target := chapters.Chapters[1]
	was := target.SlideAssetIDs[0]

	const prompt = "a harbour lighthouse swallowed by fog"
	var edited chapterBody
	h.json(http.MethodPost, "/api/chapters/"+target.ID+"/slides/0/generate",
		map[string]any{"prompt": prompt}, http.StatusOK, &edited)
	if edited.SlidePrompts[0] != prompt {
		t.Fatalf("prompt 0 = %q, want the edit", edited.SlidePrompts[0])
	}
	if edited.SlidePrompts[1] != target.SlidePrompts[1] {
		t.Fatal("the sibling prompt was overwritten by an indexed write")
	}

	// The one slide is redrawn from the new text; its sibling is untouched.
	deadline := time.Now().Add(30 * time.Second)
	var now chapterBody
	for time.Now().Before(deadline) {
		h.json(http.MethodGet, "/api/videos/DSS-1/chapters", nil, http.StatusOK, &chapters)
		now = chapters.Chapters[1]
		if now.SlideAssetIDs[0] != "" && now.SlideAssetIDs[0] != was {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if now.SlideAssetIDs[0] == was {
		t.Fatal("the slide was never redrawn from the edited prompt")
	}
	if now.SlideAssetIDs[1] != target.SlideAssetIDs[1] {
		t.Fatal("the sibling slide was redrawn too; only one slide task should have run")
	}

	// What the slide fed keeps its artifact and is flagged, exactly as re-running
	// from the task table would leave it.
	var tasks struct {
		Tasks []struct {
			Kind    string `json:"kind"`
			Ordinal int    `json:"ordinal"`
			Index   int    `json:"index"`
			State   string `json:"state"`
			Stale   bool   `json:"stale"`
		} `json:"tasks"`
	}
	h.json(http.MethodGet, "/api/videos/DSS-1/tasks", nil, http.StatusOK, &tasks)
	for _, task := range tasks.Tasks {
		switch {
		case task.Kind == "clip" && task.Ordinal == 2:
			if !task.Stale {
				t.Fatal("the clip built from the old slide is not flagged stale")
			}
		case task.Kind == "slide" && task.Ordinal == 2 && task.Index == 0:
			if task.Stale {
				t.Fatal("the redrawn slide is flagged stale rather than reset")
			}
		}
	}

	// Both halves of the input are checked at the boundary, and neither runs
	// anything: a prompt with no text, and an index the graph has no task for.
	resp, _ := h.do(http.MethodPost, "/api/chapters/"+target.ID+"/slides/0/generate",
		map[string]any{"prompt": "  "})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("empty prompt = %d, want 422", resp.StatusCode)
	}
	resp, _ = h.do(http.MethodPost, "/api/chapters/"+target.ID+"/slides/9/generate",
		map[string]any{"prompt": "a ninth slide that has no task"})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("out-of-range index = %d, want 422", resp.StatusCode)
	}
}

// The same edit-and-generate loop on the thumbnail grid. The tail here is short
// but it is the one the operator is judging: the composed thumbnail carries the
// upload gate, so redrawing a cell reopens the publish decision.
func TestEditingACellPromptRedrawsOneIcon(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	createVideo(h, 2, 1, true)
	h.approve("DSS-1", "blueprint", 15*time.Second)
	h.waitForGate("DSS-1", "upload", 30*time.Second)

	type planCell struct {
		Caption string `json:"caption"`
		Prompt  string `json:"prompt"`
	}
	type thumbBody struct {
		ThumbnailPlan    []planCell `json:"thumbnailPlan"`
		ThumbnailIconIDs []string   `json:"thumbnailIconIds"`
		ThumbnailAssetID string     `json:"thumbnailAssetId"`
	}
	var before thumbBody
	h.json(http.MethodGet, "/api/videos/DSS-1", nil, http.StatusOK, &before)
	if len(before.ThumbnailPlan) != testThumbnailCells {
		t.Fatalf("plan has %d cells, want %d", len(before.ThumbnailPlan), testThumbnailCells)
	}
	if len(before.ThumbnailIconIDs) != testThumbnailCells {
		t.Fatalf("icon ids has %d slots, want %d", len(before.ThumbnailIconIDs), testThumbnailCells)
	}

	const cell = 2
	const prompt = "a brass ship's bell, side on"
	var edited thumbBody
	h.json(http.MethodPost, "/api/videos/DSS-1/thumbnail/cells/2/generate",
		map[string]any{"prompt": prompt}, http.StatusOK, &edited)
	if edited.ThumbnailPlan[cell].Prompt != prompt {
		t.Fatalf("cell %d prompt = %q, want the edit", cell, edited.ThumbnailPlan[cell].Prompt)
	}
	// The caption belongs to the plan; editing what a cell pictures leaves what it
	// says alone.
	if edited.ThumbnailPlan[cell].Caption != before.ThumbnailPlan[cell].Caption {
		t.Fatalf("caption became %q, was %q",
			edited.ThumbnailPlan[cell].Caption, before.ThumbnailPlan[cell].Caption)
	}
	if edited.ThumbnailPlan[0].Prompt != before.ThumbnailPlan[0].Prompt {
		t.Fatal("a neighbouring cell's prompt was overwritten")
	}

	deadline := time.Now().Add(30 * time.Second)
	var now thumbBody
	for time.Now().Before(deadline) {
		h.json(http.MethodGet, "/api/videos/DSS-1", nil, http.StatusOK, &now)
		if now.ThumbnailIconIDs[cell] != "" && now.ThumbnailIconIDs[cell] != before.ThumbnailIconIDs[cell] {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if now.ThumbnailIconIDs[cell] == before.ThumbnailIconIDs[cell] {
		t.Fatal("the icon was never redrawn from the edited prompt")
	}
	if now.ThumbnailIconIDs[0] != before.ThumbnailIconIDs[0] {
		t.Fatal("a sibling icon was redrawn too; only one icon task should have run")
	}

	var tasks struct {
		Tasks []struct {
			Kind  string `json:"kind"`
			Index int    `json:"index"`
			State string `json:"state"`
			Stale bool   `json:"stale"`
		} `json:"tasks"`
	}
	h.json(http.MethodGet, "/api/videos/DSS-1/tasks", nil, http.StatusOK, &tasks)
	for _, task := range tasks.Tasks {
		switch {
		case task.Kind == "thumbnail":
			if !task.Stale {
				t.Fatal("the composed thumbnail is not flagged stale")
			}
		case task.Kind == "thumbnail_icon" && task.Index == cell:
			if task.Stale {
				t.Fatal("the redrawn icon is flagged stale rather than reset")
			}
		}
	}

	resp, _ := h.do(http.MethodPost, "/api/videos/DSS-1/thumbnail/cells/2/generate",
		map[string]any{"prompt": " "})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("empty prompt = %d, want 422", resp.StatusCode)
	}
	resp, _ = h.do(http.MethodPost, "/api/videos/DSS-1/thumbnail/cells/99/generate",
		map[string]any{"prompt": "a cell the grid does not have"})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("out-of-range cell = %d, want 422", resp.StatusCode)
	}
}

func TestSSEStreamsDeltas(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.server.URL+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := h.server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q", got)
	}

	events := make(chan string, 256)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				select {
				case events <- strings.TrimPrefix(line, "data: "):
				default:
				}
			}
		}
	}()

	createVideo(h, 3, 2, true)

	sawTask, sawVideo := false, false
	var lastID uint64
	deadline := time.After(20 * time.Second)
	for !sawTask || !sawVideo {
		select {
		case raw := <-events:
			var ev entity.Event
			if err := json.Unmarshal([]byte(raw), &ev); err != nil {
				t.Fatalf("bad event payload: %v (%s)", err, raw)
			}
			if ev.ID <= lastID && ev.ID != 0 {
				t.Fatalf("event ids are not monotonic: %d after %d", ev.ID, lastID)
			}
			lastID = ev.ID
			if len(ev.Tasks) > 0 {
				sawTask = true
			}
			if ev.Video != nil {
				sawVideo = true
			}
		case <-deadline:
			t.Fatalf("timed out; sawTask=%v sawVideo=%v", sawTask, sawVideo)
		}
	}

	// A reconnecting client resumes from Last-Event-ID rather than reloading.
	replay, complete := h.broker.Since(0)
	if !complete {
		t.Fatal("the replay buffer reported a gap it should not have")
	}
	if len(replay) == 0 {
		t.Fatal("no events were buffered for replay")
	}
	if h.broker.Subscribers() == 0 {
		t.Fatal("the broker lost track of its subscriber")
	}
}

// A pool limit is a settings row, changeable at runtime without a restart.
func TestSettingsApplyWithoutRestart(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	var updated struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	h.json(http.MethodPut, "/api/settings/pool.image.limit", map[string]any{"value": "6"}, http.StatusOK, &updated)
	if updated.Value != "6" {
		t.Fatalf("value = %q", updated.Value)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var status struct {
			Pools []struct {
				Pool  string `json:"pool"`
				Limit int    `json:"limit"`
			} `json:"pools"`
		}
		h.json(http.MethodGet, "/api/scheduler", nil, http.StatusOK, &status)
		for _, p := range status.Pools {
			if p.Pool == "image" && p.Limit == 6 {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the new pool limit never reached the scheduler")
}

// A preset is one click that moves several provider rows at once, and the route
// that does it lives under the settings it writes.
func TestSettingsPresets(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	var listed struct {
		Presets []struct {
			Name        string `json:"name"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Values      []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			} `json:"values"`
		} `json:"presets"`
	}
	h.json(http.MethodGet, "/api/settings/presets", nil, http.StatusOK, &listed)
	if len(listed.Presets) == 0 {
		t.Fatal("no presets were listed")
	}
	for _, p := range listed.Presets {
		if p.Title == "" || len(p.Values) == 0 {
			t.Errorf("preset %q arrived empty", p.Name)
		}
	}

	var applied struct {
		Settings []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"settings"`
	}
	h.json(http.MethodPost, "/api/settings/presets/sample/apply", nil, http.StatusOK, &applied)
	if len(applied.Settings) == 0 {
		t.Fatal("applying sample over the seeded mocks changed nothing")
	}

	// Only the rows that moved come back, so the second application is empty
	// rather than a rewrite of every row the preset names.
	h.json(http.MethodPost, "/api/settings/presets/sample/apply", nil, http.StatusOK, &applied)
	if len(applied.Settings) != 0 {
		t.Errorf("re-applying the preset in force changed %d rows", len(applied.Settings))
	}

	// And it really landed in the table the settings screen reads.
	var table struct {
		Settings []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"settings"`
	}
	h.json(http.MethodGet, "/api/settings", nil, http.StatusOK, &table)
	for _, row := range table.Settings {
		if row.Key == "provider.tts" && row.Value != "sample" {
			t.Errorf("provider.tts = %q, want sample", row.Value)
		}
	}

	resp, _ := h.do(http.MethodPost, "/api/settings/presets/nonesuch/apply", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSettingsRejectBadValues(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	resp, _ := h.do(http.MethodPut, "/api/settings/pool.image.limit", map[string]any{"value": "banana"})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
	resp, _ = h.do(http.MethodPut, "/api/settings/no.such.key", map[string]any{"value": "1"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// Mutations are idempotent by request key.
func TestIdempotencyKeyReplaysTheFirstResponse(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	post := func() (int, videoBody, string) {
		body, err := json.Marshal(map[string]any{
			"channel": "deep-sleep-stories", "title": "Once only", "chapterCount": 2, "slidesPerChapter": 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, h.server.URL+"/api/videos", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "abc-123")
		resp, err := h.server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		var v videoBody
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("decode: %v (%s)", err, raw)
		}
		return resp.StatusCode, v, resp.Header.Get("Idempotent-Replay")
	}

	status1, first, replay1 := post()
	if status1 != http.StatusCreated {
		t.Fatalf("first POST = %d", status1)
	}
	if replay1 != "" {
		t.Fatal("the first request was marked as a replay")
	}
	status2, second, replay2 := post()
	if status2 != http.StatusCreated {
		t.Fatalf("second POST = %d", status2)
	}
	if replay2 != "true" {
		t.Fatal("the repeated request was not marked as a replay")
	}
	if first.ID != second.ID || first.Ref != second.Ref {
		t.Fatalf("the key minted a second video: %s vs %s", first.Ref, second.Ref)
	}

	var list struct {
		Total int `json:"total"`
	}
	h.json(http.MethodGet, "/api/videos", nil, http.StatusOK, &list)
	if list.Total != 1 {
		t.Fatalf("videos = %d, want 1", list.Total)
	}
}

func TestChannelCRUD(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	var created struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	h.json(http.MethodPost, "/api/channels", map[string]any{
		"name": "Quiet Rivers",
	}, http.StatusCreated, &created)
	if created.Slug != "quiet-rivers" {
		t.Fatalf("slug = %q, want quiet-rivers (derived from the name)", created.Slug)
	}

	// Both keys resolve.
	var bySlug, byID struct {
		ID string `json:"id"`
	}
	h.json(http.MethodGet, "/api/channels/quiet-rivers", nil, http.StatusOK, &bySlug)
	h.json(http.MethodGet, "/api/channels/"+created.ID, nil, http.StatusOK, &byID)
	if bySlug.ID != byID.ID {
		t.Fatal("slug and id resolved different channels")
	}

	// The slug is immutable: renaming does not touch it.
	var updated struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	h.json(http.MethodPut, "/api/channels/quiet-rivers", map[string]any{"name": "Loud Rivers"}, http.StatusOK, &updated)
	if updated.Slug != "quiet-rivers" {
		t.Fatalf("slug changed to %q on rename", updated.Slug)
	}
	if updated.Name != "Loud Rivers" {
		t.Fatalf("name = %q", updated.Name)
	}

	// A duplicate slug is a conflict.
	resp, _ := h.do(http.MethodPost, "/api/channels", map[string]any{"slug": "quiet-rivers", "name": "Another"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate slug = %d, want 409", resp.StatusCode)
	}

	h.json(http.MethodDelete, "/api/channels/quiet-rivers", nil, http.StatusNoContent, nil)
	resp, _ = h.do(http.MethodGet, "/api/channels/quiet-rivers", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("deleted channel = %d, want 404", resp.StatusCode)
	}
}

func TestAPIErrorsAreMapped(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	cases := []struct {
		method, path string
		body         any
		want         int
	}{
		{http.MethodGet, "/api/videos/DSS-999", nil, http.StatusNotFound},
		{http.MethodDelete, "/api/videos/DSS-999", nil, http.StatusNotFound},
		{http.MethodGet, "/api/channels/no-such-channel", nil, http.StatusNotFound},
		{http.MethodGet, "/api/tasks/nope", nil, http.StatusNotFound},
		{http.MethodPost, "/api/videos", map[string]any{"channel": "deep-sleep-stories"}, http.StatusUnprocessableEntity},
		{http.MethodPost, "/api/videos", map[string]any{"channel": "nope", "title": "x"}, http.StatusNotFound},
		{http.MethodGet, "/assets/not-a-hash", nil, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			t.Parallel()
			resp, body := h.do(tc.method, tc.path, tc.body)
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d: %s", resp.StatusCode, tc.want, body)
			}
		})
	}
}

func TestOpenAPIDocumentIsGenerated(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	resp, body := h.do(http.MethodGet, "/api/openapi.json", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	var doc struct {
		OpenAPI string                    `json:"openapi"`
		Paths   map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc.OpenAPI == "" {
		t.Fatal("no openapi version in the document")
	}
	for _, want := range []string{"/api/videos", "/api/videos/{key}", "/api/channels", "/api/settings", "/api/scheduler"} {
		if _, ok := doc.Paths[want]; !ok {
			t.Errorf("path %q is missing from the OpenAPI document", want)
		}
	}
}

func TestHealthAndSchedulerConsole(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	var health struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	h.json(http.MethodGet, "/api/health", nil, http.StatusOK, &health)
	if health.Status != "ok" || health.Version != "test" {
		t.Fatalf("health = %+v", health)
	}

	var status struct {
		Pools []struct {
			Pool  string `json:"pool"`
			Limit int    `json:"limit"`
		} `json:"pools"`
	}
	h.json(http.MethodGet, "/api/scheduler", nil, http.StatusOK, &status)
	if len(status.Pools) != entity.NumPools {
		t.Fatalf("pools = %d, want %d", len(status.Pools), entity.NumPools)
	}
}

// The API response budget is p99 under 50 ms.
func TestAPIResponseLatencyBudget(t *testing.T) {
	// Not parallel: the budget describes a single-operator deployment, and the
	// other integration tests each run their own SQLite and scheduler. Measured
	// alongside them this reports contention, not the API.
	h := newHarness(t)
	createVideo(h, 10, 2, false)

	const samples = 200
	durations := make([]time.Duration, 0, samples)
	for range samples {
		start := time.Now()
		resp, _ := h.do(http.MethodGet, "/api/videos/DSS-1", nil)
		durations = append(durations, time.Since(start))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", resp.StatusCode)
		}
	}
	for i := 1; i < len(durations); i++ {
		d := durations[i]
		j := i - 1
		for j >= 0 && durations[j] > d {
			durations[j+1] = durations[j]
			j--
		}
		durations[j+1] = d
	}
	p99 := durations[int(float64(len(durations))*0.99)-1]
	if p99 > 50*time.Millisecond {
		t.Fatalf("API p99 = %s, budget is 50ms", p99)
	}
	t.Logf("API p99 = %s (median %s)", p99, durations[len(durations)/2])
}

// assetListing is a video's artifacts as the API reports them.
type assetListing struct {
	Assets []struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
	} `json:"assets"`
}

func (h *harness) fileExists(id, kind string) bool {
	h.t.Helper()
	path := filepath.Join(h.assets.Root(), assetstore.RelPath(entity.AssetID(id), entity.AssetKind(kind)))
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return true
	case os.IsNotExist(err):
		return false
	default:
		h.t.Fatalf("stat %s: %v", path, err)
		return false
	}
}

// Deleting a video takes its rows, its running work and its files with it. The
// video here is parked at the upload gate with a live graph in the scheduler,
// which is the state an operator is most likely to delete from.
func TestDeleteVideoRemovesEverythingItOwns(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	createVideo(h, 3, 2, true)
	h.approve("DSS-1", "blueprint", 15*time.Second)
	h.waitForGate("DSS-1", "upload", 30*time.Second)

	var produced assetListing
	h.json(http.MethodGet, "/api/videos/DSS-1/assets", nil, http.StatusOK, &produced)
	if len(produced.Assets) < 10 {
		t.Fatalf("the video only produced %d artifacts; the test needs it to have worked", len(produced.Assets))
	}
	first := produced.Assets[0]
	if resp, _ := h.do(http.MethodGet, "/assets/"+first.ID, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("artifact is not served before the delete: %d", resp.StatusCode)
	}

	h.json(http.MethodDelete, "/api/videos/DSS-1", nil, http.StatusNoContent, nil)

	for _, path := range []string{
		"/api/videos/DSS-1",
		"/api/videos/DSS-1/tasks",
		"/api/videos/DSS-1/chapters",
		"/api/videos/DSS-1/assets",
	} {
		if resp, _ := h.do(http.MethodGet, path, nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d after the delete, want 404", path, resp.StatusCode)
		}
	}

	// Nothing else references these bytes, so every one of them is reclaimed:
	// both the row that served them and the file behind it.
	for _, a := range produced.Assets {
		if resp, _ := h.do(http.MethodGet, "/assets/"+a.ID, nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET /assets/%s = %d after the delete, want 404", a.ID[:12], resp.StatusCode)
		}
		if h.fileExists(a.ID, a.Kind) {
			t.Errorf("%s artifact %s was left on disk", a.Kind, a.ID[:12])
		}
	}

	// Deleting it again is a not-found, not a second success.
	if resp, _ := h.do(http.MethodDelete, "/api/videos/DSS-1", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("second delete = %d, want 404", resp.StatusCode)
	}

	// The list is empty rather than holding a row that points at nothing.
	var list struct {
		Videos []videoBody `json:"videos"`
		Total  int         `json:"total"`
	}
	h.json(http.MethodGet, "/api/videos", nil, http.StatusOK, &list)
	if list.Total != 0 || len(list.Videos) != 0 {
		t.Errorf("videos after the delete = %d (total %d)", len(list.Videos), list.Total)
	}
}

// A channel takes its videos' files with it too: the cascade goes through the
// same per-video path rather than leaving the disk to the foreign key.
func TestDeleteChannelReclaimsItsVideosFiles(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	createVideo(h, 2, 1, true)
	h.approve("DSS-1", "blueprint", 15*time.Second)
	h.waitForGate("DSS-1", "upload", 30*time.Second)

	var produced assetListing
	h.json(http.MethodGet, "/api/videos/DSS-1/assets", nil, http.StatusOK, &produced)
	if len(produced.Assets) == 0 {
		t.Fatal("the video produced no artifacts")
	}

	h.json(http.MethodDelete, "/api/channels/deep-sleep-stories", nil, http.StatusNoContent, nil)

	if resp, _ := h.do(http.MethodGet, "/api/videos/DSS-1", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("the channel's video survived: %d", resp.StatusCode)
	}
	for _, a := range produced.Assets {
		if h.fileExists(a.ID, a.Kind) {
			t.Errorf("%s artifact %s was left on disk", a.Kind, a.ID[:12])
		}
	}
}

// With the blueprint gate off there is nobody to ask, so acceptance is the task
// succeeding: the blueprint expands its own video's DAG before reporting, and
// the pipeline runs straight through.
//
// This is the one path that goes through the real TaskRunner rather than a test
// double, which is what makes it worth an end-to-end run.
func TestUngatedBlueprintExpandsItsOwnGraph(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	h.json(http.MethodPut, "/api/settings/gate.blueprint.enabled",
		map[string]any{"value": "false"}, http.StatusOK, nil)
	h.json(http.MethodPut, "/api/settings/gate.upload.enabled",
		map[string]any{"value": "false"}, http.StatusOK, nil)

	v := createVideo(h, 4, 1, true)
	if v.Counts.Total != 1 {
		t.Fatalf("tasks at enqueue = %d, want 1", v.Counts.Total)
	}

	done := h.waitForState(v.Ref, "completed", 30*time.Second)
	if done.Counts.Total != scheduler.NodeCountFor(4, 1, testThumbnailCells) {
		t.Fatalf("tasks = %d, want %d: the graph did not expand", done.Counts.Total, scheduler.NodeCountFor(4, 1, testThumbnailCells))
	}
	if done.Counts.Succeeded != done.Counts.Total {
		t.Fatalf("succeeded %d of %d", done.Counts.Succeeded, done.Counts.Total)
	}
	if done.Upload == nil {
		t.Fatal("the video completed without an upload receipt")
	}
}

// An accepted blueprint cannot be run again: the whole DAG below it is built
// from the chapters it produced, and expansion is one-way.
func TestAcceptedBlueprintCannotBeRetried(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	v := createVideo(h, 3, 1, true)
	h.approve(v.Ref, "blueprint", 15*time.Second)

	var tasks struct {
		Tasks []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"tasks"`
	}
	h.json(http.MethodGet, "/api/videos/"+v.Ref+"/tasks", nil, http.StatusOK, &tasks)
	var blueprintID string
	for _, task := range tasks.Tasks {
		if task.Kind == "blueprint" {
			blueprintID = task.ID
		}
	}
	if blueprintID == "" {
		t.Fatal("the video has no blueprint task")
	}

	resp, body := h.do(http.MethodPost, "/api/tasks/"+blueprintID+"/retry", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("retry of an accepted blueprint = %d, want 409: %s", resp.StatusCode, body)
	}
}
