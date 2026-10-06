// Package home is the installation directory's layout (~/.yt-studio or $YTS_HOME).
package home

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Env overrides the installation directory.
const Env = "YTS_HOME"

// Dir is an installation directory.
type Dir string

// Default is $YTS_HOME, or ~/.yt-studio.
func Default() (Dir, error) {
	if dir := strings.TrimSpace(os.Getenv(Env)); dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", err
		}
		return Dir(abs), nil
	}
	user, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return Dir(filepath.Join(user, ".yt-studio")), nil
}

func (d Dir) path(parts ...string) string {
	return filepath.Join(append([]string{string(d)}, parts...)...)
}

// DB is the SQLite database.
func (d Dir) DB() string { return d.path("db", "yt-studio.db") }

// Assets is the content-addressed artifact store.
func (d Dir) Assets() string { return d.path("assets") }

// Resources is operator-supplied media.
func (d Dir) Resources() string { return d.path("resources") }

// Credentials holds one directory of OAuth files per channel slug.
func (d Dir) Credentials() string { return d.path("credentials") }

// Transcripts holds one file per LLM exchange.
func (d Dir) Transcripts() string { return d.path("transcripts") }

// Releases is where `make release` publishes versions to update to.
func (d Dir) Releases() string { return d.path("releases") }

// Log is the app's log, rewritten each launch.
func (d Dir) Log() string { return d.path("log", "app.log") }

// Blueprints holds pasted outlines until their blueprint task reads them.
func (d Dir) Blueprints() string { return d.path("tmp", "blueprints") }

// Window is where the window's bounds are remembered.
func (d Dir) Window() string { return d.path("window.json") }

// Ensure creates the directories nothing else creates; home is 0700 (it holds API keys).
func (d Dir) Ensure() error {
	for _, dir := range []struct {
		path string
		mode os.FileMode
	}{
		{string(d), 0o700},
		{d.Resources(), 0o755},
		{d.Credentials(), 0o700},
		{d.Blueprints(), 0o700},
	} {
		if err := os.MkdirAll(dir.path, dir.mode); err != nil {
			return fmt.Errorf("create %s: %w", dir.path, err)
		}
	}
	return nil
}
