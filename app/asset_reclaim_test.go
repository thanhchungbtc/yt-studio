package app_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tbui/yt-studio/adapters/assetstore"
	"github.com/tbui/yt-studio/adapters/sqlite"
	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// These tests are about one question: which files may a delete unlink? The
// answer depends on the store and the database agreeing, so both are real here
// and only the scheduler is stubbed.

var testTime = time.Unix(1_700_000_000, 0).UTC()

// forgetter stands in for the scheduler. Deleting a video has to tell it first;
// what it does with that is tested with the scheduler.
type forgetter struct{ forgotten []entity.VideoID }

func (f *forgetter) Forget(_ context.Context, id entity.VideoID) error {
	f.forgotten = append(f.forgotten, id)
	return nil
}

type fixture struct {
	t       *testing.T
	store   *sqlite.Store
	assets  *assetstore.FS
	root    string
	channel entity.Channel
	forget  *forgetter
	log     *slog.Logger
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	store, err := sqlite.Open(context.Background(), sqlite.Options{
		Path: filepath.Join(t.TempDir(), "test.db"),
	}, log)
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
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

	if err := sqlite.SeedSettings(context.Background(), store); err != nil {
		t.Fatalf("SeedSettings: %v", err)
	}
	if err := sqlite.SeedChannels(context.Background(), store, testTime); err != nil {
		t.Fatalf("SeedChannels: %v", err)
	}
	channel, err := store.ChannelBySlug(context.Background(), "deep-sleep-stories")
	if err != nil {
		t.Fatalf("ChannelBySlug: %v", err)
	}

	root := filepath.Join(t.TempDir(), "assets")
	assets, err := assetstore.New(root)
	if err != nil {
		t.Fatalf("assetstore.New: %v", err)
	}

	return &fixture{
		t: t, store: store, assets: assets, root: root,
		channel: channel, forget: &forgetter{}, log: log,
	}
}

// video creates a draft with one chapter, which is enough to hang an asset off.
func (f *fixture) video(ref, title string) (entity.Video, entity.Chapter) {
	f.t.Helper()
	ctx := context.Background()

	v, err := entity.NewVideo(entity.VideoID(ref), f.channel.ID, entity.Ref(ref), title, "", 1, 1, testTime)
	if err != nil {
		f.t.Fatalf("NewVideo: %v", err)
	}
	if err := f.store.CreateVideo(ctx, v); err != nil {
		f.t.Fatalf("CreateVideo: %v", err)
	}
	c, err := entity.NewChapter(v.ID, 1, "One", "", testTime)
	if err != nil {
		f.t.Fatalf("NewChapter: %v", err)
	}
	if err := f.store.ReplaceChapters(ctx, v.ID, []entity.Chapter{c}); err != nil {
		f.t.Fatalf("ReplaceChapters: %v", err)
	}
	return v, c
}

// store writes bytes into the asset store and records one video's ownership of
// them, which is what a task does when it produces a file.
func (f *fixture) put(content string, kind entity.AssetKind, v entity.Video, chapter *entity.ChapterID) entity.AssetID {
	f.t.Helper()
	ctx := context.Background()

	stored, err := f.assets.Put(ctx, kind, bytes.NewReader([]byte(content)))
	if err != nil {
		f.t.Fatalf("Put: %v", err)
	}
	a, err := entity.NewAsset(stored.ID, v.ID, chapter, kind, stored.Path, stored.Size, "test", testTime)
	if err != nil {
		f.t.Fatalf("NewAsset: %v", err)
	}
	if err := f.store.PutAsset(ctx, a); err != nil {
		f.t.Fatalf("PutAsset: %v", err)
	}
	return stored.ID
}

func (f *fixture) stored(id entity.AssetID, kind entity.AssetKind) bool {
	f.t.Helper()
	_, err := os.Stat(filepath.Join(f.root, assetstore.RelPath(id, kind)))
	switch {
	case err == nil:
		return true
	case errors.Is(err, os.ErrNotExist):
		return false
	default:
		f.t.Fatalf("stat asset: %v", err)
		return false
	}
}

func (f *fixture) delete(key string) {
	f.t.Helper()
	if err := app.DeleteVideo(context.Background(), f.store, f.store, f.forget, f.assets, f.log, key); err != nil {
		f.t.Fatalf("DeleteVideo %s: %v", key, err)
	}
}

