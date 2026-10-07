package update

import (
	"archive/zip"
	"bufio"
	"compress/flate"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const zipPrefix = "yt-studio-"

// zipSuffix ends every release zip: they're per platform.
var zipSuffix = "-" + runtime.GOOS + "-" + runtime.GOARCH + ".zip"

// ZipName is the file name of a version's release zip.
func ZipName(version string) string { return zipPrefix + version + zipSuffix }

// Publish zips the app bundle into dir as version, then points the manifest
// at it (which is what running apps react to). Only the newest keep zips
// stay in dir.
func Publish(dir, bundle, version string, notes []string, keep int) (Manifest, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Manifest{}, err
	}
	tmp, err := os.CreateTemp(dir, ".release-*.zip")
	if err != nil {
		return Manifest{}, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	h := sha256.New()
	buf := bufio.NewWriterSize(io.MultiWriter(tmp, h), 1<<20)
	err = Zip(buf, bundle)
	if err == nil {
		err = buf.Flush()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), 0o644)
	}
	if err != nil {
		return Manifest{}, fmt.Errorf("zip %s: %w", bundle, err)
	}
	info, err := os.Stat(tmp.Name())
	if err != nil {
		return Manifest{}, err
	}
	name := ZipName(version)
	if err := os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		return Manifest{}, err
	}

	m := Manifest{
		Version:     version,
		File:        name,
		SHA256:      hex.EncodeToString(h.Sum(nil)),
		Size:        info.Size(),
		Notes:       notes,
		PublishedAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := WriteManifest(dir, m); err != nil {
		return Manifest{}, err
	}
	return m, Prune(dir, keep)
}

// Zip writes the bundle folder into w as a zip whose only top-level entry
// is the bundle (what the updater expects), keeping modes and symlinks.
func Zip(w io.Writer, bundle string) error {
	zw := zip.NewWriter(w)
	// The zip is read once, from the same disk: favour speed over size.
	zw.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, flate.BestSpeed)
	})
	parent := filepath.Dir(bundle)
	err := filepath.WalkDir(bundle, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(parent, path)
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		// Local time: unzippers like ditto read the MS-DOS time as local.
		hdr.Modified = info.ModTime()
		switch {
		case d.IsDir():
			hdr.Name += "/"
			hdr.Method = zip.Store
			_, err = zw.CreateHeader(hdr)
			return err
		case info.Mode()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			hdr.Method = zip.Store
			fw, err := zw.CreateHeader(hdr)
			if err != nil {
				return err
			}
			_, err = io.WriteString(fw, target)
			return err
		case !info.Mode().IsRegular():
			return fmt.Errorf("%s: unsupported file type", rel)
		}
		hdr.Method = zip.Deflate
		fw, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		_, err = io.Copy(fw, f)
		return err
	})
	if err != nil {
		return err
	}
	return zw.Close()
}

// Prune deletes all but the newest keep release zips in dir, never the one
// the manifest points at.
func Prune(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	current := ""
	if m, err := ReadManifest(dir); err == nil {
		current = m.File
	}
	var versions []string
	for _, e := range entries {
		if v, ok := zipVersion(e.Name()); ok && !e.IsDir() {
			versions = append(versions, v)
		}
	}
	slices.SortFunc(versions, func(a, b string) int { return semver.Compare("v"+b, "v"+a) })
	for i, v := range versions {
		if i < keep || ZipName(v) == current {
			continue
		}
		if err := os.Remove(filepath.Join(dir, ZipName(v))); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// zipVersion is the version of a release zip's file name.
func zipVersion(name string) (string, bool) {
	v, ok := strings.CutPrefix(name, zipPrefix)
	if !ok {
		return "", false
	}
	v, ok = strings.CutSuffix(v, zipSuffix)
	return v, ok && Valid(v)
}
