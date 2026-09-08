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
	tasks repository.TaskReader,
	notifier ChapterNotifier,
	marker StaleMarker,
	resumer GraphResumer,
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
	if err := fields.SetChapterScript(ctx, id, script); err != nil {
		return entity.Chapter{}, err
	}
	c.Script = script

	// Seeded on the script task: the edit replaces its output, so everything
	// below is questionable but the task itself is not.
	if marker != nil {
		seed := entity.NewTaskID(c.VideoID, entity.TaskKindScript, c.Ordinal, -1)
		// Through readmitting, so a video the loop has forgotten is loaded back
		// and flagged rather than passed over: being finished is not a reason an
		// edit below it should go unrecorded.
		_, err := readmitting(ctx, tasks, resumer, c.VideoID, func() ([]entity.TaskID, error) {
			return marker.MarkStale(ctx, c.VideoID, []entity.TaskID{seed})
		})
		// ErrUnknownTask survives that: a graph with no script node for this
		// chapter has nothing to invalidate, and the edit is already committed.
		if err != nil && !errors.Is(err, scheduler.ErrUnknownTask) {
			return entity.Chapter{}, err
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
		ID:                   c.ID,
		VideoID:              c.VideoID,
		Ordinal:              c.Ordinal,
		Title:                c.Title,
		HasScript:            c.Script != "",
		AudioAssetID:         c.AudioAssetID,
		AudioDurationSeconds: c.AudioDurationSeconds,
		SlideAssetIDs:        c.SlideAssetIDs,
		ClipAssetID:          c.ClipAssetID,
		UpdatedAt:            c.UpdatedAt,
	}
}
