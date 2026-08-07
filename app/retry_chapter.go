package app

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
)

// RetryChapter resets one chapter and everything downstream, so a bad script or
// failed slide costs one chapter rather than the video. The coalesced
// slide-prompt batch is dropped first, or a retry would replay the prompts that
// produced the rejected output.
func RetryChapter(
	ctx context.Context,
	retrier ChapterRetrier,
	prompts PromptCacheInvalidator,
	videoID entity.VideoID,
	ordinal int,
) error {
	if ordinal < 1 {
		return Invalid("ordinal", "must be at least 1")
	}
	if prompts != nil {
		prompts.Forget(videoID)
	}
	return retrier.RetryChapter(ctx, videoID, ordinal)
}
