package assetstore_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/tbui/yt-studio/adapters/assetstore"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

func newStore(t *testing.T) *assetstore.FS {
	t.Helper()
	store, err := assetstore.New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

// writeTemp puts a file inside the store's own scratch directory, which is
// where a composer writes its output.
func writeTemp(t *testing.T, store *assetstore.FS, content string) string {
	t.Helper()
	dir := filepath.Join(store.Root(), ".tmp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create scratch dir: %v", err)
	}
	path := filepath.Join(dir, "render.mp4")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write scratch file: %v", err)
	}
	return path
}

func TestPutFileAddressesByContent(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	const content = "not really an mp4, but the bytes are what is addressed"
	src := writeTemp(t, store, content)

	stored, err := store.PutFile(t.Context(), entity.AssetKindFinal, src)
	if err != nil {
		t.Fatalf("put file: %v", err)
	}

	sum := sha256.Sum256([]byte(content))
	if want := entity.AssetID(hex.EncodeToString(sum[:])); stored.ID != want {
		t.Errorf("id = %s, want %s", stored.ID, want)
	}
	if stored.Size != int64(len(content)) {
		t.Errorf("size = %d, want %d", stored.Size, len(content))
	}
	if stored.Existed {
		t.Error("a first ingest must not report the content as already present")
	}

	// The source is consumed: a composer's scratch file must not survive as a
	// second copy of a multi-gigabyte render.
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("source still exists after ingest: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(store.Root(), stored.Path))
	if err != nil {
		t.Fatalf("read stored file: %v", err)
	}
	if string(got) != content {
		t.Errorf("stored content = %q, want %q", got, content)
	}
}

func TestPutFileMatchesPut(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	const content = "the same bytes by either door"

	streamed, err := store.Put(t.Context(), entity.AssetKindClip, bytes.NewReader([]byte(content)))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	renamed, err := store.PutFile(t.Context(), entity.AssetKindClip, writeTemp(t, store, content))
	if err != nil {
		t.Fatalf("put file: %v", err)
	}

	if renamed.ID != streamed.ID {
		t.Errorf("PutFile addressed %s, Put addressed %s", renamed.ID, streamed.ID)
	}
	// The address was already present, so the ingest is a no-op that still
	// consumes the source.
	if !renamed.Existed {
		t.Error("re-ingesting identical content must report Existed")
	}
}

func TestPutFileRejectsUnknownKind(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	if _, err := store.PutFile(t.Context(), entity.AssetKind("nonsense"), writeTemp(t, store, "x")); err == nil {
		t.Error("expected an error for an unknown kind, got none")
	}
}

func TestPathResolvesUnderTheRoot(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	id := entity.AssetID("abcdef0123456789")

	path, err := store.Path(id, entity.AssetKindClip)
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	if want := filepath.Join(store.Root(), assetstore.RelPath(id, entity.AssetKindClip)); path != want {
		t.Errorf("path = %s, want %s", path, want)
	}
}

func TestDeleteIsIdempotent(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	stored, err := store.Put(t.Context(), entity.AssetKindImage, bytes.NewReader([]byte("a still")))
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	if err := store.Delete(t.Context(), stored.ID, entity.AssetKindImage); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.Root(), stored.Path)); !os.IsNotExist(err) {
		t.Errorf("file survived the delete: %v", err)
	}
	// A reclaim interrupted after the database committed is retried, and must not
	// fail on the half it already finished.
	if err := store.Delete(t.Context(), stored.ID, entity.AssetKindImage); err != nil {
		t.Errorf("deleting an absent asset = %v, want nil", err)
	}
}

func TestRemoveRefusesToEscapeTheRoot(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	if err := store.Remove(t.Context(), filepath.Join("..", "outside.txt")); err == nil {
		t.Error("expected a path outside the root to be refused")
	}
}

