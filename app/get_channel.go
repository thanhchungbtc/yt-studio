package app

import (
	"context"
	"errors"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// GetChannel resolves a channel by either key.
//
// The API accepts a natural key or an id at the same position, so CLI usage and
// hand-written requests never need a lookup step first (§3). The id is tried
// first because a UUID is also shaped like a valid slug.
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
