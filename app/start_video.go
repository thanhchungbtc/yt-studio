package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
	"github.com/tbui/yt-studio/domain/scheduler"
)

// StartVideoOptions are the scheduler-shaped inputs, all sourced from settings
// rows by the caller (§3).
type StartVideoOptions struct {
	MaxAttempts   int
	BlueprintGate bool
	UploadGate    bool
}

// StartVideo builds a video's DAG and hands it to the scheduler.
//
// It is idempotent: task ids are deterministic and the graph insert is an
// upsert, so calling it twice for the same video schedules nothing new (§3).
func StartVideo(
	ctx context.Context,
	videos repository.VideoReader,
	submitter GraphSubmitter,
	now time.Time,
	opts StartVideoOptions,
	key string,
) (entity.Video, error) {
	v, err := GetVideo(ctx, videos, key)
	if err != nil {
		return entity.Video{}, err
	}
	if v.State == entity.VideoStateCompleted {
		return entity.Video{}, fmt.Errorf("%w: %s has already completed", ErrConflict, v.Ref)
	}

	g, err := scheduler.BuildGraph(scheduler.BuildSpec{
		VideoID:          v.ID,
		ChapterCount:     v.ChapterCount,
		ImagesPerChapter: v.ImagesPerChapter,
		MaxAttempts:      opts.MaxAttempts,
		BlueprintGate:    opts.BlueprintGate,
		UploadGate:       opts.UploadGate,
		Now:              now,
	})
	if err != nil {
		return entity.Video{}, fmt.Errorf("%w: %w", ErrValidation, err)
	}
	if err := submitter.Submit(ctx, g); err != nil {
		return entity.Video{}, fmt.Errorf("submit %s: %w", v.Ref, err)
	}
	return v, nil
}
