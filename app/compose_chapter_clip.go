package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// ComposeChapterClip joins one chapter's narration and stills into a clip.
//
// It is the join point of the DAG's two independent branches: the script/TTS
// branch and the prompt/image branch (§4).
//
//nolint:revive // the parameter list is the dependency list
func ComposeChapterClip(
	ctx context.Context,
	t entity.Task,
	videos repository.VideoReader,
	chapters repository.ChapterReader,
	composer provider.VideoComposer,
	fields repository.ChapterFieldWriter,
	assets repository.AssetWriter,
	store provider.AssetStore,
	notifier ChapterNotifier,
	now time.Time,
) entity.TaskOutcome {
	if t.ChapterID == nil {
		return entity.Failed{Err: fmt.Errorf("%w: clip task has no chapter", ErrValidation), Retryable: false}
	}
	chapter, err := chapters.ChapterByID(ctx, *t.ChapterID)
	if err != nil {
		return classify(err)
	}
	if chapter.AudioAssetID == nil || *chapter.AudioAssetID == "" {
		return entity.Failed{
			Err:       fmt.Errorf("%w: chapter %d has no narration", ErrValidation, chapter.Ordinal),
			Retryable: true,
		}
	}
	stills := make([]entity.AssetID, 0, len(chapter.ImageAssetIDs))
	for _, id := range chapter.ImageAssetIDs {
		if id != "" {
			stills = append(stills, id)
		}
	}
	if len(stills) == 0 {
		return entity.Failed{
			Err:       fmt.Errorf("%w: chapter %d has no stills", ErrValidation, chapter.Ordinal),
			Retryable: true,
		}
	}

	// A composer may burn both titles into the frame, so they travel with the
	// request rather than being fetched behind the port.
	video, err := videos.VideoByID(ctx, chapter.VideoID)
	if err != nil {
		return classify(err)
	}

	assetID, err := composer.Clip(ctx, provider.ClipRequest{
		VideoID:       chapter.VideoID,
		ChapterID:     chapter.ID,
		Ordinal:       chapter.Ordinal,
		ChapterTitle:  chapter.Title,
		VideoTitle:    video.Title,
		AudioAssetID:  *chapter.AudioAssetID,
		ImageAssetIDs: stills,
	})
	if err != nil {
		return classify(fmt.Errorf("compose clip for chapter %d: %w", chapter.Ordinal, err))
	}

	if _, err := RecordAsset(ctx, assets, store, assetID, entity.AssetKindClip,
		chapter.VideoID, &chapter.ID, "compose.clip", now); err != nil {
		return classify(err)
	}
	if err := fields.SetChapterClip(ctx, chapter.ID, assetID); err != nil {
		return classify(err)
	}

	chapter.ClipAssetID = &assetID
	chapter.UpdatedAt = now
	if notifier != nil {
		notifier.NotifyChapter(chapterDelta(chapter))
	}
	return entity.Success{Assets: []entity.AssetID{assetID}}
}
