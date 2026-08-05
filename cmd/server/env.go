package main

// Which environment file is read, and what a missing one means. The reading
// itself is dotenv's; everything here is policy.
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
	"fmt"
	"os"
	"strings"

	"github.com/tbui/yt-studio/cmd/server/internal/dotenv"
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
// once it has a logger. They are package state because kong constructs the
// command and runs it: main has nowhere to hand a local to.
var (
	envFileLoaded string
	envVarsLoaded int
)

// loadEnvFile applies the environment file, if there is one to apply.
//
// A missing default file is silence: production sets real environment variables
// and has no file. A missing file that was asked for by name is an error,
// because asking for one by name and not getting it is a typo, not a
// configuration.
func loadEnvFile() error {
	path := strings.TrimSpace(os.Getenv(envFileVar))
	named := path != ""
	if !named {
		path = defaultEnvFile
	}

	vars, err := dotenv.Load(path)
	if err != nil {
		if os.IsNotExist(err) && !named {
			return nil
		}
		return fmt.Errorf("environment file: %w", err)
	}
	envFileLoaded, envVarsLoaded = path, vars
	return nil
}
