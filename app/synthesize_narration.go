package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// SynthesizeNarration narrates exactly one chapter.
//
//nolint:revive // the parameter list is the dependency list
func SynthesizeNarration(
	ctx context.Context,
	t entity.Task,
	videos repository.VideoReader,
	chapters repository.ChapterReader,
	tts provider.TTSProvider,
	fields repository.ChapterFieldWriter,
	assets repository.AssetWriter,
	store provider.AssetStore,
	notifier ChapterNotifier,
	now time.Time,
) entity.TaskOutcome {
	if t.ChapterID == nil {
		return entity.Failed{Err: fmt.Errorf("%w: tts task has no chapter", ErrValidation), Retryable: false}
	}
	chapter, err := chapters.ChapterByID(ctx, *t.ChapterID)
	if err != nil {
		return classify(err)
	}
	if chapter.Script == "" {
		return entity.Failed{
			Err:       fmt.Errorf("%w: chapter %d has no script", ErrValidation, chapter.Ordinal),
			Retryable: false,
		}
	}
	video, err := videos.VideoByID(ctx, t.VideoID)
	if err != nil {
		return classify(err)
	}
	assetID, err := tts.Speak(ctx, provider.SpeakRequest{
		VideoID:   video.ID,
		ChapterID: chapter.ID,
		Ordinal:   chapter.Ordinal,
		Text:      chapter.Script,
	})
	if err != nil {
		return classify(fmt.Errorf("narrate chapter %d: %w", chapter.Ordinal, err))
	}

	if _, err := RecordAsset(ctx, assets, store, assetID, entity.AssetKindAudio,
		video.ID, &chapter.ID, "tts.speak", now); err != nil {
		return classify(err)
	}
	if err := fields.SetChapterAudio(ctx, chapter.ID, assetID); err != nil {
		return classify(err)
	}

	chapter.AudioAssetID = &assetID
	chapter.UpdatedAt = now
	if notifier != nil {
		notifier.NotifyChapter(chapterDelta(chapter))
	}
	return entity.Success{Assets: []entity.AssetID{assetID}}
}