// The reason the assets table is keyed by (address, video) rather than by
// address alone: two videos really do produce identical bytes, and the second
// one's claim on the file has to be recorded somewhere.
func TestDeleteVideoKeepsFilesAnotherVideoShares(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	first, firstChapter := f.video("DSS-1", "First")
	second, secondChapter := f.video("DSS-2", "Second")

	// The same narration for both videos: one file, two owners.
	shared := f.put("identical narration", entity.AssetKindAudio, first, &firstChapter.ID)
	if again := f.put("identical narration", entity.AssetKindAudio, second, &secondChapter.ID); again != shared {
		t.Fatalf("identical bytes stored under two addresses: %s and %s", shared, again)
	}
	private := f.put("only the first video has this", entity.AssetKindScript, first, &firstChapter.ID)

	f.delete("DSS-1")

	if f.stored(private, entity.AssetKindScript) {
		t.Error("a file only the deleted video owned was left on disk")
	}
	if !f.stored(shared, entity.AssetKindAudio) {
		t.Fatal("a file the surviving video still references was deleted")
	}

	// The surviving owner keeps both its row and its ability to serve the file.
	ctx := context.Background()
	row, err := f.store.AssetByID(ctx, shared)
	if err != nil {
		t.Fatalf("the shared asset lost its metadata row: %v", err)
	}
	if row.VideoID != second.ID {
		t.Errorf("shared row is owned by %s, want %s", row.VideoID, second.ID)
	}
	if _, err := f.store.VideoByRef(ctx, "DSS-1"); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("VideoByRef(DSS-1) error = %v, want ErrNotFound", err)
	}
	if _, err := f.store.VideoByRef(ctx, "DSS-2"); err != nil {
		t.Errorf("the surviving video is gone: %v", err)
	}
	if len(f.forget.forgotten) != 1 || f.forget.forgotten[0] != first.ID {
		t.Errorf("forgotten = %v, want [%s]", f.forget.forgotten, first.ID)
	}
}

// Deleting a video takes its chapters and its asset rows with it.
func TestDeleteVideoRemovesWhatItOwns(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	v, chapter := f.video("DSS-1", "Only")
	id := f.put("a still", entity.AssetKindImage, v, &chapter.ID)

	f.delete("DSS-1")

	if f.stored(id, entity.AssetKindImage) {
		t.Error("the file was not reclaimed")
	}
	if _, err := f.store.AssetByID(ctx, id); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("AssetByID error = %v, want ErrNotFound", err)
	}
	chapters, err := f.store.ListChaptersByVideo(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 0 {
		t.Errorf("%d chapters survived the video", len(chapters))
	}
}

// Deleting an unknown video is a not-found, not a silent success.
func TestDeleteVideoRejectsAnUnknownKey(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	err := app.DeleteVideo(context.Background(), f.store, f.store, f.forget, f.assets, f.log, "DSS-404")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if len(f.forget.forgotten) != 0 {
		t.Error("the scheduler was told to forget a video that does not exist")
	}
}

// The repair is what makes the old data safe to delete from: it restores the
// ownership rows the address-only key could not hold.
func TestRepairAssetOwnershipRestoresMissingRows(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	first, firstChapter := f.video("DSS-1", "First")
	_, secondChapter := f.video("DSS-2", "Second")

	// Exactly the state the old schema left behind: one file, a row for the video
	// that produced it, and a second video referencing it with no row of its own.
	shared := f.put("identical narration", entity.AssetKindAudio, first, &firstChapter.ID)
	if err := f.store.SetChapterAudio(ctx, secondChapter.ID, shared); err != nil {
		t.Fatalf("SetChapterAudio: %v", err)
	}

	repaired, err := app.RepairAssetOwnership(ctx, f.store, f.store, f.store, f.assets, testTime, f.log)
	if err != nil {
		t.Fatalf("RepairAssetOwnership: %v", err)
	}
	if repaired != 1 {
		t.Fatalf("repaired %d rows, want 1", repaired)
	}

	// Once more, to prove it is a no-op rather than a growing pile of rows.
	again, err := app.RepairAssetOwnership(ctx, f.store, f.store, f.store, f.assets, testTime, f.log)
	if err != nil {
		t.Fatalf("second RepairAssetOwnership: %v", err)
	}
	if again != 0 {
		t.Errorf("second run repaired %d rows, want 0", again)
	}

	// And the point of it all: the file survives its first owner.
	f.delete("DSS-1")
	if !f.stored(shared, entity.AssetKindAudio) {
		t.Fatal("the repaired reference did not protect the file")
	}
}

// A reference to a file that is neither in the store nor in the table is left
// alone: there is nothing to own.
func TestRepairAssetOwnershipIgnoresDanglingReferences(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	_, chapter := f.video("DSS-1", "First")
	absent := entity.AssetID("ff" + strings0(62))
	if err := f.store.SetChapterAudio(ctx, chapter.ID, absent); err != nil {
		t.Fatalf("SetChapterAudio: %v", err)
	}

	repaired, err := app.RepairAssetOwnership(ctx, f.store, f.store, f.store, f.assets, testTime, f.log)
	if err != nil {
		t.Fatalf("RepairAssetOwnership: %v", err)
	}
	if repaired != 0 {
		t.Errorf("repaired %d rows for a file that does not exist, want 0", repaired)
	}
}

