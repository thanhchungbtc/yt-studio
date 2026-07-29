package app

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// VideoSummary is a video plus the task census the list view needs to show
// progress and the bottleneck at a glance (§9).
type VideoSummary struct {
	Video  entity.Video
	Counts repository.TaskCounts
}

// ListVideos returns a page of videos with their progress.
func ListVideos(
	ctx context.Context,
	videos repository.VideoReader,
	tasks repository.TaskReader,
	f repository.VideoFilter,
) ([]VideoSummary, int, error) {
	rows, err := videos.ListVideos(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	total, err := videos.CountVideos(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	out := make([]VideoSummary, 0, len(rows))
	for _, v := range rows {
		counts, err := tasks.CountTasksByVideo(ctx, v.ID)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, VideoSummary{Video: v, Counts: counts})
	}
	return out, total, nil
}
