package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// ComposeFinalVideo concatenates every chapter clip into the final render.
//
// The clips arrive in ordinal order and are copied through rather than
// re-encoded: re-encoding three hours of already-correct video is the single
// largest avoidable cost in the pipeline (§11).
func ComposeFinalVideo(
	ctx context.Context,
	t entity.Task,
	chapters repository.ChapterReader,
	composer provider.VideoComposer,
	videoFields repository.VideoFieldWriter,
	assets repository.AssetWriter,
	store provider.AssetStore,
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

	assetID, err := composer.Concat(ctx, provider.ConcatRequest{VideoID: t.VideoID, ClipAssetIDs: clips})
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
