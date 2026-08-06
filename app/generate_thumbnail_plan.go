package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// GenerateThumbnailPlan writes the grid the thumbnail is built from: one
// caption and one icon prompt per cell.
//
// It runs after the metadata because it is handed the hook, so the captions can
// say something the headline does not already say.
//
//nolint:revive // the parameter list is the dependency list
func GenerateThumbnailPlan(
	ctx context.Context,
	t entity.Task,
	videos repository.VideoReader,
	chapters repository.ChapterReader,
	llm provider.LLM,
	videoFields repository.VideoFieldWriter,
	assets repository.AssetWriter,
	store provider.AssetStore,
	now time.Time,
) entity.TaskOutcome {
	video, err := videos.VideoByID(ctx, t.VideoID)
	if err != nil {
		return classify(err)
	}
	if video.Metadata == nil {
		return entity.Failed{Err: fmt.Errorf("%w: video has no metadata", ErrValidation), Retryable: true}
	}
	rows, err := chapters.ListChaptersByVideo(ctx, video.ID)
	if err != nil {
		return classify(err)
	}

	outline := make([]provider.BlueprintChapter, 0, len(rows))
	for _, c := range rows {
		outline = append(outline, provider.BlueprintChapter{
			Ordinal:        c.Ordinal,
			Title:          c.Title,
			Summary:        c.Summary,
			EstimatedWords: c.EstimatedWords,
		})
	}

	plan, err := llm.ThumbnailPlan(ctx, provider.ThumbnailPlanRequest{
		VideoID:  video.ID,
		VideoRef: video.Ref,
		Blueprint: provider.BlueprintOutline{
			Title:    video.Metadata.Title,
			Summary:  video.Topic,
			Chapters: outline,
		},
		Headline: video.Metadata.ThumbnailText,
		Cells:    video.ThumbnailCells,
	})
	if err != nil {
		return classify(fmt.Errorf("plan thumbnail for %s: %w", video.Ref, err))
	}

	cells, err := normaliseCells(plan.Plan.Cells, video.ThumbnailCells)
	if err != nil {
		return classify(err)
	}

	if _, err := RecordAsset(ctx, assets, store, plan.AssetID, entity.AssetKindThumbnailPlan,
		video.ID, nil, "llm.thumbnail_plan", now); err != nil {
		return classify(err)
	}
	if err := videoFields.SetVideoThumbnailPlan(ctx, video.ID, entity.ThumbnailPlan{Cells: cells}); err != nil {
		return classify(err)
	}
	return entity.Success{Assets: []entity.AssetID{plan.AssetID}}
}

// normaliseCells tidies what the model returned and holds it to the one thing
// that is not negotiable: the count.
//
// The graph already holds one icon task per cell by the time this runs, and it
// cannot grow — a short plan leaves tasks with no prompt to read. That is a bad
// roll rather than bad input, so it is retried, the same way an off-target
// blueprint is. A long one is simply cut: re-rolling a model that was generous
// costs more than dropping the surplus.
func normaliseCells(cells []entity.ThumbnailCell, want int) ([]entity.ThumbnailCell, error) {
	if len(cells) < want {
		return nil, fmt.Errorf("%w: plan has %d cells, the grid has %d",
			ErrThumbnailPlanOffTarget, len(cells), want)
	}
	out := make([]entity.ThumbnailCell, 0, want)
	for i, c := range cells[:want] {
		caption := strings.Join(strings.Fields(c.Caption), " ")
		prompt := strings.TrimSpace(c.Prompt)
		if caption == "" {
			return nil, fmt.Errorf("%w: cell %d has no caption", ErrThumbnailPlanOffTarget, i)
		}
		if prompt == "" {
			return nil, fmt.Errorf("%w: cell %d has no prompt", ErrThumbnailPlanOffTarget, i)
		}
		out = append(out, entity.ThumbnailCell{Caption: caption, Prompt: prompt})
	}
	return out, nil
}
