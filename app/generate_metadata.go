package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// GenerateMetadata writes the YouTube-facing listing for a finished render.
//
// When the upload gate is enabled this task carries it: on success the
// scheduler parks in awaiting_approval and does not release the upload (§6).
//
//nolint:revive // the parameter list is the dependency list
func GenerateMetadata(
	ctx context.Context,
	t entity.Task,
	videos repository.VideoReader,
	channels repository.ChannelReader,
	chapters repository.ChapterReader,
	llm provider.LLMProvider,
	videoFields repository.VideoFieldWriter,
	assets repository.AssetWriter,
	store provider.AssetStore,
	now time.Time,
) entity.TaskOutcome {
	video, err := videos.VideoByID(ctx, t.VideoID)
	if err != nil {
		return classify(err)
	}
	channel, err := channels.ChannelByID(ctx, video.ChannelID)
	if err != nil {
		return classify(err)
	}
	rows, err := chapters.ListChaptersByVideo(ctx, video.ID)
	if err != nil {
		return classify(err)
	}

	outline := make([]provider.BlueprintChapter, 0, len(rows))
	for _, c := range rows {
		outline = append(outline, provider.BlueprintChapter{
			Ordinal: c.Ordinal,
			Title:   c.Title,
			Summary: c.Summary,
		})
	}

	md, err := llm.Metadata(ctx, provider.MetadataRequest{
		VideoID:  video.ID,
		VideoRef: video.Ref,
		Title:    video.Title,
		Topic:    video.Topic,
		Chapters: outline,
		Style:    channel.Style,
	})
	if err != nil {
		return classify(fmt.Errorf("generate metadata: %w", err))
	}

	if _, err := RecordAsset(ctx, assets, store, md.AssetID, entity.AssetKindMetadata,
		video.ID, nil, "llm.metadata", now); err != nil {
		return classify(err)
	}
	if err := videoFields.SetVideoMetadata(ctx, video.ID, md.Metadata); err != nil {
		return classify(err)
	}
	return entity.Success{Assets: []entity.AssetID{md.AssetID}}
}
