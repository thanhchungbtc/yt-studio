package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// DeleteChannel removes a channel and everything it owns. Its videos go one at
// a time through the single-delete path rather than by foreign key, so each is
// dropped from the scheduler and its files reclaimed — a cascade would take the
// rows and leave the disk.
func DeleteChannel(
	ctx context.Context,
	channels repository.ChannelReader,
	writer repository.ChannelWriter,
	videos repository.VideoReader,
	videoWriter repository.VideoWriter,
	forgetter VideoForgetter,
	store provider.AssetStore,
	log *slog.Logger,
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
		unreferenced, err := videoWriter.DeleteVideo(ctx, v.ID)
		if err != nil {
			return fmt.Errorf("delete %s: %w", v.Ref, err)
		}
		reclaim(ctx, store, log, unreferenced, v.Ref)
	}
	return writer.DeleteChannel(ctx, c.ID)
}

// entityMaxVideosPerChannel bounds the cascade listing, well above any
// realistic single-operator channel.
const entityMaxVideosPerChannel = 500
