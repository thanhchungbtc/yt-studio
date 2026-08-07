package app

import (
	"context"
	"errors"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// GetChannel resolves a channel by either key, since the API accepts both at
// the same position. The id is tried first: a UUID is also a valid slug.
func GetChannel(ctx context.Context, channels repository.ChannelReader, key string) (entity.Channel, error) {
	if key == "" {
		return entity.Channel{}, Invalid("channel", "must not be empty")
	}
	c, err := channels.ChannelByID(ctx, entity.ChannelID(key))
	if err == nil {
		return c, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return entity.Channel{}, err
	}
	if !entity.LooksLikeSlug(key) {
		return entity.Channel{}, err
	}
	return channels.ChannelBySlug(ctx, entity.Slug(key))
}
