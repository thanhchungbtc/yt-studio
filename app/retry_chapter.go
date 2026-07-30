package app

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
)

// RetryChapter resets one chapter and everything downstream of it, so a bad
// script or a failed still can be re-run without redoing the whole video.
//
// The video's coalesced image-prompt batch is dropped first: a retry that
// replayed the cached prompts would reproduce exactly the output the operator
// rejected.
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
