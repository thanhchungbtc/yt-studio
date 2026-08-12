package main

// Where ffmpeg is found, when nobody set up an environment.
//
// A process started from a terminal inherits that shell's PATH, so `ffmpeg`
// resolves and none of this matters. A bundled application started from the
// Finder inherits launchd's instead — /usr/bin:/bin:/usr/sbin:/sbin — which
// contains no package manager's prefix, so the composer would report itself
// unavailable on a machine where ffmpeg is plainly installed and works.
//
// The fix is to look where the package managers put things. It is deliberately
// a PATH edit rather than an absolute path threaded into the composer: ffprobe
// is resolved the same way, and so is anything exec'd later.

import (
	"os"
	"path/filepath"
	"strings"
)

// searchPrefixes are the directories a macOS package manager installs into, in
// the order they should be preferred: Homebrew on Apple silicon, Homebrew on
// Intel, then MacPorts.
var searchPrefixes = []string{
	"/opt/homebrew/bin",
	"/usr/local/bin",
	"/opt/local/bin",
}

// widenPath appends the package-manager prefixes to PATH. Appended rather than
// prepended, so a deliberately placed binary earlier in the operator's PATH
// still wins; this only adds places to look, and never changes which of two
// candidates is chosen.
//
// A prefix that is already listed is skipped, so a normal terminal run keeps
// the PATH it started with.
func widenPath() {
	current := os.Getenv("PATH")
	have := make(map[string]bool, 8)
	for _, dir := range filepath.SplitList(current) {
		have[filepath.Clean(dir)] = true
	}

	missing := make([]string, 0, len(searchPrefixes))
	for _, dir := range searchPrefixes {
		if have[dir] {
			continue
		}
		// Only directories that exist: a PATH entry that is not there costs a
		// failed stat on every lookup, and says nothing true about the machine.
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		missing = append(missing, dir)
	}
	if len(missing) == 0 {
		return
	}

	widened := strings.Join(missing, string(filepath.ListSeparator))
	if current != "" {
		widened = current + string(filepath.ListSeparator) + widened
	}
	_ = os.Setenv("PATH", widened)
}
