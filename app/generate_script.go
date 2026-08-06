package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// GenerateScript writes exactly one chapter's narration.
//
//nolint:revive // the parameter list is the dependency list
func GenerateScript(
	ctx context.Context,
	t entity.Task,
	videos repository.VideoReader,
	chapters repository.ChapterReader,
	llm provider.LLM,
	fields repository.ChapterFieldWriter,
	assets repository.AssetWriter,
	store provider.AssetStore,
	notifier ChapterNotifier,
	now time.Time,
) entity.TaskOutcome {
	if t.ChapterID == nil {
		return entity.Failed{Err: fmt.Errorf("%w: script task has no chapter", ErrValidation), Retryable: false}
	}
	chapter, err := chapters.ChapterByID(ctx, *t.ChapterID)
	if err != nil {
		return classify(err)
	}
	video, err := videos.VideoByID(ctx, t.VideoID)
	if err != nil {
		return classify(err)
	}
	// The whole outline goes with every chapter. A writer that can see what
	// chapter 12 already covered is what keeps chapter 31 from covering it
	// again, and that is the failure a fifty-chapter video actually has.
	outline, err := blueprintOutline(ctx, chapters, video)
	if err != nil {
		return classify(err)
	}

	script, err := llm.Script(ctx, provider.ScriptRequest{
		VideoID:     video.ID,
		ChapterID:   chapter.ID,
		Ordinal:     chapter.Ordinal,
		Blueprint:   outline,
		TargetWords: chapter.EstimatedWords,
	})
	if err != nil {
		return classify(fmt.Errorf("generate script for chapter %d: %w", chapter.Ordinal, err))
	}

	if _, err := RecordAsset(ctx, assets, store, script.AssetID, entity.AssetKindScript,
		video.ID, &chapter.ID, "llm.script", now); err != nil {
		return classify(err)
	}

	duration := NarrationSeconds(script.WordCount)
	if err := fields.SetChapterScript(ctx, chapter.ID, script.Text, duration); err != nil {
		return classify(err)
	}

	chapter.Script = script.Text
	chapter.DurationSeconds = duration
	chapter.UpdatedAt = now
	if notifier != nil {
		notifier.NotifyChapter(chapterDelta(chapter))
	}
	return entity.Success{Assets: []entity.AssetID{script.AssetID}}
}

// blueprintOutline projects a video's chapters as the plan a script is written
// inside.
func blueprintOutline(
	ctx context.Context,
	chapters repository.ChapterReader,
	video entity.Video,
) (provider.BlueprintOutline, error) {
	rows, err := chapters.ListChaptersByVideo(ctx, video.ID)
	if err != nil {
		return provider.BlueprintOutline{}, err
	}
	out := provider.BlueprintOutline{
		Title:    video.Title,
		Summary:  video.Topic,
		Chapters: make([]provider.BlueprintChapter, 0, len(rows)),
	}
	for _, c := range rows {
		out.Chapters = append(out.Chapters, provider.BlueprintChapter{
			Ordinal:        c.Ordinal,
			Title:          c.Title,
			Summary:        c.Summary,
			EstimatedWords: c.EstimatedWords,
		})
	}
	return out, nil
}

// NarrationSeconds turns a word count into a duration at the narration speed
// the blueprint budgeted words with. Planning a three-hour video and then
// reporting it as two and a half is the drift one shared constant prevents.
func NarrationSeconds(words int) float64 {
	return float64(words) / float64(entity.DefaultWordsPerMinute) * 60
}

// CountWords counts whitespace-separated words without allocating, which is
// what an operator's script edit needs to re-time a chapter.
func CountWords(script string) int {
	words := 0
	inWord := false
	for i := range len(script) {
		switch script[i] {
		case ' ', '\n', '\t', '\r':
			inWord = false
		default:
			if !inWord {
				words++
				inWord = true
			}
		}
	}
	return words
}
