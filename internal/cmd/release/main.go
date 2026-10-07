// Command release builds a new version of the app and publishes it to the
// local releases folder, where the installed app picks it up and offers to
// restart into it (`make release`). `release install` puts the newest
// release in /Applications (`make install`), for a first install.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tbui/yt-studio/internal/home"
	"github.com/tbui/yt-studio/internal/update"
)

const (
	appName   = "yt-studio"
	bundle    = "bin/yt-studio.app"
	installed = "/Applications/yt-studio.app"
	keep      = 3
)

func main() {
	var err error
	if len(os.Args) > 1 && os.Args[1] == "install" {
		err = install()
	} else {
		err = release(os.Args[1:])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "\033[31m✗\033[0m", err)
		os.Exit(1)
	}
}

func release(args []string) error {
	fl := flag.NewFlagSet("release", flag.ExitOnError)
	bump := fl.String("bump", "patch", "part of the version to bump: major, minor or patch")
	version := fl.String("version", "", "release this exact version instead (x.y.z)")
	allowDirty := fl.Bool("allow-dirty", false, "release uncommitted changes (the release isn't tagged)")
	wails := fl.String("wails", "wails3", "the wails3 command")
	_ = fl.Parse(args)

	start := time.Now()
	dir, err := releasesDir()
	if err != nil {
		return err
	}
	dirty, err := git("status", "--porcelain")
	if err != nil {
		return err
	}
	if dirty != "" && !*allowDirty {
		return fmt.Errorf("uncommitted changes; commit them first (or release them anyway with ALLOW_DIRTY=1):\n%s", dirty)
	}

	// The newest version so far: tagged, or published untagged.
	tag, err := latestTag()
	if err != nil {
		return err
	}
	latest := strings.TrimPrefix(tag, "v")
	if m, err := update.ReadManifest(dir); err == nil && update.Newer(m.Version, latest) {
		latest = m.Version
	}
	next, err := nextVersion(latest, *bump, *version)
	if err != nil {
		return err
	}
	if _, err := git("rev-parse", "--quiet", "--verify", "refs/tags/v"+next); err == nil {
		return fmt.Errorf("tag v%s already exists", next)
	}
	notes, err := releaseNotes(tag)
	if err != nil {
		return err
	}

	step("Building %s %s", appName, next)
	build := exec.CommandContext(context.Background(), *wails, "task", "package", "VERSION="+next)
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}
	if got, _ := output("plutil", "-extract", "CFBundleShortVersionString", "raw", "-o", "-", filepath.Join(bundle, "Contents", "Info.plist")); got != next {
		return fmt.Errorf("the built app says version %q, want %q", got, next)
	}

	step("Publishing")
	m, err := update.Publish(dir, bundle, next, notes, keep)
	if err != nil {
		return err
	}
	if dirty == "" {
		if _, err := git("tag", "-a", "v"+next, "-m", appName+" "+next); err != nil {
			return err
		}
	}

	fmt.Fprintf(os.Stderr, "\n\033[32m✓\033[0m Released %s %s in %s (%s)\n", appName, next, time.Since(start).Round(100*time.Millisecond), size(m.Size))
	if dirty != "" {
		fmt.Fprintln(os.Stderr, "  Not tagged: it includes uncommitted changes.")
	} else {
		fmt.Fprintf(os.Stderr, "  Tagged v%s.\n", next)
	}
	for _, n := range notes {
		fmt.Fprintf(os.Stderr, "  • %s\n", n)
	}
	switch v, updates := installedVersion(); {
	case v == "":
		fmt.Fprintln(os.Stderr, "\n  Not installed yet: run make install.")
	case !updates:
		fmt.Fprintf(os.Stderr, "\n  The installed app (%s) can't update itself yet: quit it and run make install, once.\n", v)
	default:
		fmt.Fprintf(os.Stderr, "\n  yt-studio %s offers to restart into it.\n", v)
	}
	return nil
}

// installedVersion is the version in /Applications ("" if none), and whether
// it updates itself (release builds do).
func installedVersion() (string, bool) {
	plist := filepath.Join(installed, "Contents", "Info.plist")
	v, err := output("plutil", "-extract", "CFBundleShortVersionString", "raw", "-o", "-", plist)
	if err != nil {
		return "", false
	}
	updates, _ := output("plutil", "-extract", "YTStudioUpdates", "raw", "-o", "-", plist)
	return v, updates == "true"
}

