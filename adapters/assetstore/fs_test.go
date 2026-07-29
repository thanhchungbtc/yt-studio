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
