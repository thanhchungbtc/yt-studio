package repository

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
)

// ChapterReader reads chapters.
type ChapterReader interface {
	ChapterByID(ctx context.Context, id entity.ChapterID) (entity.Chapter, error)
	ListChaptersByVideo(ctx context.Context, videoID entity.VideoID) ([]entity.Chapter, error)
}

// ChapterWriter creates and updates chapters. Blueprint approval replaces a
// video's whole chapter set in one call, so ReplaceChapters is atomic.
type ChapterWriter interface {
	ReplaceChapters(ctx context.Context, videoID entity.VideoID, chapters []entity.Chapter) error
	UpdateChapter(ctx context.Context, c entity.Chapter) error
}

// ChapterFieldWriter narrows writes to a single field. Two slide tasks for one
// chapter run concurrently by design, so a read-modify-write of the whole row
// would lose one; each method here is one atomic statement.
type ChapterFieldWriter interface {
	SetChapterScript(ctx context.Context, id entity.ChapterID, script string, durationSeconds float64) error
	SetChapterPrompts(ctx context.Context, id entity.ChapterID, prompts []string) error
	SetChapterPrompt(ctx context.Context, id entity.ChapterID, index int, prompt string) error
	SetChapterAudio(ctx context.Context, id entity.ChapterID, assetID entity.AssetID) error
	SetChapterSlide(ctx context.Context, id entity.ChapterID, index int, assetID entity.AssetID) error
	SetChapterClip(ctx context.Context, id entity.ChapterID, assetID entity.AssetID) error
}
