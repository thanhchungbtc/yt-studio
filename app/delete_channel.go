package app

import (
	"context"
	"fmt"

	"github.com/tbui/yt-studio/domain/repository"
)

// DeleteChannel removes a channel and, by cascade, its videos and chapters. Its
// videos are dropped from the scheduler first so nothing keeps running against
// rows that no longer exist.
func DeleteChannel(
	ctx context.Context,
	channels repository.ChannelReader,
	writer repository.ChannelWriter,
	videos repository.VideoReader,
	tasks repository.TaskWriter,
	forgetter VideoForgetter,
	key string,
) error {
	c, err := GetChannel(ctx, channels, key)
	if err != nil {
		return err
	}
	owned, err := videos.ListVideos(ctx, repository.VideoFilter{ChannelID: c.ID, Limit: entityMaxVideosPerChannel})
	if err != nil {
		return err
	}
	for _, v := range owned {
		if err := forgetter.Forget(ctx, v.ID); err != nil {
			return fmt.Errorf("forget %s: %w", v.Ref, err)
		}
		if err := tasks.DeleteGraph(ctx, v.ID); err != nil {
			return fmt.Errorf("delete graph of %s: %w", v.Ref, err)
		}
	}
	return writer.DeleteChannel(ctx, c.ID)
}

// entityMaxVideosPerChannel bounds the cascade listing. It is deliberately
// larger than any realistic single-operator channel.
const entityMaxVideosPerChannel = 500
