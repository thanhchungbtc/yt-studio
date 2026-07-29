package app

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// CancelVideo stops a video. In-flight provider calls are cancelled through the
// per-video context, so its pool slots come back within 100 ms (§8.3).
func CancelVideo(
	ctx context.Context,
	videos repository.VideoReader,
	states repository.VideoStateWriter,
	canceller VideoCanceller,
	key string,
) (entity.Video, error) {
	v, err := GetVideo(ctx, videos, key)
	if err != nil {
		return entity.Video{}, err
	}
	if err := canceller.Cancel(ctx, v.ID); err != nil {
		// A video the scheduler never admitted is still cancellable: the task
		// table is the state, and there is simply nothing in flight.
		if err := states.SetVideoState(ctx, v.ID, entity.VideoStateCancelled, "cancelled by operator"); err != nil {
			return entity.Video{}, err
		}
	}
	v.State = entity.VideoStateCancelled
	return v, nil
}
