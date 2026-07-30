package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
	"github.com/tbui/yt-studio/domain/scheduler"
)

// ExpandOptions are the tail's scheduler-shaped inputs, sourced from settings
// rows by the caller.
//
// They are read when the graph expands rather than when the video is enqueued,
// so a gate switched on while an operator was reading a blueprint applies to
// the video they are about to approve.
type ExpandOptions struct {
	MaxAttempts int
	UploadGate  bool
}

// ExpandVideoGraph builds a video's per-chapter DAG from the chapters its
// blueprint produced, and splices it onto the head graph.
//
// The count comes from the chapter rows rather than from the blueprint response
// or from the number the video was briefed with. Those rows are what the
// operator read before approving, and they are what every task in the tail will
// address — deriving the shape from anything else would be deriving it from
// something nobody agreed to.
//
// It is idempotent. Task ids are deterministic, the row insert is an upsert and
// the scheduler treats a repeated splice of the same shape as a no-op, so an
// approval retried after a partial failure converges rather than duplicating.
func ExpandVideoGraph(
	ctx context.Context,
	videos repository.VideoReader,
	chapters repository.ChapterReader,
	expander GraphExpander,
	now time.Time,
	opts ExpandOptions,
	videoID entity.VideoID,
) error {
	v, err := videos.VideoByID(ctx, videoID)
	if err != nil {
		return err
	}
	rows, err := chapters.ListChaptersByVideo(ctx, videoID)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("%w: %s has no chapters to expand into", ErrConflict, v.Ref)
	}

	tail, err := scheduler.BuildTail(scheduler.BuildSpec{
		VideoID:          v.ID,
		ChapterCount:     len(rows),
		ImagesPerChapter: v.ImagesPerChapter,
		MaxAttempts:      opts.MaxAttempts,
		UploadGate:       opts.UploadGate,
		Now:              now,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrValidation, err)
	}
	if err := expander.Expand(ctx, v.ID, tail); err != nil {
		return fmt.Errorf("expand %s: %w", v.Ref, err)
	}
	return nil
}
