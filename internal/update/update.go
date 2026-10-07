// Package update publishes and finds local releases of the app.
//
// `make release` zips the app bundle into the releases folder and then
// writes a manifest (latest.json) describing it; the running app watches the
// folder and installs what the manifest points at through the Wails updater.
// Writing the manifest last, by rename, means the app never sees a release
// whose zip isn't complete.
package update

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"golang.org/x/mod/semver"
)

// ManifestName is the manifest's file name in the releases folder.
const ManifestName = "latest.json"

// Manifest describes the newest release in the releases folder.
type Manifest struct {
	Version     string    `json:"version"`
	File        string    `json:"file"`
	SHA256      string    `json:"sha256"`
	Size        int64     `json:"size"`
	Notes       []string  `json:"notes,omitempty"`
	PublishedAt time.Time `json:"publishedAt"`
}

// ReleasesDir is the releases folder inside the app's data folder.
func ReleasesDir(dataDir string) string { return filepath.Join(dataDir, "releases") }

// ReadManifest reads and validates the manifest in dir. A missing manifest
// is fs.ErrNotExist.
func ReadManifest(dir string) (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("read %s: %w", ManifestName, err)
	}
	switch {
	case !Valid(m.Version):
		return m, fmt.Errorf("%s: invalid version %q", ManifestName, m.Version)
	case m.File == "" || m.File != filepath.Base(m.File):
		return m, fmt.Errorf("%s: invalid file %q", ManifestName, m.File)
	case len(m.SHA256) != 64:
		return m, fmt.Errorf("%s: invalid sha256", ManifestName)
	}
	return m, nil
}

// WriteManifest writes m into dir atomically.
func WriteManifest(dir string, m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".latest-*.json")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, ManifestName))
}

// Valid reports whether v is a release version: MAJOR.MINOR.PATCH.
func Valid(v string) bool {
	c := semver.Canonical("v" + v)
	return c != "" && c == "v"+v && semver.Prerelease(c) == ""
}

// Newer reports whether version a is newer than b. Any release is newer
// than an unversioned (development) build.
func Newer(a, b string) bool {
	return Valid(a) && semver.Compare("v"+a, "v"+strings.TrimPrefix(b, "v")) > 0
}

// Source is an updater.Provider reading releases from a local folder.
type Source struct {
	Dir string
}

var _ updater.Provider = Source{}

// Name implements updater.Provider.
func (Source) Name() string { return "local" }

// Check implements updater.Provider: the release in the manifest, if it's
// newer than the running version.
func (s Source) Check(_ context.Context, req updater.CheckRequest) (*updater.Release, error) {
	m, err := ReadManifest(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !Newer(m.Version, req.CurrentVersion) {
		return nil, nil
	}
	digest, err := hex.DecodeString(m.SHA256)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid sha256: %w", ManifestName, err)
	}
	return &updater.Release{
		Version:      m.Version,
		Notes:        strings.Join(m.Notes, "\n"),
		PublishedAt:  m.PublishedAt,
		Artifact:     updater.Artifact{Filename: m.File, Filetype: "zip", Size: m.Size, Platform: req.Platform, Arch: req.Arch},
		Verification: &updater.Verification{DigestAlgo: "sha256", Digest: digest},
		Metadata:     map[string]any{"notes": m.Notes},
	}, nil
}

// Download implements updater.Provider: copies the release's zip.
func (s Source) Download(ctx context.Context, r *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	f, err := os.Open(filepath.Join(s.Dir, filepath.Base(r.Artifact.Filename)))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	total := r.Artifact.Size
	var written int64
	buf := make([]byte, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, rerr := f.Read(buf)
		if n > 0 {
			if _, err := dst.Write(buf[:n]); err != nil {
				return err
			}
			written += int64(n)
			onProgress(written, total)
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

// Notes returns the release notes Source.Check put on r.
func Notes(r *updater.Release) []string {
	if r == nil {
		return nil
	}
	notes, _ := r.Metadata["notes"].([]string)
	return notes
}
