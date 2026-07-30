// Package assetstore is the content-addressed file store behind every generated
// artifact.
//
// Files stream in and out with a pooled 256 KiB buffer: a three-hour render is
// never read into memory.
package assetstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// ErrOutsideRoot is returned if a computed path would escape the store root.
var ErrOutsideRoot = errors.New("asset path escapes store root")

const copyBufferSize = 256 << 10

// FS is a filesystem-backed AssetStore rooted at a single directory.
type FS struct {
	root    string
	bufPool sync.Pool
}

var _ provider.AssetStore = (*FS)(nil)

// New creates the store root if it does not exist.
func New(root string) (*FS, error) {
	if root == "" {
		return nil, errors.New("asset store root must not be empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve asset root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("create asset root: %w", err)
	}
	return &FS{
		root: abs,
		bufPool: sync.Pool{
			New: func() any {
				b := make([]byte, copyBufferSize)
				return &b
			},
		},
	}, nil
}

// Root returns the absolute store root.
func (f *FS) Root() string { return f.root }

// RelPath is the on-disk layout: kind/aa/<sha256><ext>. Sharding by the first
// byte of the digest keeps directory sizes sane at tens of thousands of stills.
func RelPath(id entity.AssetID, kind entity.AssetKind) string {
	s := string(id)
	shard := "00"
	if len(s) >= 2 {
		shard = s[:2]
	}
	return filepath.Join(string(kind), shard, s+kind.Ext())
}

// Put streams r into the store and returns its content address. Writing to a
// temporary file and renaming makes the operation atomic: a crash mid-write
// never leaves a half-written file under a hash that claims to be complete.
func (f *FS) Put(ctx context.Context, kind entity.AssetKind, r io.Reader) (provider.StoredAsset, error) {
	if !kind.Valid() {
		return provider.StoredAsset{}, fmt.Errorf("%w: unknown kind %q", entity.ErrInvalidAsset, kind)
	}
	tmpDir := filepath.Join(f.root, ".tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return provider.StoredAsset{}, fmt.Errorf("create temp dir: %w", err)
	}
	tmp, err := os.CreateTemp(tmpDir, "put-*")
	if err != nil {
		return provider.StoredAsset{}, fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName) // no-op once the rename has happened
	}()

	h := sha256.New()
	buf, ok := f.bufPool.Get().(*[]byte)
	if !ok {
		// Unreachable: the pool's New only ever produces *[]byte.
		scratch := make([]byte, copyBufferSize)
		buf = &scratch
	}
	size, copyErr := io.CopyBuffer(io.MultiWriter(tmp, h), &ctxReader{ctx: ctx, r: r}, *buf)
	f.bufPool.Put(buf)
	if copyErr != nil {
		return provider.StoredAsset{}, fmt.Errorf("stream asset: %w", copyErr)
	}
	if err := tmp.Sync(); err != nil {
		return provider.StoredAsset{}, fmt.Errorf("sync asset: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return provider.StoredAsset{}, fmt.Errorf("close asset: %w", err)
	}

	id := entity.AssetID(hex.EncodeToString(h.Sum(nil)))
	rel := RelPath(id, kind)
	dst, err := f.resolve(rel)
	if err != nil {
		return provider.StoredAsset{}, err
	}
	if _, err := os.Stat(dst); err == nil {
		// Identical content already stored: re-running the task is a no-op.
		return provider.StoredAsset{ID: id, Path: rel, Size: size, Existed: true}, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return provider.StoredAsset{}, fmt.Errorf("create asset dir: %w", err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return provider.StoredAsset{}, fmt.Errorf("commit asset: %w", err)
	}
	return provider.StoredAsset{ID: id, Path: rel, Size: size}, nil
}

// PutFile ingests a file that is already on disk, by hashing it in place and
// renaming it into the store. Nothing is copied when the source sits on the
// same filesystem, which is what keeps a multi-gigabyte render cheap to ingest:
// a composer writes its output once and hands the path over.
//
// The source is consumed either way — renamed on success, removed when the
// content address is already present.
func (f *FS) PutFile(ctx context.Context, kind entity.AssetKind, src string) (provider.StoredAsset, error) {
	if !kind.Valid() {
		return provider.StoredAsset{}, fmt.Errorf("%w: unknown kind %q", entity.ErrInvalidAsset, kind)
	}
	file, err := os.Open(src) //nolint:gosec // src is produced by this process
	if err != nil {
		return provider.StoredAsset{}, fmt.Errorf("open source: %w", err)
	}
	h := sha256.New()
	buf, ok := f.bufPool.Get().(*[]byte)
	if !ok {
		scratch := make([]byte, copyBufferSize)
		buf = &scratch
	}
	size, copyErr := io.CopyBuffer(h, &ctxReader{ctx: ctx, r: file}, *buf)
	f.bufPool.Put(buf)
	if err := errors.Join(copyErr, file.Close()); err != nil {
		return provider.StoredAsset{}, fmt.Errorf("hash source: %w", err)
	}

	id := entity.AssetID(hex.EncodeToString(h.Sum(nil)))
	rel := RelPath(id, kind)
	dst, err := f.resolve(rel)
	if err != nil {
		return provider.StoredAsset{}, err
	}
	if _, err := os.Stat(dst); err == nil {
		_ = os.Remove(src)
		return provider.StoredAsset{ID: id, Path: rel, Size: size, Existed: true}, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return provider.StoredAsset{}, fmt.Errorf("create asset dir: %w", err)
	}
	if err := os.Rename(src, dst); err != nil {
		// A rename across filesystems fails with EXDEV; fall back to streaming the
		// bytes so a work directory outside the store root still works.
		return f.copyInto(ctx, kind, src)
	}
	return provider.StoredAsset{ID: id, Path: rel, Size: size}, nil
}

// copyInto is PutFile's cross-filesystem fallback: the same bytes, ingested the
// slow way.
func (f *FS) copyInto(ctx context.Context, kind entity.AssetKind, src string) (provider.StoredAsset, error) {
	in, err := os.Open(src) //nolint:gosec // src is produced by this process
	if err != nil {
		return provider.StoredAsset{}, fmt.Errorf("reopen source: %w", err)
	}
	defer func() {
		_ = in.Close()
		_ = os.Remove(src)
	}()
	return f.Put(ctx, kind, in)
}

// Path returns the absolute on-disk location of an asset, for the one consumer
// that cannot take a reader: an external process invoked with argv.
func (f *FS) Path(id entity.AssetID, kind entity.AssetKind) (string, error) {
	return f.resolve(RelPath(id, kind))
}

// Open returns a seekable reader, so the HTTP layer can serve range requests
// for video scrubbing without buffering.
func (f *FS) Open(_ context.Context, id entity.AssetID, kind entity.AssetKind) (io.ReadSeekCloser, error) {
	path, err := f.resolve(RelPath(id, kind))
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path) //nolint:gosec // path is resolved and root-checked
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("asset %s: %w", id.Short(), entity.ErrAssetNotFound)
		}
		return nil, fmt.Errorf("open asset %s: %w", id.Short(), err)
	}
	return file, nil
}

