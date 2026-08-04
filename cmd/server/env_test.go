package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestLoadEnvFileSetsVariables(t *testing.T) {
	reset(t)
	writeEnv(t, strings.Join([]string{
		"# a comment",
		"",
		"RUNWARE_KEY=rw-abc123",
		"export NINEROUTER_URL=http://127.0.0.1:20128",
		`YTS_LISTEN="127.0.0.1:9090"`,
		"YTS_TRANSCRIPTS='var/transcripts'",
		"  YTS_LOG_LEVEL = debug  ",
	}, "\n"))
	for _, key := range []string{
		"RUNWARE_KEY", "NINEROUTER_URL", "YTS_LISTEN", "YTS_TRANSCRIPTS", "YTS_LOG_LEVEL",
	} {
		t.Setenv(key, "")
		os.Unsetenv(key) //nolint:errcheck,usetesting // t.Setenv above restores it after the test
	}

	if err := loadEnvFile(); err != nil {
		t.Fatalf("loadEnvFile: %v", err)
	}
	for key, want := range map[string]string{
		"RUNWARE_KEY":     "rw-abc123",
		"NINEROUTER_URL":  "http://127.0.0.1:20128",
		"YTS_LISTEN":      "127.0.0.1:9090",
		"YTS_TRANSCRIPTS": "var/transcripts",
		"YTS_LOG_LEVEL":   "debug",
	} {
		if got := os.Getenv(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if envVarsLoaded != 5 {
		t.Errorf("loaded %d variables, want 5", envVarsLoaded)
	}
}

// TestTheRealEnvironmentWins is the property that makes a one-off override
// possible: RUNWARE_KEY=other yt-studio serve must not be talked over.
func TestTheRealEnvironmentWins(t *testing.T) {
	reset(t)
	writeEnv(t, "RUNWARE_KEY=from-the-file\nNINEROUTER_KEY=from-the-file\n")
	t.Setenv("RUNWARE_KEY", "from-the-shell")
	// Deliberately empty is still set, and the file must not fill it in.
	t.Setenv("NINEROUTER_KEY", "")

	if err := loadEnvFile(); err != nil {
		t.Fatalf("loadEnvFile: %v", err)
	}
	if got := os.Getenv("RUNWARE_KEY"); got != "from-the-shell" {
		t.Errorf("RUNWARE_KEY = %q, want the shell's value", got)
	}
	if got := os.Getenv("NINEROUTER_KEY"); got != "" {
		t.Errorf("NINEROUTER_KEY = %q, want the empty value it was set to", got)
	}
	if envVarsLoaded != 0 {
		t.Errorf("loaded %d variables, want none", envVarsLoaded)
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

func TestMalformedFilesAreRejected(t *testing.T) {
	cases := map[string]struct {
		body     string
		contains string
	}{
		"no equals":      {body: "RUNWARE_KEY\n", contains: "not KEY=VALUE"},
		"no name":        {body: "=rw-abc\n", contains: "no name"},
		"unusable name":  {body: "RUNWARE-KEY=rw-abc\n", contains: "not a usable variable name"},
		"leading digit":  {body: "1KEY=rw-abc\n", contains: "not a usable variable name"},
		"set twice":      {body: "RUNWARE_KEY=one\nRUNWARE_KEY=two\n", contains: "already set on line 1"},
		"named the line": {body: "GOOD=1\nBAD\n", contains: ":2:"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			reset(t)
			writeEnv(t, tc.body)
			os.Unsetenv("GOOD") //nolint:errcheck,usetesting // only read through the failure below

			err := loadEnvFile()
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Errorf("error %q does not carry %q", err, tc.contains)
			}
			// Nothing is applied until the whole file parses, so a bad line leaves
			// the environment as it was rather than half configured.
			if _, ok := os.LookupEnv("GOOD"); ok {
				t.Error("a rejected file still set a variable")
			}
		})
	}
}

// TestValuesAreLiteral covers the two rules a secret is most likely to trip
// over: a `#` inside a value, and the absence of interpolation.
func TestValuesAreLiteral(t *testing.T) {
	reset(t)
	writeEnv(t, strings.Join([]string{
		"RUNWARE_KEY=rw-a#b$c",
		"NINEROUTER_KEY=${RUNWARE_KEY}",
		"YTS_LISTEN=",
	}, "\n"))
	for _, key := range []string{"RUNWARE_KEY", "NINEROUTER_KEY", "YTS_LISTEN"} {
		t.Setenv(key, "")
		os.Unsetenv(key) //nolint:errcheck,usetesting // t.Setenv above restores it after the test
	}

	if err := loadEnvFile(); err != nil {
		t.Fatalf("loadEnvFile: %v", err)
	}
	if got := os.Getenv("RUNWARE_KEY"); got != "rw-a#b$c" {
		t.Errorf("RUNWARE_KEY = %q; an inline # must not truncate a secret", got)
	}
	if got := os.Getenv("NINEROUTER_KEY"); got != "${RUNWARE_KEY}" {
		t.Errorf("NINEROUTER_KEY = %q, want it taken literally", got)
	}
	if got, ok := os.LookupEnv("YTS_LISTEN"); !ok || got != "" {
		t.Errorf("YTS_LISTEN = %q, %v; an empty value is still an assignment", got, ok)
	}
}

func TestByteOrderMarkAndCarriageReturns(t *testing.T) {
	reset(t)
	// What a Windows editor writes. Neither should reach the variable name or the
	// value, because the error it produces explains nothing.
	writeEnv(t, "\ufeffRUNWARE_KEY=rw-abc\r\nNINEROUTER_URL=http://localhost:20128\r\n")
	os.Unsetenv("RUNWARE_KEY")    //nolint:errcheck,usetesting // restored by the setenv below
	os.Unsetenv("NINEROUTER_URL") //nolint:errcheck,usetesting // restored by the setenv below
	t.Cleanup(func() {
		os.Unsetenv("RUNWARE_KEY")    //nolint:errcheck,usetesting // package-level cleanup
		os.Unsetenv("NINEROUTER_URL") //nolint:errcheck,usetesting // package-level cleanup
	})

	if err := loadEnvFile(); err != nil {
		t.Fatalf("loadEnvFile: %v", err)
	}
	if got := os.Getenv("RUNWARE_KEY"); got != "rw-abc" {
		t.Errorf("RUNWARE_KEY = %q, want the value without the byte order mark", got)
	}
	if got := os.Getenv("NINEROUTER_URL"); got != "http://localhost:20128" {
		t.Errorf("NINEROUTER_URL = %q, want the value without the carriage return", got)
	}
}

// TestTheCommittedExampleParses keeps the documentation honest: the file an
// operator copies has to be one the loader accepts.
func TestTheCommittedExampleParses(t *testing.T) {
	reset(t)
	t.Setenv(envFileVar, filepath.Join("..", "..", ".env.example"))
	// Every key in the example is one the daemon reads, so setting them here
	// would change this process's configuration. They are all already set to
	// something by the harness below, which is what makes the load a no-op.
	for _, key := range []string{
		"RUNWARE_KEY", "NINEROUTER_KEY", "NINEROUTER_URL", "YTS_DB", "YTS_ASSETS",
		"YTS_RESOURCES", "YTS_LISTEN", "YTS_TRANSCRIPTS", "YTS_LOG_LEVEL",
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
