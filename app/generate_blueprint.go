package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// GenerateBlueprint produces a video's chapter outline and materialises its
// chapters. It is the root of the DAG; whether the pipeline then parks on a
// gate is the scheduler's decision, not this function's.
//
// The count that comes back is the video's real one — the number it was created
// with was only the brief — so anything inside the tolerance band is accepted
// as written and the DAG is later built for it.
//
//nolint:revive // the parameter list is the dependency list
func GenerateBlueprint(
	ctx context.Context,
	t entity.Task,
	videos repository.VideoReader,
	channels repository.ChannelReader,
	llm provider.LLM,
	chapterWriter repository.ChapterWriter,
	videoFields repository.VideoFieldWriter,
	assets repository.AssetWriter,
	store provider.AssetStore,
	notifier ChapterNotifier,
	tolerancePercent int,
	now time.Time,
) entity.TaskOutcome {
	video, err := videos.VideoByID(ctx, t.VideoID)
	if err != nil {
		return classify(err)
	}
	channel, err := channels.ChannelByID(ctx, video.ChannelID)
	if err != nil {
		return classify(err)
	}

	bp, err := llm.Blueprint(ctx, provider.BlueprintRequest{
		VideoID:               video.ID,
		VideoRef:              video.Ref,
		ChannelSlug:           channel.Slug,
		Title:                 video.Title,
		Topic:                 video.Topic,
		ChapterCount:          video.ChapterCount,
		TargetDurationMinutes: video.TargetDurationMinutes,
	})
	if err != nil {
		return classify(fmt.Errorf("generate blueprint: %w", err))
	}
	minChapters, maxChapters := entity.ChapterCountBand(video.ChapterCount, tolerancePercent)
	if n := len(bp.Chapters); n < minChapters || n > maxChapters {
		// Retryable: how far a roll lands from the brief is a property of the
		// roll, not of the video, and MaxAttempts bounds the asking.
		return entity.Failed{
			Err: fmt.Errorf("%w: blueprint returned %d chapters, want %d..%d for a target of %d",
				ErrBlueprintOffTarget, n, minChapters, maxChapters, video.ChapterCount),
			Retryable: true,
		}
	}

	if _, err := RecordAsset(ctx, assets, store, bp.AssetID, entity.AssetKindBlueprint,
		video.ID, nil, "llm.blueprint", now); err != nil {
		return classify(err)
	}
	if err := videoFields.SetVideoBlueprintAsset(ctx, video.ID, bp.AssetID); err != nil {
		return classify(err)
	}

	chapters := make([]entity.Chapter, 0, len(bp.Chapters))
	for i, bc := range bp.Chapters {
		// Ordinals are renumbered 1..N rather than taken from the response: they
		// are the index every task id derives from, so a gap or a repeat would put
		// the DAG and the chapter table out of correspondence. This is the only
		// moment renumbering is free — no chapter work has run.
		c, err := entity.NewChapter(video.ID, i+1, bc.Title, bc.Summary, now)
		if err != nil {
			return classify(err)
		}
		// Pre-sized so a chapter's slide tasks each write their own index atomically.
		c.SlideAssetIDs = make([]entity.AssetID, video.SlidesPerChapter)
		c.SlidePrompts = make([]string, 0, video.SlidesPerChapter)
		c.EstimatedWords = bc.EstimatedWords
		chapters = append(chapters, c)
	}
	if err := chapterWriter.ReplaceChapters(ctx, video.ID, chapters); err != nil {
		return classify(err)
	}
	if notifier != nil {
		for _, c := range chapters {
			notifier.NotifyChapter(chapterDelta(c))
		}
	}
	return entity.Success{Assets: []entity.AssetID{bp.AssetID}}
}
