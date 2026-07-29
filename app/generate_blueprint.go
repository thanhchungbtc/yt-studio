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
// the pipeline parks for human review (§6). The scheduler owns that decision;
// this function just does the work and records the result.
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
		VideoID:      video.ID,
		VideoRef:     video.Ref,
		ChannelSlug:  channel.Slug,
		Title:        video.Title,
		Topic:        video.Topic,
		ChapterCount: video.ChapterCount,
		Style:        channel.Style,
	})
	if err != nil {
		return classify(fmt.Errorf("generate blueprint: %w", err))
	}
	if len(bp.Chapters) != video.ChapterCount {
		return entity.Failed{
			Err: fmt.Errorf("%w: blueprint returned %d chapters, the DAG was built for %d",
				ErrValidation, len(bp.Chapters), video.ChapterCount),
			Retryable: false,
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
	for _, bc := range bp.Chapters {
		c, err := entity.NewChapter(video.ID, bc.Ordinal, bc.Title, bc.Summary, now)
		if err != nil {
			return classify(err)
		}
		// The still slots are pre-sized so that the two image tasks of a chapter
		// can each write their own index atomically (§4).
		c.ImageAssetIDs = make([]entity.AssetID, video.ImagesPerChapter)
		c.ImagePrompts = make([]string, 0, video.ImagesPerChapter)
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
