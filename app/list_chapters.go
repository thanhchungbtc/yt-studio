package app

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// ListChapters returns a video's chapters in ordinal order.
func ListChapters(
	ctx context.Context,
	chapters repository.ChapterReader,
	videoID entity.VideoID,
) ([]entity.Chapter, error) {
	return chapters.ListChaptersByVideo(ctx, videoID)
}
