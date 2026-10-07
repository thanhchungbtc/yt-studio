//go:build !darwin

package fswatch

import (
	"errors"
	"time"
)

type stream struct{}

func start(string, time.Duration, func([]string, bool)) (*stream, error) {
	return nil, errors.New("file watching is not available on this system")
}

func startDirs([]string, time.Duration, func([]string, bool)) (*stream, error) {
	return nil, errors.New("file watching is not available on this system")
}

func (s *stream) stop() {}
