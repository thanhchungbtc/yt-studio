package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// NarrationOptions are the settings-sourced inputs of one chapter's narration:
// how it should sound. They are a use-case input rather than a backend's own
// configuration for the reason given on provider.SpeakRequest — a voice belongs
// to whoever the video is for, not to the server that speaks it.
type NarrationOptions struct {
	Voice    string
	Language string
	Speed    float64
}

// SynthesizeNarration narrates exactly one chapter.
//
//nolint:revive // the parameter list is the dependency list
func SynthesizeNarration(
	ctx context.Context,
	t entity.Task,
	videos repository.VideoReader,
	chapters repository.ChapterReader,
	tts provider.TTS,
	fields repository.ChapterFieldWriter,
	assets repository.AssetWriter,
	store provider.AssetStore,
	notifier ChapterNotifier,
	opts NarrationOptions,
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
		VideoID:      video.ID,
		ChapterID:    chapter.ID,
		Ordinal:      chapter.Ordinal,
		Text:         chapter.Script,
		ChapterTitle: chapter.Title,
		Voice:        opts.Voice,
		Language:     opts.Language,
		Speed:        opts.Speed,
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