// Stat describes a stored asset without opening it.
func (f *FS) Stat(_ context.Context, id entity.AssetID, kind entity.AssetKind) (provider.StoredAsset, error) {
	rel := RelPath(id, kind)
	path, err := f.resolve(rel)
	if err != nil {
		return provider.StoredAsset{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return provider.StoredAsset{}, fmt.Errorf("asset %s: %w", id.Short(), entity.ErrAssetNotFound)
		}
		return provider.StoredAsset{}, fmt.Errorf("stat asset %s: %w", id.Short(), err)
	}
	return provider.StoredAsset{ID: id, Path: rel, Size: info.Size(), Existed: true}, nil
}

// Delete unlinks one stored file. A missing file is success: the caller unlinks
// after its database transaction has committed, so a retry of an interrupted
// reclaim must not fail on the half it already finished.
//
// Empty shard directories are left behind. There are 256 of them per kind at
// most, they are reused by the next asset with the same digest prefix, and
// removing them would race every concurrent Put into the same shard.
func (f *FS) Delete(ctx context.Context, id entity.AssetID, kind entity.AssetKind) error {
	return f.Remove(ctx, RelPath(id, kind))
}

// Remove unlinks one path relative to the root, which is how the sweep reaches a
// file that is not a valid content address.
func (f *FS) Remove(_ context.Context, rel string) error {
	path, err := f.resolve(rel)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", rel, err)
	}
	return nil
}

// Walk visits every regular file in the store, so a sweep can compare what is on
// disk against what the database still references.
//
// It walks rather than reading a manifest because the store is the authority on
// what exists: a file whose row was lost is exactly what needs finding, and it
// would be invisible to anything driven from the database side.
func (f *FS) Walk(ctx context.Context, fn func(provider.StoredFile) error) error {
	tmpDir := filepath.Join(f.root, ".tmp")
	return filepath.WalkDir(f.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			// The file went away mid-walk, which is a Put being cleaned up under us.
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("stat %s: %w", path, err)
		}
		rel, err := filepath.Rel(f.root, path)
		if err != nil {
			return fmt.Errorf("relativise %s: %w", path, err)
		}
		found := provider.StoredFile{
			Rel:       rel,
			Size:      info.Size(),
			Temporary: strings.HasPrefix(path, tmpDir+string(os.PathSeparator)),
			ModTime:   info.ModTime(),
		}
		if id, kind, ok := parseRelPath(rel); ok {
			found.ID, found.Kind = id, kind
		}
		return fn(found)
	})
}

// parseRelPath is RelPath read backwards. It insists on the whole layout --
// kind/shard/<64 hex digits><ext>, with the shard matching the digest -- so that
// only a file this package wrote can be recognised as an asset, and anything
// else is left for a human to look at.
func parseRelPath(rel string) (entity.AssetID, entity.AssetKind, bool) {
	parts := strings.Split(rel, string(os.PathSeparator))
	if len(parts) != 3 {
		return "", "", false
	}
	kind := entity.AssetKind(parts[0])
	if !kind.Valid() {
		return "", "", false
	}
	name := parts[2]
	digest := strings.TrimSuffix(name, kind.Ext())
	if digest == name || len(digest) != 64 || !isHex(digest) {
		return "", "", false
	}
	if parts[1] != digest[:2] {
		return "", "", false
	}
	return entity.AssetID(digest), kind, true
}

func isHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// resolve joins a relative path to the root and refuses anything that would
// escape it — a content address is attacker-supplied as far as this layer
// knows.
func (f *FS) resolve(rel string) (string, error) {
	clean := filepath.Clean(filepath.Join(f.root, rel))
	if clean != f.root && !strings.HasPrefix(clean, f.root+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: %q", ErrOutsideRoot, rel)
	}
	return clean, nil
}

// ctxReader makes a long copy cancellable: a shutdown or an aborted video stops
// an in-flight multi-gigabyte read rather than waiting it out.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
