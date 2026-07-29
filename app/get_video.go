package app

import (
	"context"
	"errors"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// GetVideo resolves a video by either key: the opaque id or the human-readable
// ref such as DSS-14 (§3).
func GetVideo(ctx context.Context, videos repository.VideoReader, key string) (entity.Video, error) {
	if key == "" {
		return entity.Video{}, Invalid("video", "must not be empty")
	}
	if entity.LooksLikeRef(key) {
		return videos.VideoByRef(ctx, entity.Ref(key))
	}
	v, err := videos.VideoByID(ctx, entity.VideoID(key))
	if err != nil && errors.Is(err, repository.ErrNotFound) {
		return videos.VideoByRef(ctx, entity.Ref(key))
	}
	return v, err
}
