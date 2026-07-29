package app

import (
	"context"
	"errors"
	"strings"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
	"github.com/tbui/yt-studio/domain/scheduler"
)

// UpdateChapterScript records an operator's edit to a chapter's narration (§9).
//
// It still does not re-run anything — that decision stays with the operator —
// but it no longer leaves the consequences invisible. The narration and clip
// below this chapter were derived from the text that has just been replaced, so
// they are flagged stale. Before this, an edited script and the audio of the
// old one sat side by side with nothing anywhere saying they disagreed.
func UpdateChapterScript(
	ctx context.Context,
	chapters repository.ChapterReader,
	fields repository.ChapterFieldWriter,
	notifier ChapterNotifier,
	marker StaleMarker,
	id entity.ChapterID,
	script string,
	estimateSeconds func(string) float64,
) (entity.Chapter, error) {
	script = strings.TrimSpace(script)
	if script == "" {
		return entity.Chapter{}, Invalid("script", "must not be empty")
	}
	c, err := chapters.ChapterByID(ctx, id)
	if err != nil {
		return entity.Chapter{}, err
	}
	duration := 0.0
	if estimateSeconds != nil {
		duration = estimateSeconds(script)
	}
	if err := fields.SetChapterScript(ctx, id, script, duration); err != nil {
		return entity.Chapter{}, err
	}
	c.Script = script
	c.DurationSeconds = duration

	// Seeded on the chapter's own script task: the edit replaces that task's
	// output, so everything below it is questionable while the script task
	// itself is not — the operator's text is now the source of truth.
	if marker != nil {
		seed := entity.NewTaskID(c.VideoID, entity.TaskKindScript, c.Ordinal, -1)
		if _, err := marker.MarkStale(ctx, c.VideoID, []entity.TaskID{seed}); err != nil {
			// A video that is not in the scheduler's memory has nothing running
			// to invalidate, and the edit itself is already committed.
			if !errors.Is(err, scheduler.ErrUnknownVideo) && !errors.Is(err, scheduler.ErrUnknownTask) {
				return entity.Chapter{}, err
			}
		}
	}

	if notifier != nil {
		notifier.NotifyChapter(chapterDelta(c))
	}
	return c, nil
}

// chapterDelta projects a chapter for the SSE stream.
func chapterDelta(c entity.Chapter) entity.ChapterDelta {
	return entity.ChapterDelta{
		ID:            c.ID,
		VideoID:       c.VideoID,
		Ordinal:       c.Ordinal,
		Title:         c.Title,
		HasScript:     c.Script != "",
		AudioAssetID:  c.AudioAssetID,
		ImageAssetIDs: c.ImageAssetIDs,
		ClipAssetID:   c.ClipAssetID,
		UpdatedAt:     c.UpdatedAt,
	}
}
