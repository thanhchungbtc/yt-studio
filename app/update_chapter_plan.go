package app

import (
	"context"
	"strings"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// ChapterPlan is what one chapter of a blueprint promises: what it covers, and
// how much of the video's spoken-word budget it gets to cover it in.
type ChapterPlan struct {
	Title          string
	Summary        string
	EstimatedWords int
}

// UpdateChapterPlan records an operator's edit to the blueprint.
//
// It is a write and nothing else. Nothing re-runs, nothing is flagged stale.
//
// That is not a simplification, it is how the plan is already read: no task ever
// reads the stored blueprint asset — that file is a receipt of what the model
// returned. What a script, the metadata and the thumbnail plan are all written
// against is the chapter table, projected by blueprintOutline. So an edit made
// before a chapter's script has run is simply the plan that runs, and approving
// a gate expands the DAG from these rows rather than from the asset.
//
// Once a script exists the edit no longer reaches it, and whether that script
// is now wrong enough to rewrite is a judgement about the words in it. That
// belongs to the operator, who has RetryChapter to act on it.
func UpdateChapterPlan(
	ctx context.Context,
	chapters repository.ChapterReader,
	fields repository.ChapterFieldWriter,
	notifier ChapterNotifier,
	id entity.ChapterID,
	plan ChapterPlan,
) (entity.Chapter, error) {
	title := strings.TrimSpace(plan.Title)
	if title == "" {
		return entity.Chapter{}, Invalid("title", "must not be empty")
	}
	if plan.EstimatedWords < 0 {
		return entity.Chapter{}, Invalid("estimatedWords", "must not be negative")
	}
	summary := strings.TrimSpace(plan.Summary)

	c, err := chapters.ChapterByID(ctx, id)
	if err != nil {
		return entity.Chapter{}, err
	}
	if err := fields.SetChapterPlan(ctx, id, title, summary, plan.EstimatedWords); err != nil {
		return entity.Chapter{}, err
	}
	c.Title = title
	c.Summary = summary
	c.EstimatedWords = plan.EstimatedWords

	// DurationSeconds is deliberately untouched. It is what the narration came
	// to, measured from the script; the projection from the new budget is the
	// caller's arithmetic — NarrationSeconds — and overwriting a measurement
	// with an estimate would make the two indistinguishable afterwards.
	if notifier != nil {
		notifier.NotifyChapter(chapterDelta(c))
	}
	return c, nil
}
