package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The parsing rules are the dotenv package's and are tested there. What is left
// here is the policy: which file is read, what a missing one means, and what
// the load reports for the startup log line.

// writeEnv puts a file in a temporary directory and makes it the one the loader
// finds, by name rather than by working directory: a test that chdir'd would
// not be safe to run beside its neighbours.
func writeEnv(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	t.Setenv(envFileVar, path)
	return path
}

// reset clears the package state the loader records, so each test reads its own
// result rather than the previous one's.
func reset(t *testing.T) {
	t.Helper()
	envFileLoaded, envVarsLoaded = "", 0
	t.Cleanup(func() { envFileLoaded, envVarsLoaded = "", 0 })
}

func TestLoadEnvFileReportsWhatItDid(t *testing.T) {
	reset(t)
	path := writeEnv(t, "RUNWARE_KEY=rw-abc123\nNINEROUTER_URL=http://127.0.0.1:20128\n")
	t.Setenv("RUNWARE_KEY", "")
	if err := os.Unsetenv("RUNWARE_KEY"); err != nil {
		t.Fatalf("unset: %v", err)
	}
	// Already set, so the file cannot claim it and the count must not either.
	t.Setenv("NINEROUTER_URL", "http://from-the-shell")

	if err := loadEnvFile(); err != nil {
		t.Fatalf("loadEnvFile: %v", err)
	}
	if envFileLoaded != path {
		t.Errorf("reported loading %q, want %q", envFileLoaded, path)
	}
	if envVarsLoaded != 1 {
		t.Errorf("reported %d variables, want 1", envVarsLoaded)
	}
	if got := os.Getenv("RUNWARE_KEY"); got != "rw-abc123" {
		t.Errorf("RUNWARE_KEY = %q, want the file's value", got)
	}
	if got := os.Getenv("NINEROUTER_URL"); got != "http://from-the-shell" {
		t.Errorf("NINEROUTER_URL = %q, want the shell's value", got)
	}
}

func TestMissingFile(t *testing.T) {
	t.Run("the default one is silence", func(t *testing.T) {
		reset(t)
		// The default is relative to the working directory, which during a test is
		// this package's own — where there is no .env.
		t.Setenv(envFileVar, "")
		if err := loadEnvFile(); err != nil {
			t.Fatalf("a missing default file must not be an error: %v", err)
		}
		if envFileLoaded != "" {
			t.Errorf("reported loading %q", envFileLoaded)
		}
	})

	t.Run("a named one is an error", func(t *testing.T) {
		reset(t)
		t.Setenv(envFileVar, filepath.Join(t.TempDir(), "nope.env"))
		if err := loadEnvFile(); err == nil {
			t.Fatal("asking for a file by name and not getting it is a typo, not a configuration")
		}
	})
}

// TestMalformedFilesFailTheBoot checks only that the rejection reaches main
// with the line on it; which lines are malformed is the dotenv package's rule.
func TestMalformedFilesFailTheBoot(t *testing.T) {
	reset(t)
	writeEnv(t, "RUNWARE_KEY=rw-abc\nBAD LINE\n")

	err := loadEnvFile()
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), ":2:") {
		t.Errorf("error %q does not point at the offending line", err)
	}
	if envFileLoaded != "" {
		t.Errorf("a rejected file was reported as loaded: %q", envFileLoaded)
	}
}

// TestTheCommittedExampleParses keeps the documentation honest: the file an
// operator copies has to be one the loader accepts.
func TestTheCommittedExampleParses(t *testing.T) {
	reset(t)
	t.Setenv(envFileVar, filepath.Join("..", "..", ".env.example"))
	// Every key in the example is one the server reads, so letting the file win
	// here would change this process's configuration. Holding them all already
	// set is what makes the load a no-op.
	for _, key := range []string{
		"RUNWARE_KEY", "NINEROUTER_KEY", "NINEROUTER_URL", "XTTS_URL", "YTS_DB",
		"YTS_ASSETS", "YTS_RESOURCES", "YTS_LISTEN", "YTS_TRANSCRIPTS", "YTS_LOG_LEVEL",
	} {
		t.Setenv(key, "held")
	}

	if err := loadEnvFile(); err != nil {
		t.Fatalf(".env.example does not parse: %v", err)
	}
	if envVarsLoaded != 0 {
		t.Errorf("the example overwrote %d already-set variables", envVarsLoaded)
	}
}
