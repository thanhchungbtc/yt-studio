package app

import (
	"context"
	"errors"
	"strings"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
	"github.com/tbui/yt-studio/domain/scheduler"
)

// UpdateChapterScript records an operator's edit to a chapter's narration. It
// re-runs nothing — that decision stays with the operator — but the narration
// and clip derived from the replaced text are flagged stale.
func UpdateChapterScript(
	ctx context.Context,
	chapters repository.ChapterReader,
	fields repository.ChapterFieldWriter,
	notifier ChapterNotifier,
	marker StaleMarker,
	id entity.ChapterID,
	script string,
) (entity.Chapter, error) {
	script = strings.TrimSpace(script)
	if script == "" {
		return entity.Chapter{}, Invalid("script", "must not be empty")
	}
	c, err := chapters.ChapterByID(ctx, id)
	if err != nil {
		return entity.Chapter{}, err
	}
	// The blueprint's reading speed, so an edited chapter and a generated one
	// are measured the same way.
	duration := NarrationSeconds(CountWords(script))
	if err := fields.SetChapterScript(ctx, id, script, duration); err != nil {
		return entity.Chapter{}, err
	}
	c.Script = script
	c.DurationSeconds = duration

	// Seeded on the script task: the edit replaces its output, so everything
	// below is questionable but the task itself is not.
	if marker != nil {
		seed := entity.NewTaskID(c.VideoID, entity.TaskKindScript, c.Ordinal, -1)
		if _, err := marker.MarkStale(ctx, c.VideoID, []entity.TaskID{seed}); err != nil {
			// A video the scheduler does not hold has nothing to invalidate, and
			// the edit is already committed.
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
		SlideAssetIDs: c.SlideAssetIDs,
		ClipAssetID:   c.ClipAssetID,
		UpdatedAt:     c.UpdatedAt,
	}
}
