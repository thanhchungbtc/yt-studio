package dotenv_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tbui/yt-studio/cmd/server/internal/dotenv"
)

// write puts a file in a temporary directory and returns its path.
func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	return path
}

// unset clears variables for the duration of one test. t.Setenv registers the
// restore, and the Unsetenv after it is what makes them genuinely absent rather
// than present and empty — a distinction this package is built on.
func unset(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
	}
}

func TestLoadSetsVariables(t *testing.T) {
	unset(t, "DOTENV_A", "DOTENV_B", "DOTENV_C", "DOTENV_D", "DOTENV_E")
	path := write(t, strings.Join([]string{
		"# a comment",
		"",
		"DOTENV_A=plain",
		"export DOTENV_B=http://127.0.0.1:20128",
		`DOTENV_C="double quoted"`,
		"DOTENV_D='single quoted'",
		"  DOTENV_E = spaced  ",
	}, "\n"))

	set, err := dotenv.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if set != 5 {
		t.Errorf("set %d variables, want 5", set)
	}
	for key, want := range map[string]string{
		"DOTENV_A": "plain",
		"DOTENV_B": "http://127.0.0.1:20128",
		"DOTENV_C": "double quoted",
		"DOTENV_D": "single quoted",
		"DOTENV_E": "spaced",
	} {
		if got := os.Getenv(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

// TestTheEnvironmentWins is the property that makes a one-off override
// possible: an exported variable must not be talked over by a file.
func TestTheEnvironmentWins(t *testing.T) {
	path := write(t, "DOTENV_SET=from-the-file\nDOTENV_EMPTY=from-the-file\n")
	t.Setenv("DOTENV_SET", "from-the-shell")
	// Deliberately empty is still set, and the file must not fill it in.
	t.Setenv("DOTENV_EMPTY", "")

	set, err := dotenv.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if set != 0 {
		t.Errorf("set %d variables, want none", set)
	}
	if got := os.Getenv("DOTENV_SET"); got != "from-the-shell" {
		t.Errorf("DOTENV_SET = %q, want the shell's value", got)
	}
	if got := os.Getenv("DOTENV_EMPTY"); got != "" {
		t.Errorf("DOTENV_EMPTY = %q, want the empty value it was set to", got)
	}
}

func TestMissingFile(t *testing.T) {
	_, err := dotenv.Load(filepath.Join(t.TempDir(), "nope.env"))
	if !os.IsNotExist(err) {
		// The caller decides whether a missing file is an error, and it can only
		// decide if the reason survives the return.
		t.Fatalf("expected a not-exist error the caller can recognise, got %v", err)
	}
}

func TestMalformedFilesAreRejected(t *testing.T) {
	cases := map[string]struct {
		body     string
		contains string
	}{
		"no equals":        {body: "DOTENV_X\n", contains: "not KEY=VALUE"},
		"no name":          {body: "=value\n", contains: "no name"},
		"unusable name":    {body: "DOTENV-X=value\n", contains: "not a usable variable name"},
		"leading digit":    {body: "1DOTENV=value\n", contains: "not a usable variable name"},
		"set twice":        {body: "DOTENV_X=one\nDOTENV_X=two\n", contains: "already set on line 1"},
		"names the line":   {body: "DOTENV_GOOD=1\nBAD\n", contains: ":2:"},
		"names the reason": {body: "DOTENV_GOOD=1\n!!!=2\n", contains: "not a usable variable name"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			unset(t, "DOTENV_GOOD")
			set, err := dotenv.Load(write(t, tc.body))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Errorf("error %q does not carry %q", err, tc.contains)
			}
			// Nothing is applied until the whole file parses, so a bad line leaves
			// the environment as it was rather than half configured.
			if set != 0 {
				t.Errorf("reported setting %d variables from a rejected file", set)
			}
			if _, ok := os.LookupEnv("DOTENV_GOOD"); ok {
				t.Error("a rejected file still set a variable")
			}
		})
	}
}

// TestValuesAreLiteral covers the two rules a secret is most likely to trip
// over: a `#` inside a value, and the absence of interpolation.
func TestValuesAreLiteral(t *testing.T) {
	unset(t, "DOTENV_HASH", "DOTENV_EXPAND", "DOTENV_BLANK")
	path := write(t, strings.Join([]string{
		"DOTENV_HASH=rw-a#b$c",
		"DOTENV_EXPAND=${DOTENV_HASH}",
		"DOTENV_BLANK=",
	}, "\n"))

	if _, err := dotenv.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := os.Getenv("DOTENV_HASH"); got != "rw-a#b$c" {
		t.Errorf("DOTENV_HASH = %q; an inline # must not truncate a secret", got)
	}
	if got := os.Getenv("DOTENV_EXPAND"); got != "${DOTENV_HASH}" {
		t.Errorf("DOTENV_EXPAND = %q, want it taken literally", got)
	}
	if got, ok := os.LookupEnv("DOTENV_BLANK"); !ok || got != "" {
		t.Errorf("DOTENV_BLANK = %q, %v; an empty value is still an assignment", got, ok)
	}
}

func TestByteOrderMarkAndCarriageReturns(t *testing.T) {
	unset(t, "DOTENV_BOM", "DOTENV_CRLF")
	// What a Windows editor writes. Neither should reach the variable name or the
	// value, because the error that produces explains nothing.
	path := write(t, "\ufeffDOTENV_BOM=first\r\nDOTENV_CRLF=second\r\n")

	if _, err := dotenv.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := os.Getenv("DOTENV_BOM"); got != "first" {
		t.Errorf("DOTENV_BOM = %q, want the value without the byte order mark", got)
	}
	if got := os.Getenv("DOTENV_CRLF"); got != "second" {
		t.Errorf("DOTENV_CRLF = %q, want the value without the carriage return", got)
	}
}

// TestPartialApplication pins the count down: a file whose keys are half
// already set reports only what it actually changed.
func TestPartialApplication(t *testing.T) {
	unset(t, "DOTENV_FRESH")
	t.Setenv("DOTENV_HELD", "held")
	path := write(t, "DOTENV_HELD=from-the-file\nDOTENV_FRESH=from-the-file\n")

	set, err := dotenv.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if set != 1 {
		t.Errorf("set %d variables, want 1 of the 2 in the file", set)
	}
	if got := os.Getenv("DOTENV_HELD"); got != "held" {
		t.Errorf("DOTENV_HELD = %q, want the value it already had", got)
	}
}
