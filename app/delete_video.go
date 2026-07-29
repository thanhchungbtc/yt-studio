package app

import (
	"context"

	"github.com/tbui/yt-studio/domain/repository"
)

// DeleteVideo removes a video, its chapters and its whole DAG. Assets are left
// in the content-addressed store: they are shared by hash and cheap to keep.
func DeleteVideo(
	ctx context.Context,
	videos repository.VideoReader,
	writer repository.VideoWriter,
	tasks repository.TaskWriter,
	forgetter VideoForgetter,
	key string,
) error {
	v, err := GetVideo(ctx, videos, key)
	if err != nil {
		return err
	}
	if err := forgetter.Forget(ctx, v.ID); err != nil {
		return err
	}
	if err := tasks.DeleteGraph(ctx, v.ID); err != nil {
		return err
	}
	return writer.DeleteVideo(ctx, v.ID)
}
