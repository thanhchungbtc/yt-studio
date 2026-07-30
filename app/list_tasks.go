package app

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// ListTasksByVideo returns a video's whole DAG in dispatch order.
func ListTasksByVideo(
	ctx context.Context,
	tasks repository.TaskReader,
	videoID entity.VideoID,
) ([]entity.Task, error) {
	return tasks.ListTasksByVideo(ctx, videoID)
}

// ListRecentTasks powers the operator console's live table.
func ListRecentTasks(ctx context.Context, tasks repository.TaskReader, limit int) ([]entity.Task, error) {
	return tasks.ListRecentTasks(ctx, limit)
}