// strings0 returns n zero digits, for a well-formed address that is not stored.
func strings0(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = '0'
	}
	return string(b)
}

// tempFile plants debris in the staging area, as a write that died between
// streaming the bytes and renaming them into place would have left it.
func (f *fixture) tempFile(name string, age time.Duration) string {
	f.t.Helper()
	dir := filepath.Join(f.root, ".tmp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		f.t.Fatalf("mkdir .tmp: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("half a file"), 0o600); err != nil {
		f.t.Fatalf("write %s: %v", name, err)
	}
	stamp := testTime.Add(-age)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		f.t.Fatalf("chtimes %s: %v", name, err)
	}
	return path
}

func (f *fixture) sweep(opts app.SweepOptions) app.SweepReport {
	f.t.Helper()
	if opts.Now.IsZero() {
		opts.Now = testTime
	}
	report, err := app.SweepAssets(context.Background(), f.store, f.assets, opts, f.log)
	if err != nil {
		f.t.Fatalf("SweepAssets: %v", err)
	}
	return report
}

// The sweep is the other half of the reclaim story: it collects what earlier
// deletes leaked, back when nothing cleaned up at all.
func TestSweepReclaimsOnlyWhatNothingReferences(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	v, chapter := f.video("DSS-1", "First")
	referenced := f.put("a still that is still needed", entity.AssetKindImage, v, &chapter.ID)

	// A file with no row at all: exactly what a pre-cleanup delete left behind.
	stored, err := f.assets.Put(ctx, entity.AssetKindImage, bytes.NewReader([]byte("nobody owns this")))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	orphan := stored.ID
	stale := f.tempFile("put-crashed", 3*time.Hour)
	fresh := f.tempFile("put-in-flight", time.Minute)
	unknown := filepath.Join(f.root, "notes-from-a-person.txt")
	if err := os.WriteFile(unknown, []byte("do not delete me"), 0o600); err != nil {
		t.Fatal(err)
	}

	// A report first, changing nothing: this is what the CLI does by default.
	dry := f.sweep(app.SweepOptions{})
	if dry.Unreferenced != 1 || dry.Referenced != 1 || dry.Debris != 1 || dry.Unrecognised != 1 {
		t.Errorf("dry run = %+v", dry)
	}
	if dry.Removed != 0 || dry.Bytes != 0 {
		t.Errorf("a dry run deleted %d files", dry.Removed)
	}
	if !f.stored(orphan, entity.AssetKindImage) {
		t.Error("a dry run deleted the orphan")
	}

	applied := f.sweep(app.SweepOptions{Apply: true})
	if applied.Removed != 2 {
		t.Errorf("removed %d files, want 2 (the orphan and the crashed write)", applied.Removed)
	}
	if applied.DirsRemoved == 0 {
		t.Error("the directories the orphan left empty were not pruned")
	}
	if dry.DirsRemoved != 0 {
		t.Error("a dry run pruned directories")
	}
	if applied.Bytes == 0 {
		t.Error("no bytes were reported reclaimed")
	}
	if f.stored(orphan, entity.AssetKindImage) {
		t.Error("the unreferenced file survived")
	}
	if !f.stored(referenced, entity.AssetKindImage) {
		t.Error("a referenced file was deleted")
	}
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Error("old debris survived")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Error("a write that may still be in flight was deleted")
	}
	if _, err := os.Stat(unknown); err != nil {
		t.Error("a file the store layout does not explain was deleted")
	}
}

// Pointed at the wrong database, every file looks like garbage. That is the one
// mistake with an irreversible cost, so it is refused rather than reported.
func TestSweepRefusesADatabaseThatKnowsNoAssets(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	stored, err := f.assets.Put(ctx, entity.AssetKindImage, bytes.NewReader([]byte("a real asset")))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	id := stored.ID

	// Reporting is still allowed, and is how the mistake gets noticed: it changes
	// nothing, and the numbers are what show the database does not match the store.
	dry := f.sweep(app.SweepOptions{})
	if dry.Unreferenced != 1 || dry.Removed != 0 {
		t.Errorf("dry run = %+v", dry)
	}

	_, err = app.SweepAssets(ctx, f.store, f.assets, app.SweepOptions{Apply: true, Now: testTime}, f.log)
	if !errors.Is(err, app.ErrSweepUnsafe) {
		t.Fatalf("error = %v, want ErrSweepUnsafe", err)
	}
	if !f.stored(id, entity.AssetKindImage) {
		t.Fatal("the refusal still deleted the file")
	}

	forced := f.sweep(app.SweepOptions{Apply: true, Force: true})
	if forced.Removed != 1 {
		t.Errorf("--force removed %d files, want 1", forced.Removed)
	}
}
