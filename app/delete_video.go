package app

import (
	"context"
	"log/slog"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// DeleteVideo removes a video, its chapters, its task graph and the files only
// it was using. The order is forced by what each step can undo: the scheduler
// first, since it would otherwise keep working against rows about to vanish;
// the rows next, in one transaction that also computes what nobody references;
// the files last, because a stray file is waste the sweep collects while an
// early unlink can take a file from a video the commit failed to delete.
//
// A file another video also uses is left alone — identical bytes across videos
// are ordinary, so the store keeps one copy with a row per owner.
func DeleteVideo(
	ctx context.Context,
	videos repository.VideoReader,
	writer repository.VideoWriter,
	forgetter VideoForgetter,
	store provider.AssetStore,
	log *slog.Logger,
	key string,
) error {
	v, err := GetVideo(ctx, videos, key)
	if err != nil {
		return err
	}
	if err := forgetter.Forget(ctx, v.ID); err != nil {
		return err
	}
	unreferenced, err := writer.DeleteVideo(ctx, v.ID)
	if err != nil {
		return err
	}
	reclaim(ctx, store, log, unreferenced, v.Ref)
	return nil
}

// reclaim unlinks files whose last owner is gone. A failure is logged rather
// than returned: the delete has already committed, so reporting failure would
// invite a retry of a video that no longer exists, and the sweep collects what
// is left.
func reclaim(
	ctx context.Context,
	store provider.AssetStore,
	log *slog.Logger,
	assets []entity.Asset,
	ref entity.Ref,
) {
	if len(assets) == 0 {
		return
	}
	var freed int64
	var failed int
	for _, a := range assets {
		if err := store.Delete(ctx, a.ID, a.Kind); err != nil {
			failed++
			log.Warn("failed to delete an unreferenced asset",
				slog.String("video", string(ref)),
				slog.String("asset", a.ID.Short()),
				slog.String("kind", string(a.Kind)),
				slog.String("error", err.Error()))
			continue
		}
		freed += a.Size
	}
	log.Info("reclaimed assets",
		slog.String("video", string(ref)),
		slog.Int("files", len(assets)-failed),
		slog.Int64("bytes", freed))
}
