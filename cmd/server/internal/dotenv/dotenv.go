// Package dotenv reads KEY=VALUE files into the process environment.
//
// The format is the small one: KEY=VALUE, `#` comments, blank lines, an
// optional `export` prefix, surrounding quotes stripped. No `${VAR}`
// interpolation, which is where implementations start disagreeing, and no
// inline comments, since a secret may contain a `#`.
//
// A variable already set is never overwritten, and the whole file is parsed
// before anything is applied, so a malformed line changes nothing.
//
// It holds no policy — which file, and what a missing one means, belong to the
// command. That is also why it is under cmd/server/internal: Load mutates the
// process environment, so a use case calling it must not compile.
package dotenv

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Load applies the file at path and reports how many variables it set. A count
// smaller than the file is the precedence working, not an error.
func Load(path string) (int, error) {
	file, err := os.Open(path) //nolint:gosec // the path is the caller's own, by definition
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()

	vars, err := parse(file, path)
	if err != nil {
		return 0, err
	}

	set := 0
	for _, v := range vars {
		// LookupEnv rather than Getenv: a variable deliberately set to empty is
		// still set, and the file must not talk over it.
		if _, ok := os.LookupEnv(v.key); ok {
			continue
		}
		if err := os.Setenv(v.key, v.value); err != nil {
			return set, fmt.Errorf("%s:%d: set %s: %w", path, v.line, v.key, err)
		}
		set++
	}
	return set, nil
}

// assignment is one KEY=VALUE, carrying the line it came from so an error can
// point at it.
type assignment struct {
	key   string
	value string
	line  int
}

// parse reads every line before the caller applies any of them, which is what
// makes a malformed file leave nothing behind.
func parse(r *os.File, path string) ([]assignment, error) {
	var (
		vars []assignment
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
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "export ")

		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			return nil, fmt.Errorf("%s:%d: %q is not KEY=VALUE", path, line, trimmed)
		}
		key = strings.TrimSpace(key)
		if err := validKey(key); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		// A key set twice is a paste over an old value that was never deleted.
		// First-wins would apply the stale one and say nothing, which is twenty
		// minutes of an operator's afternoon.
		if first, dup := seen[key]; dup {
			return nil, fmt.Errorf("%s:%d: %s is already set on line %d", path, line, key, first)
		}
		seen[key] = line
		vars = append(vars, assignment{key: key, value: unquote(strings.TrimSpace(value)), line: line})
	}
	if err := scan.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return vars, nil
}

// validKey holds the shell's own rule, so a file that loads here is a file that
// would also have worked as a series of exports.
func validKey(key string) error {
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

// unquote strips one layer of matching quotes. There are no escape sequences,
// so a value is exactly what it looks like in the file.
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
