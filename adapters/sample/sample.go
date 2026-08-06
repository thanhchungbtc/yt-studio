// Package sample serves narration and slides from real media files on
// disk, so the pipeline can be exercised against production-shaped input
// without a GPU or a network call.
//
// It is a real backend, not a mock: it does real work, takes as long as that
// work takes, and fails only when something is genuinely wrong. A video
// composed from these files is one you can actually watch.
//
// The files are operator-supplied and sit beside the other fixed production
// assets under the resources directory, discovered by pattern at first use:
//
//	<resources>/sample/*.wav       narration, reused by every chapter
//	<resources>/sample/img*.jpg    slides, rotated across chapters
//	<resources>/sample/icon*.jpg   thumbnail tiles, one per grid cell
//
// The icons are optional: they arrived after the other two, and a library
// without them still serves narration and slides. Selecting this backend for
// the icon port without them is what reports the absence, rather than a
// startup check failing over a file an operator may never have wanted.
package sample

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/tbui/yt-studio/domain/provider"
)

// ErrUnavailable reports missing or unusable sample media. It wraps the port's
// sentinel, so a task fails once and says why instead of retrying a directory
// that will not appear on its own.
var ErrUnavailable = fmt.Errorf("sample provider: %w", provider.ErrUnavailable)

// Library is the discovered media, shared by both backends.
//
// Scanning lazily behind a sync.Once mirrors the ffmpeg composer: construction
// touches no filesystem, so wiring cannot fail, and the operator learns about a
// missing directory from Check at startup rather than from the first chapter of
// a fifty-chapter video.
type Library struct {
	dir string

	once   sync.Once
	err    error
	audio  string
	slides []string
	icons  []string
}

// NewLibrary points at the sample directory inside a resources root. Sharing
// that flag is deliberate: these are fixed, operator-supplied,
// too-large-to-commit files, which is exactly what the directory already holds.
func NewLibrary(resourcesDir string) *Library {
	return &Library{dir: filepath.Join(resourcesDir, "sample")}
}

// Dir returns the directory the media is read from.
func (l *Library) Dir() string { return l.dir }

// Check reports whether the media is present and usable, so startup can say so
// once instead of every task saying it separately.
func (l *Library) Check() error {
	l.once.Do(func() { l.err = l.scan() })
	return l.err
}

func (l *Library) scan() error {
	info, err := os.Stat(l.dir)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrUnavailable, l.dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %s is not a directory", ErrUnavailable, l.dir)
	}

	audio, err := l.glob("*.wav")
	if err != nil {
		return err
	}
	if err := checkRIFF(audio[0]); err != nil {
		return err
	}
	slides, err := l.glob("img*.jpg")
	if err != nil {
		return err
	}
	// Optional, so a missing set is not an error here — Icons is where it is
	// reported, to whoever actually asked for one.
	icons, _ := l.glob("icon*.jpg")

	l.audio, l.slides, l.icons = audio[0], slides, icons
	return nil
}

// Icons returns the thumbnail tile set, or says why there is none.
func (l *Library) Icons() ([]string, error) {
	if err := l.Check(); err != nil {
		return nil, err
	}
	if len(l.icons) == 0 {
		return nil, fmt.Errorf("%w: no icon*.jpg in %s", ErrUnavailable, l.dir)
	}
	return l.icons, nil
}

// glob returns the matching files, sorted so the rotation follows the order the
// operator named them rather than whatever order the filesystem returned.
func (l *Library) glob(pattern string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(l.dir, pattern))
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrUnavailable, l.dir, err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("%w: no %s in %s", ErrUnavailable, pattern, l.dir)
	}
	sort.Strings(matches)
	return matches, nil
}

// checkRIFF rejects a file that is not a WAV up front. The composer reads the
// header directly to time a chapter, so a mislabelled file would otherwise
// surface much later as a confusing composition failure.
func checkRIFF(path string) error {
	file, err := os.Open(path) //nolint:gosec // path comes from the resources directory
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrUnavailable, path, err)
	}
	defer func() { _ = file.Close() }()

	var header [12]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrUnavailable, path, err)
	}
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return fmt.Errorf("%w: %s is not a RIFF/WAVE file", ErrUnavailable, path)
	}
	return nil
}
