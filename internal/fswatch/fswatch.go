// Package fswatch reports file changes under a folder, recursively, using
// the operating system's file events (FSEvents on macOS): one stream per
// folder however large the tree, coalesced by the system.
package fswatch

import (
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Change is a batch of changed paths, relative to the watched folder
// ("/"-separated). Rescan means events were lost or coalesced beyond
// recognition: treat everything as changed.
type Change struct {
	Paths  []string
	Rescan bool
}

// Watcher watches one folder.
type Watcher struct {
	root   string
	real   string
	stream *stream
	once   sync.Once
}

// Watch starts watching root, calling onChange (from a background thread)
// with what changed, at most every latency.
func Watch(root string, latency time.Duration, onChange func(Change)) (*Watcher, error) {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	w := &Watcher{root: root, real: resolved}
	s, err := start(resolved, latency, func(paths []string, rescan bool) {
		onChange(w.relativize(paths, rescan))
	})
	if err != nil {
		return nil, err
	}
	w.stream = s
	return w, nil
}

// WatchDirs watches several folders (recursively, given symlink-free) in a
// single stream, calling onChange (from a background thread) at most every
// latency with the folders whose contents changed: absolute, symlink-free,
// and not the files themselves, so busy trees cost few events. Rescan means
// events were lost or a watched folder itself moved: treat all as changed.
// onChange must not Close the watcher.
func WatchDirs(roots []string, latency time.Duration, onChange func(dirs []string, rescan bool)) (*Watcher, error) {
	s, err := startDirs(roots, latency, func(paths []string, rescan bool) {
		for i, p := range paths {
			paths[i] = filepath.Clean(p)
		}
		onChange(paths, rescan)
	})
	if err != nil {
		return nil, err
	}
	return &Watcher{stream: s}, nil
}

// relativize maps reported paths (always symlink-free) to paths relative to
// the watched folder.
func (w *Watcher) relativize(paths []string, rescan bool) Change {
	c := Change{Rescan: rescan}
	seen := make(map[string]bool, len(paths))
	for _, p := range paths {
		rel, ok := strings.CutPrefix(p, w.real)
		if !ok {
			continue
		}
		rel = strings.Trim(filepath.ToSlash(rel), "/")
		if !seen[rel] {
			seen[rel] = true
			c.Paths = append(c.Paths, rel)
		}
	}
	return c
}

// Close stops watching; no callback runs after it returns.
func (w *Watcher) Close() {
	w.once.Do(func() {
		if w.stream != nil {
			w.stream.stop()
		}
	})
}
