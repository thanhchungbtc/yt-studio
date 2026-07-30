package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// repairProvenance marks a row this command reconstructed rather than a task
// wrote, so the origin of a row that was inferred stays visible.
const repairProvenance = "repair.ownership"

// RepairAssetOwnership gives every reference to a stored file the asset row that
// says who owns it, and reports how many it had to add.
//
// It exists because the assets table was once keyed by content address alone: a
// video that produced bytes another video had already produced got no row, so its
// ownership survived only as an id on the video or on one of its chapters.
// Nothing was visibly wrong until a delete had to decide which files were still
// needed, at which point the missing rows read as "nobody references this".
//
// Running it is a precondition for reclaiming any disk. Both callers respect
// that: the daemon repairs at startup, before it can serve a delete, and the
// sweep repairs before it looks at a single file.
//
// It is idempotent and normally does nothing at all — the query behind it is an
// anti-join that comes back empty once every edge is recorded — so there is no
// flag to turn it off and no state that says it has run.
func RepairAssetOwnership(
	ctx context.Context,
	maintainer repository.AssetMaintainer,
	assets repository.AssetReader,
	writer repository.AssetWriter,
	store provider.AssetStore,
	now time.Time,
	log *slog.Logger,
) (int, error) {
	missing, err := maintainer.MissingAssetOwners(ctx)
	if err != nil {
		return 0, err
	}
	if len(missing) == 0 {
		return 0, nil
	}

	var repaired, dangling int
	for _, ref := range missing {
		a, err := describe(ctx, assets, store, ref, now)
		switch {
		case errors.Is(err, entity.ErrAssetNotFound):
			// The reference points at a file that is not in the store and has no row
			// anywhere: there is nothing to own and nothing to protect. Recording it
			// would only invent a row for a file that does not exist.
			dangling++
			continue
		case err != nil:
			return repaired, err
		}
		if err := writer.PutAsset(ctx, a); err != nil {
			return repaired, fmt.Errorf("record owner %s of asset %s: %w", ref.VideoID, ref.AssetID.Short(), err)
		}
		repaired++
	}

	log.Info("repaired asset ownership",
		slog.Int("rows_added", repaired),
		slog.Int("dangling_references", dangling))
	return repaired, nil
}

// describe fills in the columns a reference does not carry.
//
// path, size and mime are properties of the content, so any existing row for the
// same address already has them and is preferred: it costs one indexed lookup
// instead of a stat, and it keeps a reconstructed row byte-identical to the
// original owner's. Only an address with no row at all — the producing video was
// deleted before this repair existed — has to ask the store.
func describe(
	ctx context.Context,
	assets repository.AssetReader,
	store provider.AssetStore,
	ref repository.AssetOwnerRef,
	now time.Time,
) (entity.Asset, error) {
	if existing, err := assets.AssetByID(ctx, ref.AssetID); err == nil {
		return entity.NewAsset(ref.AssetID, ref.VideoID, ref.ChapterID, existing.Kind,
			existing.Path, existing.Size, repairProvenance, now)
	} else if !errors.Is(err, repository.ErrNotFound) && !errors.Is(err, entity.ErrAssetNotFound) {
		return entity.Asset{}, fmt.Errorf("read asset %s: %w", ref.AssetID.Short(), err)
	}

	stored, err := store.Stat(ctx, ref.AssetID, ref.Kind)
	if err != nil {
		return entity.Asset{}, err
	}
	return entity.NewAsset(ref.AssetID, ref.VideoID, ref.ChapterID, ref.Kind,
		stored.Path, stored.Size, repairProvenance, now)
}
