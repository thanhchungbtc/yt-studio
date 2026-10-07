package update

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// fakeBundle makes a small .app bundle with an executable, a resource and
// a symlink, like the real one.
func fakeBundle(t *testing.T, version string) string {
	t.Helper()
	app := filepath.Join(t.TempDir(), "yt-studio.app")
	macos := filepath.Join(app, "Contents", "MacOS")
	res := filepath.Join(app, "Contents", "Resources")
	for _, d := range []string{macos, res} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(macos, "yt-studio"), []byte("binary "+version), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(res, "icons.icns"), []byte(strings.Repeat("icon", 1000)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("icons.icns", filepath.Join(res, "current.icns")); err != nil {
		t.Fatal(err)
	}
	return app
}

func TestVersions(t *testing.T) {
	for _, v := range []string{"0.1.0", "1.20.3"} {
		if !Valid(v) {
			t.Errorf("Valid(%q) = false", v)
		}
	}
	for _, v := range []string{"", "dev", "v0.1.0", "0.1", "0.1.0-rc1", "01.0.0"} {
		if Valid(v) {
			t.Errorf("Valid(%q) = true", v)
		}
	}
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.1.1", "0.1.0", true},
		{"0.10.0", "0.9.9", true},
		{"0.1.0", "0.1.0", false},
		{"0.1.0", "0.2.0", false},
		{"0.1.0", "dev", true},
		{"0.1.0", "v0.0.9", true},
		{"dev", "0.1.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestZipKeepsBundleLayout(t *testing.T) {
	app := fakeBundle(t, "1")
	path := filepath.Join(t.TempDir(), "out.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Zip(f, app); err != nil {
		t.Fatal(err)
	}
	f.Close()

	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	entries := map[string]*zip.File{}
	for _, f := range zr.File {
		if top, _, _ := strings.Cut(f.Name, "/"); top != "yt-studio.app" {
			t.Errorf("entry %q outside the bundle", f.Name)
		}
		entries[f.Name] = f
	}
	exe := entries["yt-studio.app/Contents/MacOS/yt-studio"]
	if exe == nil || exe.Mode().Perm() != 0o755 {
		t.Fatalf("executable missing or not executable: %v", exe)
	}
	link := entries["yt-studio.app/Contents/Resources/current.icns"]
	if link == nil || link.Mode()&fs.ModeSymlink == 0 {
		t.Fatalf("symlink not kept: %v", link)
	}
	rc, _ := link.Open()
	target, _ := io.ReadAll(rc)
	rc.Close()
	if string(target) != "icons.icns" {
		t.Errorf("symlink target = %q", target)
	}
}

func TestPublishWritesManifestAndPrunes(t *testing.T) {
	dir := t.TempDir()
	for _, v := range []string{"0.1.0", "0.1.1", "0.1.2", "0.2.0"} {
		m, err := Publish(dir, fakeBundle(t, v), v, []string{"change " + v}, 2)
		if err != nil {
			t.Fatal(err)
		}
		if m.File != ZipName(v) {
			t.Errorf("file = %q", m.File)
		}
	}
	m, err := ReadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != "0.2.0" || len(m.Notes) != 1 || m.Size == 0 {
		t.Errorf("manifest = %+v", m)
	}
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := []string{ZipName("0.1.2"), ZipName("0.2.0"), ManifestName}
	slices.Sort(want)
	if !slices.Equal(names, want) {
		t.Errorf("folder = %v, want %v", names, want)
	}
}

func TestReadManifestRejectsBadFields(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadManifest(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing manifest: %v", err)
	}
	good := Manifest{Version: "0.1.0", File: ZipName("0.1.0"), SHA256: strings.Repeat("a", 64)}
	for name, m := range map[string]Manifest{
		"version": {Version: "latest", File: good.File, SHA256: good.SHA256},
		"path":    {Version: good.Version, File: "../evil.zip", SHA256: good.SHA256},
		"digest":  {Version: good.Version, File: good.File, SHA256: "abc"},
	} {
		if err := WriteManifest(dir, m); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadManifest(dir); err == nil {
			t.Errorf("%s: accepted %+v", name, m)
		}
	}
}

func TestSourceOffersOnlyNewerReleases(t *testing.T) {
	dir := t.TempDir()
	src := Source{Dir: dir}
	if rel, err := src.Check(context.Background(), updater.CheckRequest{CurrentVersion: "0.1.0"}); rel != nil || err != nil {
		t.Fatalf("empty folder: %v, %v", rel, err)
	}
	if _, err := Publish(dir, fakeBundle(t, "0.1.1"), "0.1.1", []string{"Faster startup"}, 3); err != nil {
		t.Fatal(err)
	}
	if rel, _ := src.Check(context.Background(), updater.CheckRequest{CurrentVersion: "0.1.1"}); rel != nil {
		t.Errorf("same version offered: %+v", rel)
	}
	rel, err := src.Check(context.Background(), updater.CheckRequest{CurrentVersion: "0.1.0"})
	if err != nil || rel == nil {
		t.Fatalf("newer release not offered: %v, %v", rel, err)
	}
	if rel.Version != "0.1.1" || !slices.Equal(Notes(rel), []string{"Faster startup"}) || len(rel.Verification.Digest) != 32 {
		t.Errorf("release = %+v", rel)
	}
}

// The real updater downloads, verifies and unpacks a published release.
func TestUpdaterInstallsPublishedRelease(t *testing.T) {
	dir := t.TempDir()
	if _, err := Publish(dir, fakeBundle(t, "0.2.0"), "0.2.0", nil, 3); err != nil {
		t.Fatal(err)
	}
	u := updater.New(nopHost{})
	if err := u.Init(updater.Config{CurrentVersion: "0.1.0", Providers: []updater.Provider{Source{Dir: dir}}, Window: updater.WindowNone}); err != nil {
		t.Fatal(err)
	}
	rel, err := u.Check(context.Background())
	if err != nil || rel == nil {
		t.Fatalf("check: %v, %v", rel, err)
	}
	if err := u.DownloadAndInstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	staged := u.DownloadedPath()
	t.Cleanup(func() { os.RemoveAll(filepath.Dir(staged)) })
	if filepath.Base(staged) != "yt-studio.app" {
		t.Fatalf("staged %q, want the bundle", staged)
	}
	got, err := os.ReadFile(filepath.Join(staged, "Contents", "MacOS", "yt-studio"))
	if err != nil || string(got) != "binary 0.2.0" {
		t.Errorf("staged executable = %q, %v", got, err)
	}

	// A tampered zip fails verification.
	m, _ := ReadManifest(dir)
	if err := os.WriteFile(filepath.Join(dir, m.File), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := u.DownloadAndInstall(context.Background()); err == nil {
		t.Error("tampered release installed")
	}
}

type nopHost struct{}

func (nopHost) Emit(string, ...any) bool                              { return false }
func (nopHost) OnEvent(string, func(any)) func()                      { return func() {} }
func (nopHost) OpenWindow(updater.WindowOptions) updater.WindowHandle { return nil }
func (nopHost) Quit()                                                 {}
