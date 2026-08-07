package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// RegenerateChapterSlide rewrites one slide prompt and re-runs the slide it
// describes. The two are one operation so the stored text is always what drew
// the current image, or what is drawing it now.
//
// The prompt is written first because GenerateSlide reads it at dispatch. If
// the scheduler then refuses the re-run, the edit stands and a second press
// resolves it. The coalesced prompt batch is left alone: a slide task never
// reads it, and re-running the prompt task overwrites this edit anyway.
//
//nolint:revive // the parameter list is the dependency list
func RegenerateChapterSlide(
	ctx context.Context,
	chapters repository.ChapterReader,
	fields repository.ChapterFieldWriter,
	rerunner TaskRerunner,
	notifier ChapterNotifier,
	id entity.ChapterID,
	index int,
	prompt string,
) (entity.Chapter, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return entity.Chapter{}, Invalid("prompt", "must not be empty")
	}
	if index < 0 {
		return entity.Chapter{}, Invalid("index", "must not be negative")
	}
	c, err := chapters.ChapterByID(ctx, id)
	if err != nil {
		return entity.Chapter{}, err
	}
	// Refused rather than appended: json_set would grow the array past the image
	// width the DAG was expanded with, leaving a prompt no task will ever read.
	if index >= len(c.SlidePrompts) {
		return entity.Chapter{}, Invalid("index", fmt.Sprintf(
			"chapter %d has %d prompts", c.Ordinal, len(c.SlidePrompts)))
	}

	if err := fields.SetChapterPrompt(ctx, id, index, prompt); err != nil {
		return entity.Chapter{}, err
	}
	c.SlidePrompts[index] = prompt

	// Rerun rather than RetryTask: the clip and render built from the old slide
	// keep their artifacts and are flagged rather than thrown away. Nothing below
	// a failed slide ever ran, so nothing below it gets flagged either.
	seed := entity.NewTaskID(c.VideoID, entity.TaskKindSlide, c.Ordinal, index)
	if _, err := rerunner.Rerun(ctx, c.VideoID, []entity.TaskID{seed}, false); err != nil {
		return entity.Chapter{}, err
	}

	if notifier != nil {
		notifier.NotifyChapter(chapterDelta(c))
	}
	return c, nil
}
