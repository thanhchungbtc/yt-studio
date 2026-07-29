package app

import (
	"context"
	"strings"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// UpdateChapterScript records an operator's edit to a chapter's narration (§9).
//
// It does not itself re-run anything: the operator decides whether to retry the
// chapter afterwards, which is what RetryChapter is for.
func UpdateChapterScript(
	ctx context.Context,
	chapters repository.ChapterReader,
	fields repository.ChapterFieldWriter,
	notifier ChapterNotifier,
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