// install puts the newest release in /Applications.
func install() error {
	dir, err := releasesDir()
	if err != nil {
		return err
	}
	m, err := update.ReadManifest(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return errors.New("no release yet: run make release first")
	}
	if err != nil {
		return err
	}
	// pgrep exits 1 when nothing matches; other failures don't block.
	if err := exec.CommandContext(context.Background(), "pgrep", "-f", installed+"/Contents/MacOS/").Run(); err == nil {
		return fmt.Errorf("%s is running; use Restart to Update in the app, or quit it first", appName)
	}
	zip := filepath.Join(dir, m.File)
	if err := verify(zip, m.SHA256); err != nil {
		return err
	}

	// Unpack next to the target, then swap: the app is never half there.
	tmp, err := os.MkdirTemp(filepath.Dir(installed), ".yt-studio-install-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if out, err := exec.CommandContext(context.Background(), "ditto", "-x", "-k", zip, tmp).CombinedOutput(); err != nil {
		return fmt.Errorf("unzip: %w: %s", err, out)
	}
	if _, err := os.Stat(installed); err == nil {
		if err := os.Rename(installed, filepath.Join(tmp, "previous.app")); err != nil {
			return err
		}
	}
	if err := os.Rename(filepath.Join(tmp, filepath.Base(bundle)), installed); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "\033[32m✓\033[0m Installed %s %s in %s\n", appName, m.Version, installed)
	return nil
}

// nextVersion is the version after latest ("" when there's none yet).
func nextVersion(latest, bump, explicit string) (string, error) {
	if explicit != "" {
		explicit = strings.TrimPrefix(explicit, "v")
		if !update.Valid(explicit) {
			return "", fmt.Errorf("invalid version %q: want MAJOR.MINOR.PATCH", explicit)
		}
		if latest != "" && !update.Newer(explicit, latest) {
			return "", fmt.Errorf("version %s isn't newer than %s", explicit, latest)
		}
		return explicit, nil
	}
	if latest == "" {
		return "0.1.0", nil
	}
	parts := strings.Split(latest, ".")
	n := make([]int, 3)
	for i := range n {
		n[i], _ = strconv.Atoi(parts[i])
	}
	switch bump {
	case "major":
		n = []int{n[0] + 1, 0, 0}
	case "minor":
		n = []int{n[0], n[1] + 1, 0}
	case "patch":
		n[2]++
	default:
		return "", fmt.Errorf("invalid bump %q: want major, minor or patch", bump)
	}
	return fmt.Sprintf("%d.%d.%d", n[0], n[1], n[2]), nil
}

// latestTag is the highest release tag (vX.Y.Z), or "".
func latestTag() (string, error) {
	out, err := git("tag", "--list", "v*")
	if err != nil {
		return "", err
	}
	best := ""
	for t := range strings.FieldsSeq(out) {
		v := strings.TrimPrefix(t, "v")
		if update.Valid(v) && (best == "" || update.Newer(v, best[1:])) {
			best = t
		}
	}
	return best, nil
}

// releaseNotes are the commit subjects since tag.
func releaseNotes(tag string) ([]string, error) {
	args := []string{"log", "--no-merges", "--format=%s"}
	if tag != "" {
		args = append(args, tag+"..HEAD")
	} else {
		args = append(args, "-10")
	}
	out, err := git(args...)
	if err != nil {
		return nil, err
	}
	var notes []string
	for line := range strings.SplitSeq(out, "\n") {
		if line = strings.TrimSpace(line); line != "" && len(notes) < 30 {
			notes = append(notes, line)
		}
	}
	return notes, nil
}

// releasesDir is the installation's releases folder.
func releasesDir() (string, error) {
	dir, err := home.Default()
	if err != nil {
		return "", err
	}
	return dir.Releases(), nil
}

func verify(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("%s is damaged (checksum mismatch)", filepath.Base(path))
	}
	return nil
}

func git(args ...string) (string, error) { return output("git", args...) }

func output(name string, args ...string) (string, error) {
	out, err := exec.CommandContext(context.Background(), name, args...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func step(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "\033[1m→ "+format+"…\033[0m\n", args...)
}

func size(n int64) string { return fmt.Sprintf("%.1f MB", float64(n)/(1<<20)) }
