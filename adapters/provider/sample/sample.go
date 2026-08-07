// Package sample serves narration, slides and thumbnail icons from real media
// files on disk, so the pipeline can be exercised against production-shaped
// input without a GPU or a network call.
//
// It is a real backend, not a mock: a video composed from these files is one
// you can actually watch. They are operator-supplied, under the resources
// directory, and discovered by pattern at first use:
//
//	<resources>/sample/*.wav       narration, reused by every chapter
//	<resources>/sample/img*.jpg    slides, rotated across chapters
//	<resources>/sample/icon*.jpg   thumbnail tiles, one per grid cell
//	<resources>/sample/video.mp4   the composed clip, and the final render
//
// The icons and the video are optional: they arrived after the rest, so a
// library without them still serves the others, and only Icons()/Video()
// report the absence.
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

// ErrUnavailable reports missing or unusable sample media. Wrapping the port's
// sentinel stops a task retrying a directory that will not appear on its own.
var ErrUnavailable = fmt.Errorf("sample provider: %w", provider.ErrUnavailable)

// Library is the discovered media, shared by every backend here. It scans
// lazily behind a sync.Once, so construction touches no filesystem and Check
// reports a missing directory at startup.
type Library struct {
	dir string

	once   sync.Once
	err    error
	audio  string
	slides []string
	icons  []string
	video  string
}

// NewLibrary points at the sample directory inside a resources root, which
// already holds exactly this kind of operator-supplied file.
func NewLibrary(resourcesDir string) *Library {
	return &Library{dir: filepath.Join(resourcesDir, "sample")}
}

// Dir returns the directory the media is read from.
func (l *Library) Dir() string { return l.dir }

// Check reports whether the media is present and usable, so startup says it
// once rather than every task saying it separately.
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
	// Optional, so a missing set is reported by Icons, to whoever asked.
	icons, _ := l.glob("icon*.jpg")
	// Optional too, and reported by Video. The convention is video.mp4; the
	// glob lets an operator keep the take in use beside the ones that are not.
	video, _ := l.glob("*.mp4")

	l.audio, l.slides, l.icons = audio[0], slides, icons
	if len(video) > 0 {
		l.video = video[0]
	}
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

// Video returns the sample render, or says why there is none.
func (l *Library) Video() (string, error) {
	if err := l.Check(); err != nil {
		return "", err
	}
	if l.video == "" {
		return "", fmt.Errorf("%w: no video.mp4 in %s", ErrUnavailable, l.dir)
	}
	return l.video, nil
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

// checkRIFF rejects a non-WAV up front: the composer reads the header to time
// a chapter, so a mislabelled file would surface much later.
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
