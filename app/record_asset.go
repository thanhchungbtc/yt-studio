package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// RecordAsset writes the metadata row for a file a provider just stored.
//
// Providers return only a content address; the size and path come from the
// store. The write is an upsert by that address, so a task re-run that produced
// identical bytes changes nothing.
func RecordAsset(
	ctx context.Context,
	assets repository.AssetWriter,
	store provider.AssetStore,
	id entity.AssetID,
	kind entity.AssetKind,
	videoID entity.VideoID,
	chapterID *entity.ChapterID,
	provenance string,
	now time.Time,
) (entity.Asset, error) {
	stored, err := store.Stat(ctx, id, kind)
	if err != nil {
		return entity.Asset{}, fmt.Errorf("stat %s asset: %w", kind, err)
	}
	a, err := entity.NewAsset(id, videoID, chapterID, kind, stored.Path, stored.Size, provenance, now)
	if err != nil {
		return entity.Asset{}, err
	}
	if err := assets.PutAsset(ctx, a); err != nil {
		return entity.Asset{}, fmt.Errorf("record %s asset: %w", kind, err)
	}
	return a, nil
}
