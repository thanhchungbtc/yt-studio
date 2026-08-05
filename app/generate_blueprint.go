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
// chapters.
//
// It is the root of the DAG and, when the gate is enabled, the point at which
// the pipeline parks for human review. The scheduler owns that decision; this
// function just does the work and records the result.
//
// The chapter count it returns is the video's real one. The number the video
// was created with is a target the model was briefed with, and this is where
// the two are reconciled: anything inside the tolerance band is accepted as
// written, and the DAG is later built for what came back.
//
//nolint:revive // the parameter list is the dependency list
func GenerateBlueprint(
	ctx context.Context,
	t entity.Task,
	videos repository.VideoReader,
	channels repository.ChannelReader,
	llm provider.LLMProvider,
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
		// Retryable: how far a roll lands from the brief is a property of the roll,
		// not of the video, so another attempt is worth having. MaxAttempts bounds
		// how many times we ask.
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
		// Ordinals are renumbered 1..N in the order the model returned them, rather
		// than taken from the response. They are the chapter's natural key and the
		// index every task id is derived from, so a gap or a repeat would put the
		// DAG and the chapter table out of correspondence. Here is the only moment
		// renumbering is free: no chapter work has run and no asset exists yet.
		c, err := entity.NewChapter(video.ID, i+1, bc.Title, bc.Summary, now)
		if err != nil {
			return classify(err)
		}
		// The slide slots are pre-sized so that the two slide tasks of a chapter can
		// each write their own index atomically.
		c.SlideAssetIDs = make([]entity.AssetID, video.SlidesPerChapter)
		c.SlidePrompts = make([]string, 0, video.SlidesPerChapter)
		// The budget the outline assigned this chapter, carried as a field so the
		// script writer reads it rather than recovering it from prose.
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
