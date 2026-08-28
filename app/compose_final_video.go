package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// ComposeFinalVideo concatenates every chapter clip into the final render. The
// clips are handed to the composer in ordinal order; whether it can copy them
// through or has to re-encode is the backend's decision.
//
// onPercent may be nil. This is the one task long enough that a person watching
// needs more than "running": a re-encode of a ten-minute video is minutes of a
// single task, and a backend that can measure it reports through here.
func ComposeFinalVideo(
	ctx context.Context,
	t entity.Task,
	chapters repository.ChapterReader,
	composer provider.VideoComposer,
	videoFields repository.VideoFieldWriter,
	assets repository.AssetWriter,
	store provider.AssetStore,
	onPercent func(int),
	now time.Time,
) entity.TaskOutcome {
	rows, err := chapters.ListChaptersByVideo(ctx, t.VideoID)
	if err != nil {
		return classify(err)
	}
	if len(rows) == 0 {
		return entity.Failed{Err: fmt.Errorf("%w: video has no chapters", ErrValidation), Retryable: false}
	}

	clips := make([]entity.AssetID, 0, len(rows))
	for _, c := range rows {
		if c.ClipAssetID == nil || *c.ClipAssetID == "" {
			return entity.Failed{
				Err:       fmt.Errorf("%w: chapter %d has no clip", ErrValidation, c.Ordinal),
				Retryable: true,
			}
		}
		clips = append(clips, *c.ClipAssetID)
	}

	assetID, err := composer.Concat(ctx, provider.ConcatRequest{
		VideoID:      t.VideoID,
		ClipAssetIDs: clips,
		OnPercent:    onPercent,
	})
	if err != nil {
		return classify(fmt.Errorf("concatenate %d clips: %w", len(clips), err))
	}

	if _, err := RecordAsset(ctx, assets, store, assetID, entity.AssetKindFinal,
		t.VideoID, nil, "compose.concat", now); err != nil {
		return classify(err)
	}
	if err := videoFields.SetVideoFinalAsset(ctx, t.VideoID, assetID); err != nil {
		return classify(err)
	}
	return entity.Success{Assets: []entity.AssetID{assetID}}
}
