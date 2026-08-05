package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// GenerateSlide produces exactly one image for one chapter.
//
// Two of these run per chapter and they write to the same row, so the slide is
// recorded through a single indexed statement rather than a read-modify-write.
//
//nolint:revive // the parameter list is the dependency list
func GenerateSlide(
	ctx context.Context,
	t entity.Task,
	videos repository.VideoReader,
	chapters repository.ChapterReader,
	slides provider.SlideProvider,
	fields repository.ChapterFieldWriter,
	assets repository.AssetWriter,
	store provider.AssetStore,
	notifier ChapterNotifier,
	now time.Time,
) entity.TaskOutcome {
	if t.ChapterID == nil {
		return entity.Failed{Err: fmt.Errorf("%w: slide task has no chapter", ErrValidation), Retryable: false}
	}
	if t.Index < 0 {
		return entity.Failed{Err: fmt.Errorf("%w: slide task has no index", ErrValidation), Retryable: false}
	}
	chapter, err := chapters.ChapterByID(ctx, *t.ChapterID)
	if err != nil {
		return classify(err)
	}
	if t.Index >= len(chapter.SlidePrompts) {
		return entity.Failed{
			Err: fmt.Errorf("%w: chapter %d has %d prompts, task wants index %d",
				ErrValidation, chapter.Ordinal, len(chapter.SlidePrompts), t.Index),
			Retryable: true,
		}
	}
	video, err := videos.VideoByID(ctx, t.VideoID)
	if err != nil {
		return classify(err)
	}
	assetID, err := slides.Generate(ctx, provider.SlideRequest{
		VideoID:   video.ID,
		ChapterID: chapter.ID,
		Ordinal:   chapter.Ordinal,
		Index:     t.Index,
		Prompt:    chapter.SlidePrompts[t.Index],
	})
	if err != nil {
		return classify(fmt.Errorf("generate slide %d of chapter %d: %w", t.Index, chapter.Ordinal, err))
	}

	if _, err := RecordAsset(ctx, assets, store, assetID, entity.AssetKindImage,
		video.ID, &chapter.ID, "slide.generate", now); err != nil {
		return classify(err)
	}
	if err := fields.SetChapterSlide(ctx, chapter.ID, t.Index, assetID); err != nil {
		return classify(err)
	}

	if notifier != nil {
		// Re-read so the delta carries every slide recorded so far, including the
		// sibling task's, rather than a stale local copy.
		if fresh, err := chapters.ChapterByID(ctx, chapter.ID); err == nil {
			notifier.NotifyChapter(chapterDelta(fresh))
		}
	}
	return entity.Success{Assets: []entity.AssetID{assetID}}
}
