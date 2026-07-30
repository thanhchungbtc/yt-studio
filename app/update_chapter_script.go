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
//
//nolint:revive // the parameter list is the dependency list
func UpdateChapterScript(
	ctx context.Context,
	chapters repository.ChapterReader,
	videos repository.VideoReader,
	channels repository.ChannelReader,
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
	// The re-timed duration has to use the same reading speed the blueprint
	// budgeted with, which is the channel's, so an edited chapter and a
	// generated one are measured the same way.
	video, err := videos.VideoByID(ctx, c.VideoID)
	if err != nil {
		return entity.Chapter{}, err
	}
	channel, err := channels.ChannelByID(ctx, video.ChannelID)
	if err != nil {
		return entity.Chapter{}, err
	}
	duration := NarrationSeconds(CountWords(script), channel.Style.WordsPerMinute)
	if err := fields.SetChapterScript(ctx, id, script, duration); err != nil {
		return entity.Chapter{}, err
	}
	c.Script = script
	c.DurationSeconds = duration

	// Seeded on the chapter's own script task: the edit replaces that task's
	// output, so everything below it is questionable but the task itself is not.
	if marker != nil {
		seed := entity.NewTaskID(c.VideoID, entity.TaskKindScript, c.Ordinal, -1)
		if _, err := marker.MarkStale(ctx, c.VideoID, []entity.TaskID{seed}); err != nil {
			// A video that is not in the scheduler's memory has nothing running to
			// invalidate, and the edit itself is already committed.
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