// The sweep decides what to delete from what Walk says a file is, so the
// classification is the safety-critical part: only a file this package wrote may
// be recognised as an asset.
func TestWalkClassifiesWhatItFinds(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	asset, err := store.Put(t.Context(), entity.AssetKindImage, bytes.NewReader([]byte("a still")))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	debris := writeTemp(t, store, "half a render")

	// Two impostors: the right shape in the wrong place, and the right place with
	// the wrong name.
	strays := map[string]string{
		filepath.Join(store.Root(), "notes.txt"):                     "written by a person",
		filepath.Join(store.Root(), "image", "aa", "not-a-hash.png"): "wrong name",
	}
	for path, content := range strays {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	seen := map[string]provider.StoredFile{}
	if err := store.Walk(t.Context(), func(f provider.StoredFile) error {
		seen[f.Rel] = f
		return nil
	}); err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(seen) != 4 {
		t.Fatalf("walked %d files, want 4: %v", len(seen), seen)
	}

	found, ok := seen[assetstore.RelPath(asset.ID, entity.AssetKindImage)]
	if !ok {
		t.Fatal("the stored asset was not walked")
	}
	if found.ID != asset.ID || found.Kind != entity.AssetKindImage {
		t.Errorf("asset walked as id=%q kind=%q", found.ID, found.Kind)
	}
	if found.Size != asset.Size || found.Temporary {
		t.Errorf("asset walked as %+v", found)
	}

	relDebris, err := filepath.Rel(store.Root(), debris)
	if err != nil {
		t.Fatal(err)
	}
	if temp := seen[relDebris]; !temp.Temporary || temp.ID != "" {
		t.Errorf("scratch file walked as %+v, want temporary with no address", temp)
	}

	for path := range strays {
		rel, err := filepath.Rel(store.Root(), path)
		if err != nil {
			t.Fatal(err)
		}
		if stray := seen[rel]; stray.ID != "" {
			t.Errorf("%s was recognised as asset %s", rel, stray.ID)
		}
	}
}

func TestPruneEmptyDirsLeavesWhatIsInUse(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	kept, err := store.Put(t.Context(), entity.AssetKindImage, bytes.NewReader([]byte("a still that stays")))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	gone, err := store.Put(t.Context(), entity.AssetKindAudio, bytes.NewReader([]byte("narration to delete")))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := store.Delete(t.Context(), gone.ID, entity.AssetKindAudio); err != nil {
		t.Fatalf("delete: %v", err)
	}
	// The staging directory exists because Put made it, and must survive: every
	// write recreates it and its use has no rename to retry.
	tmpDir := filepath.Join(store.Root(), ".tmp")
	if _, err := os.Stat(tmpDir); err != nil {
		t.Fatalf("the staging directory is missing before the prune: %v", err)
	}

	// Both the shard and the kind above it are now empty, so both go: two levels
	// from one pass.
	pruned, err := store.PruneEmptyDirs(t.Context())
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if pruned != 2 {
		t.Errorf("pruned %d directories, want 2 (the empty shard and its kind)", pruned)
	}

	if _, err := os.Stat(filepath.Join(store.Root(), "audio")); !os.IsNotExist(err) {
		t.Errorf("the emptied kind directory survived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.Root(), kept.Path)); err != nil {
		t.Errorf("a stored asset was disturbed: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(filepath.Join(store.Root(), kept.Path))); err != nil {
		t.Errorf("a shard still holding an asset was pruned: %v", err)
	}
	if _, err := os.Stat(tmpDir); err != nil {
		t.Errorf("the staging directory was pruned: %v", err)
	}
	if _, err := os.Stat(store.Root()); err != nil {
		t.Errorf("the root was pruned: %v", err)
	}

	// Nothing left to do the second time.
	if again, err := store.PruneEmptyDirs(t.Context()); err != nil || again != 0 {
		t.Errorf("second prune = %d, %v; want 0, nil", again, err)
	}
}

// A write whose shard directory is pruned out from under it retries rather than
// failing the task, which is what makes pruning safe against a live daemon.
func TestPutSurvivesAPrunedShardDirectory(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	const content = "the bytes arrive either way"
	sum := sha256.Sum256([]byte(content))
	id := entity.AssetID(hex.EncodeToString(sum[:]))
	shard := filepath.Dir(filepath.Join(store.Root(), assetstore.RelPath(id, entity.AssetKindImage)))

	// Stand the directory up and take it away again: the state Put finds if a
	// prune lands between its MkdirAll and its Rename.
	if err := os.MkdirAll(shard, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(shard); err != nil {
		t.Fatal(err)
	}

	stored, err := store.Put(t.Context(), entity.AssetKindImage, bytes.NewReader([]byte(content)))
	if err != nil {
		t.Fatalf("put with a missing shard directory: %v", err)
	}
	if stored.ID != id {
		t.Errorf("id = %s, want %s", stored.ID, id)
	}
	got, err := os.ReadFile(filepath.Join(store.Root(), stored.Path))
	if err != nil {
		t.Fatalf("read stored file: %v", err)
	}
	if string(got) != content {
		t.Errorf("stored content = %q", got)
	}
}
