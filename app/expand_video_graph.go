package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
	"github.com/tbui/yt-studio/domain/scheduler"
)

// ExpandOptions are the tail's settings-sourced inputs, read at expansion
// rather than at enqueue, so a gate switched on while an operator was reading a
// blueprint still applies to the video they approve.
type ExpandOptions struct {
	MaxAttempts int
	UploadGate  bool
}

// ExpandVideoGraph builds a video's per-chapter DAG and splices it onto the
// head graph. The count comes from the chapter rows — what the operator read
// before approving, and what every task in the tail addresses — rather than
// from the response or the brief.
//
// Idempotent: task ids are deterministic, the insert upserts and a repeated
// splice of the same shape is a no-op, so a retried approval converges.
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
		SlidesPerChapter: v.SlidesPerChapter,
		// Off the video row, not settings: it fixes how many icon tasks this
		// graph gets, and the graph can never grow after.
		ThumbnailCells: v.ThumbnailCells,
		MaxAttempts:    opts.MaxAttempts,
		UploadGate:     opts.UploadGate,
		Now:            now,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrValidation, err)
	}
	if err := expander.Expand(ctx, v.ID, tail); err != nil {
		return fmt.Errorf("expand %s: %w", v.Ref, err)
	}
	return nil
}
