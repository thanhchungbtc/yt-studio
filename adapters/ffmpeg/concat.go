package ffmpeg

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// Concat joins every chapter clip into the final render: the chapters
// crossfaded into each other, laid over the looping background with the music
// mixed under the narration.
//
// The clips arrive in ordinal order and every one of them is re-encoded, which
// is unavoidable here — a crossfade and an overlay both rewrite pixels, so
// there is no copy path to take.
func (c *Composer) Concat(ctx context.Context, req provider.ConcatRequest) (entity.AssetID, error) {
	if err := c.Check(); err != nil {
		return "", err
	}
	if len(req.ClipAssetIDs) == 0 {
		return "", errors.New("ffmpeg composer: a concat needs at least one clip")
	}

	clips, err := c.inputPaths(req.ClipAssetIDs, entity.AssetKindClip)
	if err != nil {
		return "", err
	}
	durations, err := probeAll(ctx, clips, c.lanes*2)
	if err != nil {
		return "", err
	}

	// Each crossfade consumes a second of two chapters at once, so the finished
	// render is shorter than the sum of its parts.
	total := 0.0
	for _, d := range durations {
		total += d
	}
	total -= max(0, float64(len(clips)-1)) * chapterCrossfade

	log := c.log.With(slog.String("video_id", req.VideoID.String()))
	log.Info("composing final render",
		slog.Int("clips", len(clips)),
		slog.Float64("total_s", total))

	dir, cleanup, err := c.tempDir("concat")
	if err != nil {
		return "", err
	}
	defer cleanup()

	args := []string{
		"-stream_loop", "-1",
		"-t", f3(total),
		"-i", c.res.BgVideo,
	}
	for _, clip := range clips {
		args = append(args, "-i", clip)
	}
	// The music is optional: without it the narration is mapped straight out.
	bgMusicIndex := 0
	if _, err := os.Stat(c.res.BgMusic); err == nil {
		bgMusicIndex = 1 + len(clips)
		args = append(args,
			"-stream_loop", "-1",
			"-t", f3(total),
			"-i", c.res.BgMusic)
	}

	audioMap := "[narration]"
	if bgMusicIndex > 0 {
		audioMap = "[final_a]"
	}
	output := filepath.Join(dir, "final.mp4")
	args = append(args,
		"-filter_complex", concatGraph(durations, bgMusicIndex),
		"-map", "[final_v]",
		"-map", audioMap,
		"-r", itoa(videoFPS))
	args = append(args, encodeArgs()...)
	args = append(args, "-c:a", "aac", "-b:a", audioBitrate, output)

	onPercent := func(pct int) { log.Debug("final render", slog.Int("percent", pct)) }
	if err := c.runProgress(ctx, total, onPercent, args...); err != nil {
		return "", err
	}
	return c.ingest(ctx, entity.AssetKindFinal, output)
}

func itoa(v int) string { return strconv.Itoa(v) }
