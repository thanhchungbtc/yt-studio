package sqlite_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tbui/yt-studio/adapters/sqlite"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
	"github.com/tbui/yt-studio/domain/scheduler"
)

func newStore(t *testing.T) *sqlite.Store {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	store, err := sqlite.Open(context.Background(), sqlite.Options{
		Path: filepath.Join(t.TempDir(), "test.db"),
	}, log)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = store.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
		_ = store.Close()
	})
	return store
}

func seeded(t *testing.T) (*sqlite.Store, entity.Channel) {
	t.Helper()
	store := newStore(t)
	ctx := context.Background()
	if err := sqlite.SeedSettings(ctx, store); err != nil {
		t.Fatalf("SeedSettings: %v", err)
	}
	if err := sqlite.SeedChannels(ctx, store, time.Unix(0, 0).UTC()); err != nil {
		t.Fatalf("SeedChannels: %v", err)
	}
	ch, err := store.ChannelBySlug(ctx, "deep-sleep-stories")
	if err != nil {
		t.Fatalf("ChannelBySlug: %v", err)
	}
	return store, ch
}

// A fresh database and a ten-times-seeded database must end up in the same
// state.
func TestSeedIsIdempotent(t *testing.T) {
	t.Parallel()
	store, _ := seeded(t)
	ctx := context.Background()

	before, err := store.ListChannels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settingsBefore, err := store.ListSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}

	for range 10 {
		if err := sqlite.SeedSettings(ctx, store); err != nil {
			t.Fatalf("SeedSettings: %v", err)
		}
		if err := sqlite.SeedChannels(ctx, store, time.Unix(0, 0).UTC()); err != nil {
			t.Fatalf("SeedChannels: %v", err)
		}
	}

	after, err := store.ListChannels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("channels = %d after re-seeding, want %d", len(after), len(before))
	}
	for i := range after {
		if after[i].ID != before[i].ID || after[i].Slug != before[i].Slug {
			t.Fatalf("channel %d changed identity across seeds", i)
		}
	}
	settingsAfter, err := store.ListSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(settingsAfter) != len(settingsBefore) {
		t.Fatalf("settings = %d after re-seeding, want %d", len(settingsAfter), len(settingsBefore))
	}
}

// An operator's edit must survive a re-seed: the seed refreshes metadata, not
// values.
func TestSeedPreservesOperatorValues(t *testing.T) {
	t.Parallel()
	store, _ := seeded(t)
	ctx := context.Background()

	if _, err := store.UpdateSetting(ctx, entity.SettingPoolImageLimit, "9"); err != nil {
		t.Fatalf("UpdateSetting: %v", err)
	}
	if err := sqlite.SeedSettings(ctx, store); err != nil {
		t.Fatalf("SeedSettings: %v", err)
	}
	got, err := store.SettingByKey(ctx, entity.SettingPoolImageLimit)
	if err != nil {
		t.Fatal(err)
	}
	if got.Value != "9" {
		t.Fatalf("pool.image.limit = %q after re-seed, want the operator's 9", got.Value)
	}
}

