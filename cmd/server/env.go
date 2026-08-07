package main

// Which environment file is read, and what a missing one means; the reading
// itself is dotenv's.
//
// Every bootstrap value is a flag with an `env:` tag, which kong reads at Parse
// time, so a file supplying them has to reach the environment before that —
// hence main rather than a command's Run.
//
// It holds the bootstrap flags and nothing else. It is not a second
// configuration tier: a value settable in two places can disagree with itself.

import (
	"fmt"
	"os"
	"strings"

	"github.com/tbui/yt-studio/cmd/server/internal/dotenv"
)

// The file, and the variable that points somewhere else. The override is an
// environment variable because a flag cannot be read before the parse it feeds.
const (
	defaultEnvFile = ".env"
	envFileVar     = "YTS_ENV_FILE"
)

// envFileLoaded and envVarsLoaded record what the load did, so serve can report
// it once it has a logger. Package state because kong owns the command, so main
// has nowhere to hand a local to.
var (
	envFileLoaded string
	envVarsLoaded int
)

// loadEnvFile applies the environment file, if there is one. A missing default
// is silence, since production sets real variables; a missing file asked for by
// name is an error, because that is a typo rather than a configuration.
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
