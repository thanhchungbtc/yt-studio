package app

import (
	"context"
	"log/slog"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// DeleteVideo removes a video, its chapters, its task graph and the files only
// it was using.
//
// The order is forced by what each step can undo. The scheduler is told first,
// because it holds the cancel function for whatever is running and would
// otherwise keep working against rows that are about to disappear. The rows go
// next, as one transaction, which is also what computes the set of files nobody
// references any more. The files go last: unlinking before the commit risks
// taking a file from a video the commit then failed to delete, while a file left
// behind after it is only waste that `yt-studio sweep` collects.
//
// A file another video also uses is left alone. Identical bytes across two
// videos are ordinary rather than exotic — the sample backends serve one
// narration recording and a handful of stills to every video — so the store
// keeps one copy and each video records a row against it.
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

// reclaim unlinks files whose last owner is gone.
//
// A failure here is logged rather than returned: the delete the operator asked
// for has already committed, and reporting it as failed would invite a retry of
// a video that no longer exists. What is left behind is disk, which the sweep
// reclaims on the next run.
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
