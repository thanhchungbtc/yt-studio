package ffmpeg

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"
)

// audioDuration returns the narration length in seconds.
//
// A WAV header is read directly: it is exact for the PCM the TTS backends
// produce, and it costs one open instead of a process. Anything else, or a
// malformed header, falls through to ffprobe.
func audioDuration(ctx context.Context, path string) (float64, error) {
	if strings.EqualFold(filepath.Ext(path), ".wav") {
		if d, err := wavDuration(path); err == nil {
			return d, nil
		}
	}
	if d, err := probeStream(ctx, path); err == nil {
		return d, nil
	}
	return probeDuration(ctx, path)
}

// wavDuration walks the RIFF chunks for the sample rate and the payload size.
func wavDuration(path string) (float64, error) {
	file, err := os.Open(path) //nolint:gosec // path comes from the asset store
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()

	var header [12]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		return 0, err
	}
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return 0, errors.New("not a RIFF/WAVE file")
	}

	var byteRate uint32
	var dataSize uint32
	var chunk [8]byte
	for {
		if _, err := io.ReadFull(file, chunk[:]); err != nil {
			break
		}
		id := string(chunk[0:4])
		size := binary.LittleEndian.Uint32(chunk[4:8])
		switch id {
		case "fmt ":
			buf := make([]byte, size)
			if _, err := io.ReadFull(file, buf); err != nil {
				return 0, err
			}
			if len(buf) < 16 {
				return 0, errors.New("short fmt chunk")
			}
			byteRate = binary.LittleEndian.Uint32(buf[8:12])
		case "data":
			dataSize = size
		default:
			if _, err := file.Seek(int64(size), io.SeekCurrent); err != nil {
				return 0, err
			}
			continue
		}
		if id == "data" {
			break
		}
	}
	if byteRate == 0 || dataSize == 0 {
		return 0, errors.New("incomplete WAV header")
	}
	return float64(dataSize) / float64(byteRate), nil
}

// probeStream asks for the first audio stream's duration.
func probeStream(ctx context.Context, path string) (float64, error) {
	return ffprobe(ctx, path,
		"-select_streams", "a:0",
		"-show_entries", "stream=duration")
}

// probeDuration returns a media file's container duration, for video and audio
// alike.
func probeDuration(ctx context.Context, path string) (float64, error) {
	return ffprobe(ctx, path, "-show_entries", "format=duration")
}

func ffprobe(ctx context.Context, path string, args ...string) (float64, error) {
	argv := append([]string{"-v", "error"}, args...)
	argv = append(argv, "-of", "default=noprint_wrappers=1:nokey=1", path)

	out, err := exec.CommandContext(ctx, "ffprobe", argv...).Output() //nolint:gosec // argv is built here
	if err != nil {
		return 0, fmt.Errorf("ffprobe %s: %w", filepath.Base(path), err)
	}
	value := strings.TrimSpace(string(out))
	// Multi-stream files answer once per stream; the first is the one asked for.
	if line, _, ok := strings.Cut(value, "\n"); ok {
		value = strings.TrimSpace(line)
	}
	if value == "" || value == "N/A" {
		return 0, fmt.Errorf("ffprobe %s: no duration", filepath.Base(path))
	}
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("ffprobe %s: %w", filepath.Base(path), err)
	}
	if seconds <= 0 {
		return 0, fmt.Errorf("ffprobe %s: duration is %g", filepath.Base(path), seconds)
	}
	return seconds, nil
}

// probeAll probes a list of files concurrently, preserving order. Fifty chapter
// clips are fifty process spawns; serially that is a visible pause before the
// final render even starts.
func probeAll(ctx context.Context, paths []string, lanes int) ([]float64, error) {
	durations := make([]float64, len(paths))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(lanes)
	for i, p := range paths {
		g.Go(func() error {
			d, err := probeDuration(gctx, p)
			if err != nil {
				return err
			}
			durations[i] = d
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return durations, nil
}
