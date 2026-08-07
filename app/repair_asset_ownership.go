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

// repairProvenance marks a row this command inferred rather than a task wrote.
const repairProvenance = "repair.ownership"

// RepairAssetOwnership gives every reference to a stored file an asset row
// naming its owner, and reports how many it added. A file reachable only
// through a video's or chapter's id list has no owning row and would otherwise
// read as "nobody references this" the moment a delete had to decide.
//
// It is a precondition for reclaiming any disk, so the server runs it at
// startup and the sweep runs it before looking at a file. Idempotent, and
// normally a no-op: the query behind it is an anti-join.
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
			// Nothing in the store and no row anywhere: recording it would invent
			// a row for a file that does not exist.
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

// describe fills in the columns a reference does not carry. Path, size and mime
// are properties of the content, so an existing row for the same address is
// preferred — one indexed lookup instead of a stat, and the reconstructed row
// matches the original owner's. Only an address with no row asks the store.
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