func TestSettingValidationRejectsBadValues(t *testing.T) {
	t.Parallel()
	store, _ := seeded(t)
	ctx := context.Background()

	if _, err := store.UpdateSetting(ctx, entity.SettingPoolImageLimit, "banana"); !errors.Is(err, entity.ErrInvalidSetting) {
		t.Fatalf("err = %v, want ErrInvalidSetting", err)
	}
	if _, err := store.UpdateSetting(ctx, entity.SettingPoolImageLimit, "9999"); !errors.Is(err, entity.ErrInvalidSetting) {
		t.Fatalf("out-of-range value accepted: %v", err)
	}
	if _, err := store.UpdateSetting(ctx, "no.such.key", "1"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestChannelSlugIsUnique(t *testing.T) {
	t.Parallel()
	store, ch := seeded(t)
	ctx := context.Background()

	dup, err := entity.NewChannel("other-id", ch.Slug, "Duplicate", entity.StyleConfig{}, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateChannel(ctx, dup); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

func TestVideoRefsAreMintedInSequence(t *testing.T) {
	t.Parallel()
	store, ch := seeded(t)
	ctx := context.Background()

	for want := 1; want <= 5; want++ {
		seq, err := store.NextVideoSeq(ctx, ch.ID)
		if err != nil {
			t.Fatalf("NextVideoSeq: %v", err)
		}
		if seq != want {
			t.Fatalf("seq = %d, want %d", seq, want)
		}
	}
}

func TestVideoLifecyclePersistence(t *testing.T) {
	t.Parallel()
	store, ch := seeded(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	v, err := entity.NewVideo("v1", ch.ID, "DSS-1", "Title", "topic", 3, 2, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateVideo(ctx, v); err != nil {
		t.Fatalf("CreateVideo: %v", err)
	}

	byID, err := store.VideoByID(ctx, "v1")
	if err != nil {
		t.Fatal(err)
	}
	byRef, err := store.VideoByRef(ctx, "DSS-1")
	if err != nil {
		t.Fatal(err)
	}
	if byID.ID != byRef.ID {
		t.Fatal("both keys must resolve the same video")
	}

	if err := store.SetVideoState(ctx, "v1", entity.VideoStateRunning, ""); err != nil {
		t.Fatal(err)
	}
	got, err := store.VideoByID(ctx, "v1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != entity.VideoStateRunning {
		t.Fatalf("state = %q", got.State)
	}
	if got.StartedAt == nil {
		t.Fatal("started_at was not stamped on the first non-draft transition")
	}
	firstStart := *got.StartedAt

	if err := store.SetVideoState(ctx, "v1", entity.VideoStateCompleted, ""); err != nil {
		t.Fatal(err)
	}
	got, err = store.VideoByID(ctx, "v1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.StartedAt.Equal(firstStart) {
		t.Fatal("started_at moved on a later transition")
	}
	if got.CompletedAt == nil {
		t.Fatal("completed_at was not stamped on a terminal state")
	}

	md := entity.Metadata{Title: "T", Description: "D", Tags: []string{"a"}, Privacy: "private"}
	if err := store.SetVideoMetadata(ctx, "v1", md); err != nil {
		t.Fatal(err)
	}
	if err := store.SetVideoThumbnailAsset(ctx, "v1", "sha256:thumb"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetVideoUpload(ctx, "v1", entity.UploadRecord{VideoID: "yt", URL: "u", DryRun: true, UploadedAt: now}); err != nil {
		t.Fatal(err)
	}
	got, err = store.VideoByID(ctx, "v1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Metadata == nil || got.Metadata.Title != "T" {
		t.Fatalf("metadata = %+v", got.Metadata)
	}
	if got.Upload == nil || !got.Upload.DryRun {
		t.Fatalf("upload = %+v", got.Upload)
	}
	if got.ThumbnailAssetID == nil || *got.ThumbnailAssetID != "sha256:thumb" {
		t.Fatalf("thumbnail asset = %v", got.ThumbnailAssetID)
	}
}

// Two image tasks for the same chapter write concurrently; each must land
// without clobbering the other.
func TestSetChapterImageIsIndexed(t *testing.T) {
	t.Parallel()
	store, ch := seeded(t)
	ctx := context.Background()
	now := time.Unix(0, 0).UTC()

	v, err := entity.NewVideo("v1", ch.ID, "DSS-1", "Title", "", 1, 3, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateVideo(ctx, v); err != nil {
		t.Fatal(err)
	}
	c, err := entity.NewChapter("v1", 1, "Chapter", "", now)
	if err != nil {
		t.Fatal(err)
	}
	c.ImageAssetIDs = make([]entity.AssetID, 3)
	if err := store.ReplaceChapters(ctx, "v1", []entity.Chapter{c}); err != nil {
		t.Fatal(err)
	}

	// Deliberately out of order, as concurrent tasks would arrive.
	if err := store.SetChapterImage(ctx, c.ID, 2, "cc"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetChapterImage(ctx, c.ID, 0, "aa"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetChapterImage(ctx, c.ID, 1, "bb"); err != nil {
		t.Fatal(err)
	}

	got, err := store.ChapterByID(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []entity.AssetID{"aa", "bb", "cc"}
	if len(got.ImageAssetIDs) != len(want) {
		t.Fatalf("images = %v, want %v", got.ImageAssetIDs, want)
	}
	for i := range want {
		if got.ImageAssetIDs[i] != want[i] {
			t.Fatalf("images = %v, want %v", got.ImageAssetIDs, want)
		}
	}
}

func TestGraphPersistenceRoundTrip(t *testing.T) {
	t.Parallel()
	store, ch := seeded(t)
	ctx := context.Background()
	now := time.Unix(0, 0).UTC()

	v, err := entity.NewVideo("v1", ch.ID, "DSS-1", "Title", "", 4, 2, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateVideo(ctx, v); err != nil {
		t.Fatal(err)
	}
	g, err := scheduler.BuildGraph(scheduler.BuildSpec{
		VideoID: "v1", ChapterCount: 4, ImagesPerChapter: 2, MaxAttempts: 3,
		BlueprintGate: true, UploadGate: true, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.InsertGraph(ctx, "v1", g.Tasks(), g.Edges()); err != nil {
		t.Fatalf("InsertGraph: %v", err)
	}
	// Idempotent: re-inserting the same deterministic ids changes nothing.
	if err := store.InsertGraph(ctx, "v1", g.Tasks(), g.Edges()); err != nil {
		t.Fatalf("InsertGraph twice: %v", err)
	}

	graphs, err := store.ListOpenGraphs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(graphs) != 1 {
		t.Fatalf("open graphs = %d, want 1", len(graphs))
	}
	if len(graphs[0].Tasks) != g.NodeCount() {
		t.Fatalf("tasks = %d, want %d", len(graphs[0].Tasks), g.NodeCount())
	}
	if len(graphs[0].Edges) != len(g.Edges()) {
		t.Fatalf("edges = %d, want %d", len(graphs[0].Edges), len(g.Edges()))
	}
	restored, err := scheduler.GraphFromPersisted(graphs[0])
	if err != nil {
		t.Fatalf("GraphFromPersisted: %v", err)
	}
	if restored.NodeCount() != g.NodeCount() {
		t.Fatal("restored graph has a different shape")
	}

	// A batch of transitions commits in one call.
	batch := make([]repository.TaskTransition, 0, g.NodeCount())
	for i := range g.NodeCount() {
		batch = append(batch, repository.TaskTransition{
			ID: g.Task(i).ID, State: entity.TaskStateSucceeded, UpdatedAt: now,
		})
	}
	if err := store.ApplyTransitions(ctx, batch); err != nil {
		t.Fatalf("ApplyTransitions: %v", err)
	}
	counts, err := store.CountTasksByVideo(ctx, "v1")
	if err != nil {
		t.Fatal(err)
	}
	if counts.Succeeded != g.NodeCount() {
		t.Fatalf("succeeded = %d, want %d", counts.Succeeded, g.NodeCount())
	}
	// Nothing is open any more, so nothing needs resuming.
	graphs, err = store.ListOpenGraphs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(graphs) != 0 {
		t.Fatalf("open graphs = %d after completion, want 0", len(graphs))
	}
}

// No scheduler query may fall back to a full table scan.
func TestSchedulerQueriesUseIndexes(t *testing.T) {
	t.Parallel()
	store, _ := seeded(t)
	cases := []struct {
		name  string
		query string
		args  []any
	}{
		{"tasks by video", "SELECT * FROM tasks WHERE video_id = ? ORDER BY ordinal, idx, kind", []any{"v1"}},
		{"tasks by video and state", "SELECT * FROM tasks WHERE video_id = ? AND state = ?", []any{"v1", "ready"}},
		{"open videos", "SELECT DISTINCT video_id FROM tasks WHERE state IN ('blocked','ready','running','awaiting_approval')", nil},
		{"task by id", "SELECT * FROM tasks WHERE id = ?", []any{"t1"}},
		{"deps by video", "SELECT video_id, from_id, to_id FROM task_deps WHERE video_id = ?", []any{"v1"}},
		{"recent tasks", "SELECT * FROM tasks ORDER BY updated_at DESC LIMIT 10", nil},
		{"chapters by video", "SELECT * FROM chapters WHERE video_id = ? ORDER BY ordinal", []any{"v1"}},
		{"videos by channel", "SELECT * FROM videos WHERE channel_id = ? ORDER BY created_at DESC LIMIT 10", []any{"c1"}},
		{"assets by video", "SELECT * FROM assets WHERE video_id = ?", []any{"v1"}},
		{"channel by slug", "SELECT * FROM channels WHERE slug = ?", []any{"deep-sleep-stories"}},
		{"video by ref", "SELECT * FROM videos WHERE ref = ?", []any{"DSS-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rows, err := store.ReadDB().QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+tc.query, tc.args...)
			if err != nil {
				t.Fatalf("EXPLAIN: %v", err)
			}
			defer func() { _ = rows.Close() }()

			var plan []string
			for rows.Next() {
				var id, parent, notused int
				var detail string
				if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
					t.Fatalf("scan: %v", err)
				}
				plan = append(plan, detail)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			for _, line := range plan {
				upper := strings.ToUpper(line)
				// "SCAN <table>" without an index is the failure mode; a scan of a subquery
				// or a temp b-tree is not.
				if strings.HasPrefix(upper, "SCAN ") && !strings.Contains(upper, "USING") {
					t.Fatalf("query plan does a full scan: %q\nfull plan: %v", line, plan)
				}
			}
			t.Logf("plan: %v", plan)
		})
	}
}

// Migrations must be fast on an existing database.
func TestMigrationsAreFastOnAnExistingDatabase(t *testing.T) {
	// Not parallel: goose serialises migrations behind a package mutex, so a
	// concurrent test would measure queueing rather than the migration.
	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	path := filepath.Join(t.TempDir(), "test.db")

	first, err := sqlite.Open(context.Background(), sqlite.Options{Path: path}, log)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	second, err := sqlite.Open(context.Background(), sqlite.Options{Path: path}, log)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	// The budget covers migrations alone; opening two pools and preparing every
	// statement is measured with it, so the bound is generous.
	if elapsed > 250*time.Millisecond {
		t.Fatalf("reopening an existing database took %s", elapsed)
	}
	t.Logf("open + migrate on existing database: %s", elapsed)
}

func TestNotFoundIsDistinguishable(t *testing.T) {
	t.Parallel()
	store, _ := seeded(t)
	ctx := context.Background()

	if _, err := store.ChannelByID(ctx, "nope"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if _, err := store.VideoByRef(ctx, "XXX-1"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if _, err := store.TaskByID(ctx, "nope"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
