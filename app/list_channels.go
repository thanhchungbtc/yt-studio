package app

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// ListChannels returns every channel, ordered by display name.
func ListChannels(ctx context.Context, channels repository.ChannelReader) ([]entity.Channel, error) {
	return channels.ListChannels(ctx)
}
