package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tbui/yt-studio/internal/domain/entity"
	"github.com/tbui/yt-studio/internal/domain/repository"
)

// UpdateVideoInput is the brief as the operator holds it: what the video is
// about, and how big it is. The channel and the ref are absent because they are
// the video's identity rather than its description.
type UpdateVideoInput struct {
	Title                 string
	Topic                 string
	ChapterCount          int
	SlidesPerChapter      int
	ThumbnailCells        int
	TargetDurationMinutes int
}

// UpdateVideo corrects the brief a video was created from.
//
// The whole brief, not the fields somebody typed — the same shape
// UpdateVideoMetadata takes, and for the same reason: the dialog holds all six
// and sends all six, which is what keeps this function free of merge logic.
//
// It re-runs nothing and flags nothing stale. These fields are read when a task
// runs, so an edit reaches whatever has not happened yet and leaves what has
// alone. Nothing here asks what state the video is in: this is information the
// operator is entitled to correct, and a chapter count on a graph that has
// already been laid out is a record of the plan rather than an instruction.
func UpdateVideo(
	ctx context.Context,
	videos repository.VideoReader,
	writer repository.VideoWriter,
	now time.Time,
	key string,
	in UpdateVideoInput,
) (entity.Video, error) {
	v, err := GetVideo(ctx, videos, key)
	if err != nil {
		return entity.Video{}, err
	}
	v.Title = strings.TrimSpace(in.Title)
	v.Topic = strings.TrimSpace(in.Topic)
	v.ChapterCount = in.ChapterCount
	v.SlidesPerChapter = in.SlidesPerChapter
	v.ThumbnailCells = in.ThumbnailCells
	v.TargetDurationMinutes = in.TargetDurationMinutes
	v.UpdatedAt = now

	if err := v.Validate(); err != nil {
		return entity.Video{}, fmt.Errorf("%w: %w", ErrValidation, err)
	}
	if err := writer.UpdateVideo(ctx, v); err != nil {
		return entity.Video{}, err
	}
	return v, nil
}
