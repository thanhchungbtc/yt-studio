package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// Clip composes exactly one chapter: its slides dissolved across its narration,
// keyed onto the chalkboard under the chapter and video titles.
//
// The result runs for the narration plus a head pad, a tail pad and the beat at
// the end, so the concat has material to crossfade with on both sides.
func (c *Composer) Clip(ctx context.Context, req provider.ClipRequest) (entity.AssetID, error) {
	if err := c.Check(); err != nil {
		return "", err
	}
	if len(req.SlideAssetIDs) == 0 {
		return "", errors.New("ffmpeg composer: a clip needs at least one slide")
	}
	if req.AudioAssetID == "" {
		return "", errors.New("ffmpeg composer: a clip needs narration")
	}

	audioPath, err := c.store.Path(req.AudioAssetID, entity.AssetKindAudio)
	if err != nil {
		return "", fmt.Errorf("resolve audio %s: %w", req.AudioAssetID.Short(), err)
	}
	slidePaths, err := c.inputPaths(req.SlideAssetIDs, entity.AssetKindImage)
	if err != nil {
		return "", err
	}
	narration, err := audioDuration(ctx, audioPath)
	if err != nil {
		return "", err
	}

	log := c.log.With(
		slog.String("video_id", req.VideoID.String()),
		slog.Int("ordinal", req.Ordinal))
	log.Info("composing chapter",
		slog.String("title", req.ChapterTitle),
		slog.Float64("narration_s", narration),
		slog.Int("slides", len(slidePaths)))

	dir, cleanup, err := c.tempDir("clip")
	if err != nil {
		return "", err
	}
	defer cleanup()

	// Every slide gets an equal share of the narration. The crossfades overlap, so
	// each one has to be long enough to pay for its own transition.
	n := len(slidePaths)
	slideDuration := (narration + float64(n-1)*slideCrossfade) / float64(n)

	slideClips, err := c.encodeSlides(ctx, slidePaths, slideDuration, dir)
	if err != nil {
		return "", err
	}
	slideshow := filepath.Join(dir, "raw_chapter.mp4")
	if err := c.dissolveAndMux(ctx, slideClips, audioPath, slideshow); err != nil {
		return "", err
	}
	total := narration + 2*chapterCrossfade + chapterTailPad
	output := filepath.Join(dir, "chapter.mp4")
	if err := c.composite(ctx, slideshow, req, total, output); err != nil {
		return "", err
	}
	return c.ingest(ctx, entity.AssetKindClip, output)
}

// encodeSlides turns each slide into a fixed-length silent clip.
//
// They are independent files, so they encode concurrently — a chapter with
// several slides otherwise spends most of its wall clock waiting on process
// startup, one slide at a time.
func (c *Composer) encodeSlides(ctx context.Context, slidePaths []string, duration float64, dir string) ([]string, error) {
	clips := make([]string, len(slidePaths))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(c.lanes)
	for i, slide := range slidePaths {
		clips[i] = filepath.Join(dir, fmt.Sprintf("img_%02d.mp4", i))
		g.Go(func() error {
			args := []string{
				"-loop", "1",
				"-r", "1",
				"-i", slide,
				"-vf", "format=yuv420p",
				"-r", itoa(videoFPS),
				"-t", f6(duration),
				"-an",
			}
			args = append(args, encodeArgs()...)
			return c.run(gctx, append(args, clips[i])...)
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return clips, nil
}

// dissolveAndMux crossfades the slide clips into one track and attaches the
// narration. A single slide needs no transition, so its video is copied through
// rather than re-encoded.
func (c *Composer) dissolveAndMux(ctx context.Context, clips []string, audio, output string) error {
	if len(clips) == 1 {
		return c.run(ctx,
			"-i", clips[0],
			"-i", audio,
			"-map", "0:v",
			"-map", "1:a",
			"-c:v", "copy",
			"-c:a", "aac",
			"-b:a", audioBitrate,
			output)
	}

	durations, err := probeAll(ctx, clips, c.lanes*2)
	if err != nil {
		return err
	}

	args := make([]string, 0, len(clips)*2+16)
	for _, clip := range clips {
		args = append(args, "-i", clip)
	}
	args = append(args,
		"-i", audio,
		"-filter_complex", slideXfadeGraph(durations),
		"-map", "[vout]",
		"-map", itoa(len(clips))+":a")
	args = append(args, encodeArgs()...)
	args = append(args, "-c:a", "aac", "-b:a", audioBitrate, output)
	return c.run(ctx, args...)
}

// composite lays the slideshow onto the chalkboard, draws the titles and pads
// both ends.
func (c *Composer) composite(ctx context.Context, slideshow string, req provider.ClipRequest, total float64, output string) error {
	title := strings.ToUpper(req.ChapterTitle)
	layout := layOutTitles(c.fitFontSize(title, titleMaxWidth))

	fontFile := c.res.TitleFont
	if _, err := os.Stat(fontFile); err != nil {
		fontFile = ""
	}

	args := make([]string, 0, 24)
	args = append(args,
		"-loop", "1",
		"-i", c.res.Chalkboard,
		"-i", slideshow,
		"-filter_complex", chapterCompositeGraph(title, req.VideoTitle, fontFile, layout, total),
		"-map", "[out_padded]",
		"-map", "[aout]",
		"-r", itoa(videoFPS))
	args = append(args, encodeArgs()...)
	args = append(args, "-c:a", "aac", "-b:a", audioBitrate, "-t", f3(total), output)
	return c.run(ctx, args...)
}
