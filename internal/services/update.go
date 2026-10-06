package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"

	"github.com/tbui/yt-studio/internal/fswatch"
	"github.com/tbui/yt-studio/internal/update"
)

// UpdateEvent is emitted when the app's update state changes.
const UpdateEvent = "update:state"

// Update states.
const (
	// UpdateDisabled: a development build, which doesn't update.
	UpdateDisabled = "disabled"
	// UpdateIdle: up to date.
	UpdateIdle = "idle"
	// UpdatePreparing: unpacking and verifying a new version.
	UpdatePreparing = "preparing"
	// UpdateReady: a new version is ready; restarting installs it.
	UpdateReady = "ready"
	// UpdateFailed: the last check or preparation failed.
	UpdateFailed = "failed"
)

// UpdateState is where updating stands. It's the payload of UpdateEvent.
type UpdateState struct {
	State string `json:"state"`
	// Current is the running version.
	Current string `json:"current"`
	// Version is the new version (preparing, ready).
	Version string `json:"version,omitempty"`
	// Notes are the new version's changes (ready).
	Notes []string `json:"notes,omitempty"`
	// Error says what failed (failed; ready, when restarting failed).
	Error string `json:"error,omitempty"`
}

// Installer finds, stages and installs new versions (the Wails updater).
type Installer interface {
	Check(ctx context.Context) (*updater.Release, error)
	DownloadAndInstall(ctx context.Context) error
	Restart(ctx context.Context) error
}

// UpdateService keeps a new version ready as soon as one is released, and
// restarts into it when asked.
type UpdateService struct {
	installer Installer
	emit      Emitter
	logger    *slog.Logger
	poke      chan struct{}
	checking  sync.Mutex

	mu    sync.Mutex
	state UpdateState
}

// NewUpdateService creates an UpdateService for the running version; a nil
// installer disables updating.
func NewUpdateService(installer Installer, current string, emit Emitter, logger *slog.Logger) *UpdateService {
	s := &UpdateService{installer: installer, emit: emit, logger: logger, poke: make(chan struct{}, 1)}
	s.state = UpdateState{State: UpdateIdle, Current: current}
	if installer == nil {
		s.state.State = UpdateDisabled
	}
	return s
}

// RunUpdates checks for a new version now and whenever a release is
// published, until ctx ends.
func RunUpdates(ctx context.Context, s *UpdateService) {
	if s.installer == nil {
		return
	}
	s.recheck()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.poke:
			s.check(ctx)
		}
	}
}

// recheck asks RunUpdates to check again (while one is pending, more
// coalesce).
func (s *UpdateService) recheck() {
	select {
	case s.poke <- struct{}{}:
	default:
	}
}

// WatchReleases has s check whenever a release is published in dir. Close
// the watcher when done.
func WatchReleases(s *UpdateService, dir string) (*fswatch.Watcher, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return fswatch.Watch(dir, 200*time.Millisecond, func(c fswatch.Change) {
		if c.Rescan || slices.Contains(c.Paths, update.ManifestName) {
			s.recheck()
		}
	})
}

// State returns where updating stands.
func (s *UpdateService) State() UpdateState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Check checks for a new version now and prepares it; it returns the
// resulting state.
func (s *UpdateService) Check(ctx context.Context) UpdateState {
	if s.installer != nil {
		s.check(ctx)
	}
	return s.State()
}

// Restart quits and relaunches into the new version. Videos in flight resume
// on the next launch.
func (s *UpdateService) Restart(ctx context.Context) error {
	st := s.State()
	if st.State != UpdateReady {
		return errors.New("no update is ready")
	}
	if err := s.installer.Restart(ctx); err != nil {
		s.logger.Error("restart into update failed", "version", st.Version, "err", err)
		st.Error = fmt.Sprintf("Couldn't restart into %s: %v", st.Version, err)
		s.set(st)
		return err
	}
	return nil
}

func (s *UpdateService) check(ctx context.Context) {
	s.checking.Lock()
	defer s.checking.Unlock()
	cur := s.State()
	rel, err := s.installer.Check(ctx)
	switch {
	case err != nil:
		s.logger.Warn("update check failed", "err", err)
		if cur.State != UpdateReady {
			s.set(UpdateState{State: UpdateFailed, Error: err.Error()})
		}
		return
	case rel == nil:
		if cur.State != UpdateReady {
			s.set(UpdateState{State: UpdateIdle})
		}
		return
	case cur.State == UpdateReady && cur.Version == rel.Version:
		return // already staged
	}

	start := time.Now()
	s.set(UpdateState{State: UpdatePreparing, Version: rel.Version})
	if err := s.installer.DownloadAndInstall(ctx); err != nil {
		s.logger.Warn("preparing update failed", "version", rel.Version, "err", err)
		s.set(UpdateState{State: UpdateFailed, Version: rel.Version, Error: err.Error()})
		return
	}
	s.logger.Info("update ready", "version", rel.Version, "took", time.Since(start).Round(time.Millisecond))
	s.set(UpdateState{State: UpdateReady, Version: rel.Version, Notes: update.Notes(rel)})
}

func (s *UpdateService) set(st UpdateState) {
	s.mu.Lock()
	st.Current = s.state.Current
	changed := !equalUpdateState(s.state, st)
	s.state = st
	s.mu.Unlock()
	if changed {
		s.emit.Emit(UpdateEvent, st)
	}
}

func equalUpdateState(a, b UpdateState) bool {
	return a.State == b.State && a.Version == b.Version && a.Error == b.Error && slices.Equal(a.Notes, b.Notes)
}
