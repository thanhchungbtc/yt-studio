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
	channels repository.ChannelReader,
	chapters repository.ChapterReader,
	llm provider.LLMProvider,
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
	channel, err := channels.ChannelByID(ctx, video.ChannelID)
	if err != nil {
		return classify(err)
	}

	script, err := llm.Script(ctx, provider.ScriptRequest{
		VideoID:          video.ID,
		ChapterID:        chapter.ID,
		Ordinal:          chapter.Ordinal,
		ChapterTitle:     chapter.Title,
		ChapterSummary:   chapter.Summary,
		BlueprintTitle:   video.Title,
		BlueprintSummary: video.Topic,
		Style:            channel.Style,
	})
	if err != nil {
		return classify(fmt.Errorf("generate script for chapter %d: %w", chapter.Ordinal, err))
	}

	if _, err := RecordAsset(ctx, assets, store, script.AssetID, entity.AssetKindScript,
		video.ID, &chapter.ID, "llm.script", now); err != nil {
		return classify(err)
	}

	duration := float64(script.WordCount) / narrationWordsPerMinute * 60
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

// narrationWordsPerMinute is the delivery rate used to turn a word count into
// the chapter duration the UI shows.
const narrationWordsPerMinute = 150.0

// EstimateNarrationSeconds reports how long a script takes to narrate. It is
// exported so an operator's script edit updates the same figure.
func EstimateNarrationSeconds(script string) float64 {
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
	return float64(words) / narrationWordsPerMinute * 60
}
