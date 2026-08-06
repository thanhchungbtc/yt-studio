package ffmpeg

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
)

// stderrTail is how much of a failed run's stderr is kept. ffmpeg reports the
// cause in its last few lines; the rest is noise.
const stderrTail = 16 << 10

// run invokes ffmpeg and waits. The argv prefix matches the reference pipeline
// exactly, so a logged command is a runnable command.
func (c *Composer) run(ctx context.Context, args ...string) error {
	argv := append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)
	return c.exec(ctx, argv, nil, 0)
}

// runProgress invokes ffmpeg with machine-readable progress on stdout. -nostats
// suppresses the interactive line that would otherwise be interleaved on
// stderr.
func (c *Composer) runProgress(ctx context.Context, totalSeconds float64, onPercent func(int), args ...string) error {
	argv := append([]string{
		"-hide_banner", "-loglevel", "error",
		"-progress", "pipe:1", "-nostats",
		"-y",
	}, args...)
	return c.exec(ctx, argv, onPercent, totalSeconds)
}

// exec runs one ffmpeg process to completion.
//
// Both pipes are drained concurrently: ffmpeg blocks once a pipe buffer fills,
// and a composition that deadlocks instead of finishing holds a compose slot
// forever.
func (c *Composer) exec(ctx context.Context, argv []string, onPercent func(int), totalSeconds float64) error {
	started := time.Now()
	c.log.Debug("ffmpeg", slog.String("argv", strings.Join(argv, " ")))

	cmd := exec.CommandContext(ctx, "ffmpeg", argv...) //nolint:gosec // argv is built here, never from input
	// The context kills the process; WaitDelay bounds how long we then wait for
	// its pipes to close, so a cancelled video frees its slot promptly.
	cmd.WaitDelay = 5 * time.Second

	var tail tailWriter
	tail.limit = stderrTail
	cmd.Stderr = &tail

	var stdout io.ReadCloser
	if onPercent != nil {
		var err error
		if stdout, err = cmd.StdoutPipe(); err != nil {
			return fmt.Errorf("ffmpeg stdout: %w", err)
		}
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	var wg sync.WaitGroup
	if stdout != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			readProgress(stdout, totalSeconds, onPercent)
		}()
	}
	waitErr := cmd.Wait()
	wg.Wait()

	if waitErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("ffmpeg failed: %w\n%s", waitErr, strings.TrimSpace(tail.String()))
	}
	if onPercent != nil {
		onPercent(100)
	}
	c.log.Debug("ffmpeg done", slog.Duration("took", time.Since(started)))
	return nil
}

// readProgress translates ffmpeg's key=value stream into whole percentages,
// reporting only when the number changes.
func readProgress(r io.Reader, totalSeconds float64, onPercent func(int)) {
	last := -1
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		value, ok := strings.CutPrefix(line, "out_time_us=")
		if !ok {
			continue
		}
		micros, err := strconv.ParseInt(value, 10, 64)
		if err != nil || micros <= 0 || totalSeconds <= 0 {
			continue
		}
		pct := min(99, int(float64(micros)/(totalSeconds*1_000_000)*100))
		if pct > last {
			last = pct
			onPercent(pct)
		}
	}
}

// tailWriter keeps the last limit bytes written to it and discards the rest.
type tailWriter struct {
	limit int
	buf   []byte
}

func (t *tailWriter) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - t.limit; over > 0 {
		t.buf = t.buf[over:]
	}
	return len(p), nil
}

func (t *tailWriter) String() string { return string(t.buf) }

// ingest hands a finished render to the store. The file is renamed into place
// rather than copied, which is the difference between seconds and minutes on a
// multi-gigabyte final render.
func (c *Composer) ingest(ctx context.Context, kind entity.AssetKind, path string) (entity.AssetID, error) {
	stored, err := c.store.PutFile(ctx, kind, path)
	if err != nil {
		return "", fmt.Errorf("store %s: %w", kind, err)
	}
	return stored.ID, nil
}
