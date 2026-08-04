package main

// Environment file loading, which happens before kong parses anything.
//
// Every bootstrap value is already a flag with an `env:` tag, and kong reads
// those tags through os.Getenv at Parse time. So a file that supplies them has
// to be in the process environment before Parse runs — which is why this is
// called from main rather than from a command's Run.
//
// What the file may hold is exactly the bootstrap flags and nothing else. It is
// not a second configuration tier: everything the server needs after the
// database is open is a settings row, editable live, and a value that could be
// set in both places is a value that can disagree with itself.

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// The file, and the variable that points somewhere else.
//
// The override is an environment variable rather than a flag because a flag
// cannot be read before the parse it is meant to feed. Reading it from the real
// environment has no such ordering problem.
const (
	defaultEnvFile = ".env"
	envFileVar     = "YTS_ENV_FILE"
)

// envFileLoaded and envVarsLoaded record what the load did, so serve can say so
// once it has a logger. They are package state because the load happens before
// the rest of the server exists to carry it.
var (
	envFileLoaded string
	envVarsLoaded int
)

// loadEnvFile reads KEY=VALUE lines into the process environment.
//
// A variable already set is never overwritten, so `RUNWARE_KEY=other yt-studio`
// still wins over the file for one run and a deployment that sets real
// environment variables is unaffected by a file someone left in the working
// directory.
//
// A missing default file is silence: production sets real variables and has no
// file. A missing file that was asked for by name is an error, because asking
// for one by name and not getting it is a typo, not a configuration.
func loadEnvFile() error {
	path := strings.TrimSpace(os.Getenv(envFileVar))
	named := path != ""
	if !named {
		path = defaultEnvFile
	}

	file, err := os.Open(path) //nolint:gosec // the path is the operator's own, by definition
	if err != nil {
		if os.IsNotExist(err) && !named {
			return nil
		}
		return fmt.Errorf("environment file: %w", err)
	}
	defer func() { _ = file.Close() }()

	vars, err := parseEnvFile(file, path)
	if err != nil {
		return err
	}
	for _, v := range vars {
		// LookupEnv rather than Getenv: a variable deliberately set to empty is
		// still set, and the file must not talk over it.
		if _, ok := os.LookupEnv(v.key); ok {
			continue
		}
		if err := os.Setenv(v.key, v.value); err != nil {
			return fmt.Errorf("%s:%d: set %s: %w", path, v.line, v.key, err)
		}
		envVarsLoaded++
	}
	envFileLoaded = path
	return nil
}

// envVar is one assignment, carrying the line it came from so an error can
// point at it.
type envVar struct {
	key   string
	value string
	line  int
}

// parseEnvFile reads the whole file before setting anything, so a malformed
// line leaves the environment untouched rather than half applied.
//
// The format is deliberately the small one: KEY=VALUE, `#` comments, blank
// lines, an optional `export` prefix, and surrounding quotes stripped. There is
// no `${VAR}` interpolation and no multiline value — interpolation is where
// dotenv implementations start disagreeing with each other, and nothing here
// needs it.
func parseEnvFile(r *os.File, path string) ([]envVar, error) {
	var (
		vars []envVar
		seen = make(map[string]int, 16)
		scan = bufio.NewScanner(r)
		line int
	)
	for scan.Scan() {
		line++
		text := strings.TrimRight(scan.Text(), "\r")
		if line == 1 {
			// An editor's byte order mark would otherwise become part of the first
			// key, and the error that produces explains nothing.
			text = strings.TrimPrefix(text, "\ufeff")
		}
		trimmed := strings.TrimSpace(text)
		// A `#` comments out a line only as its first character. Inline comments
		// are not supported on purpose: a secret may contain a `#`, and silently
		// truncating a key at one is a failure nobody would think to look for.
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "export ")

		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			return nil, fmt.Errorf("%s:%d: %q is not KEY=VALUE", path, line, trimmed)
		}
		key = strings.TrimSpace(key)
		if err := validEnvKey(key); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		// A key set twice is a paste over an old value that was never deleted.
		// First-wins would apply the stale one and say nothing, which is twenty
		// minutes of an operator's afternoon.
		if first, dup := seen[key]; dup {
			return nil, fmt.Errorf("%s:%d: %s is already set on line %d", path, line, key, first)
		}
		seen[key] = line
		vars = append(vars, envVar{key: key, value: unquote(strings.TrimSpace(value)), line: line})
	}
	if err := scan.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return vars, nil
}

// validEnvKey holds the shell's own rule, so a file that loads here is a file
// that would also have worked as a series of exports.
func validEnvKey(key string) error {
	if key == "" {
		return fmt.Errorf("a line with no name before the =")
	}
	for i, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return fmt.Errorf("%q is not a usable variable name", key)
		}
	}
	return nil
}

// unquote strips one layer of matching quotes. What is inside is taken
// literally: there are no escape sequences, so a value is exactly what it looks
// like in the file.
func unquote(value string) string {
	if len(value) < 2 {
		return value
	}
	quote := value[0]
	if (quote == '"' || quote == '\'') && value[len(value)-1] == quote {
		return value[1 : len(value)-1]
	}
	return value
}
