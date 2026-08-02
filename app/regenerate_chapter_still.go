package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// RegenerateChapterStill rewrites one image prompt and re-runs the one still it
// describes.
//
// The write and the re-run are deliberately a single operation. A prompt that
// had been saved but not generated from would be a third thing to explain — the
// text beside a still would no longer be the text that drew it — and the
// operator would have to remember which of the two they were looking at.
// Because this is the only hand write to a prompt, the stored text is always
// what produced the current image, or what is producing it right now.
//
// The prompt is written first because GenerateStill reads it when the task is
// dispatched rather than when the re-run is asked for. If the scheduler then
// refuses the re-run the edit stands and nothing has run, which is the state a
// second press of the same button resolves.
//
// The video's coalesced prompt batch is deliberately left alone: an image task
// never reads it, and dropping it here would only make some later re-run of the
// prompt task cost an LLM call. That re-run overwrites this edit either way —
// it is the operator's own instruction to go back to generated prompts.
//
//nolint:revive // the parameter list is the dependency list
func RegenerateChapterStill(
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
	if index >= len(c.ImagePrompts) {
		return entity.Chapter{}, Invalid("index", fmt.Sprintf(
			"chapter %d has %d prompts", c.Ordinal, len(c.ImagePrompts)))
	}

	if err := fields.SetChapterPrompt(ctx, id, index, prompt); err != nil {
		return entity.Chapter{}, err
	}
	c.ImagePrompts[index] = prompt

	// Rerun rather than RetryTask: the clip built from the old still, and the
	// render built from that clip, keep their artifacts and are flagged for the
	// operator instead of being thrown away. It is the same call the tile's
	// re-run makes, and it holds for a still that failed just as well as one that
	// succeeded — nothing below a failure ever ran, so nothing below it is
	// flagged.
	seed := entity.NewTaskID(c.VideoID, entity.TaskKindImage, c.Ordinal, index)
	if _, err := rerunner.Rerun(ctx, c.VideoID, []entity.TaskID{seed}, false); err != nil {
		return entity.Chapter{}, err
	}

	if notifier != nil {
		notifier.NotifyChapter(chapterDelta(c))
	}
	return c, nil
}
