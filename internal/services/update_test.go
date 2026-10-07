package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"

	"github.com/tbui/yt-studio/internal/update"
)

// fakeInstaller offers release (nil: up to date) and records what it's
// asked to do.
type fakeInstaller struct {
	mu         sync.Mutex
	release    *updater.Release
	checkErr   error
	restartErr error
	downloads  int
	restarts   int
}

func (f *fakeInstaller) Check(context.Context) (*updater.Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.release, f.checkErr
}

func (f *fakeInstaller) DownloadAndInstall(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.downloads++
	return nil
}

func (f *fakeInstaller) Restart(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restarts++
	return f.restartErr
}

func release(version string, notes ...string) *updater.Release {
	return &updater.Release{Version: version, Metadata: map[string]any{"notes": notes}}
}

func newTestUpdates(inst Installer) (*UpdateService, *syncEmitter) {
	emit := &syncEmitter{}
	return NewUpdateService(inst, "0.1.0", emit, slog.New(slog.NewTextHandler(io.Discard, nil))), emit
}

func (e *syncEmitter) states() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []string
	for _, ev := range e.events {
		if ev.name == UpdateEvent {
			if st, ok := ev.data.(UpdateState); ok {
				out = append(out, st.State)
			}
		}
	}
	return out
}

func TestUpdatesDisabledWithoutInstaller(t *testing.T) {
	s, _ := newTestUpdates(nil)
	if st := s.Check(context.Background()); st.State != UpdateDisabled || st.Current != "0.1.0" {
		t.Fatalf("state = %+v", st)
	}
	if err := s.Restart(context.Background()); err == nil {
		t.Fatal("restart without an update")
	}
}

func TestUpdatePreparedOnceAndKept(t *testing.T) {
	inst := &fakeInstaller{}
	s, emit := newTestUpdates(inst)
	ctx := context.Background()

	if st := s.Check(ctx); st.State != UpdateIdle {
		t.Fatalf("up to date: %+v", st)
	}
	inst.release = release("0.1.1", "Faster startup")
	st := s.Check(ctx)
	if st.State != UpdateReady || st.Version != "0.1.1" || st.Current != "0.1.0" || !slices.Equal(st.Notes, []string{"Faster startup"}) {
		t.Fatalf("ready: %+v", st)
	}
	if got := emit.states(); !slices.Equal(got, []string{UpdatePreparing, UpdateReady}) {
		t.Fatalf("events = %v", got)
	}

	// Checking again doesn't prepare it again, nor does a failing check
	// (or the release going away) lose it.
	s.Check(ctx)
	inst.checkErr = errors.New("unreadable manifest")
	s.Check(ctx)
	inst.checkErr, inst.release = nil, nil
	if st := s.Check(ctx); st.State != UpdateReady || inst.downloads != 1 {
		t.Fatalf("after rechecks: %+v, %d downloads", st, inst.downloads)
	}

	// A newer release replaces it.
	inst.release = release("0.1.2")
	if st := s.Check(ctx); st.State != UpdateReady || st.Version != "0.1.2" || inst.downloads != 2 {
		t.Fatalf("newer: %+v, %d downloads", st, inst.downloads)
	}
}

func TestUpdateCheckFailure(t *testing.T) {
	inst := &fakeInstaller{checkErr: errors.New("boom")}
	s, _ := newTestUpdates(inst)
	if st := s.Check(context.Background()); st.State != UpdateFailed || st.Error != "boom" {
		t.Fatalf("state = %+v", st)
	}
}

func TestRestart(t *testing.T) {
	inst := &fakeInstaller{release: release("0.1.1")}
	s, _ := newTestUpdates(inst)
	if err := s.Restart(context.Background()); err == nil {
		t.Fatal("restart before an update is ready")
	}
	s.Check(context.Background())
	if err := s.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	if inst.restarts != 1 {
		t.Fatalf("restarts = %d", inst.restarts)
	}
}

func TestFailedRestartSaysSo(t *testing.T) {
	inst := &fakeInstaller{release: release("0.1.1"), restartErr: errors.New("helper failed")}
	s, _ := newTestUpdates(inst)
	s.Check(context.Background())
	if err := s.Restart(context.Background()); err == nil {
		t.Fatal("failed restart reported success")
	}
	if st := s.State(); st.State != UpdateReady || st.Error == "" {
		t.Fatalf("state = %+v", st)
	}
}

func TestPublishingAReleaseTriggersACheck(t *testing.T) {
	dir := t.TempDir()
	inst := &fakeInstaller{}
	s, emit := newTestUpdates(inst)
	w, err := WatchReleases(s, dir)
	if err != nil {
		t.Skip("file events unavailable:", err)
	}
	defer w.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go RunUpdates(ctx, s)

	inst.mu.Lock()
	inst.release = release("0.1.1")
	inst.mu.Unlock()
	if err := update.WriteManifest(dir, update.Manifest{Version: "0.1.1", File: update.ZipName("0.1.1"), SHA256: strings.Repeat("a", 64)}); err != nil {
		t.Fatal(err)
	}
	emit.waitFor(t, func(e recordedEvent) bool {
		st, ok := e.data.(UpdateState)
		return ok && st.State == UpdateReady && st.Version == "0.1.1"
	})
}
