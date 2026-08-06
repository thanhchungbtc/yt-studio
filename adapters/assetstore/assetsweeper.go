package assetstore

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

var _ provider.AssetSweeper = (*FS)(nil)

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

// PruneEmptyDirs removes the kind and shard directories that no longer hold
// anything, and reports how many it removed.
//
// Emptiness is checked rather than inferred, and deeper directories go first, so
// a kind directory is only considered once the shards under it are gone. A
// directory that fills up between the check and the removal simply stays: the
// next sweep will find it empty again if it empties.
//
// The staging directory is left alone. Every write recreates it, so pruning it
// buys nothing, and unlike a shard directory its use has no rename to retry.
func (f *FS) PruneEmptyDirs(ctx context.Context) (int, error) {
	tmpDir := filepath.Join(f.root, ".tmp")
	var dirs []string
	err := filepath.WalkDir(f.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !d.IsDir() || path == f.root {
			return nil
		}
		if path == tmpDir {
			return fs.SkipDir
		}
		dirs = append(dirs, path)
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("walk the asset store: %w", err)
	}

	sort.SliceStable(dirs, func(i, j int) bool { return depth(dirs[i]) > depth(dirs[j]) })

	var removed int
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return removed, fmt.Errorf("read %s: %w", dir, err)
		}
		if len(entries) > 0 {
			continue
		}
		if err := os.Remove(dir); err == nil {
			removed++
		}
	}
	return removed, nil
}

func depth(path string) int { return strings.Count(path, string(os.PathSeparator)) }

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
