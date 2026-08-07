package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// SeedSettings writes the default settings table, upserting by key: a second
// run updates metadata in place and leaves operator-set values alone.
func SeedSettings(ctx context.Context, settings repository.SettingWriter) error {
	if err := settings.UpsertSettings(ctx, entity.DefaultSettings()); err != nil {
		return fmt.Errorf("seed settings: %w", err)
	}
	return nil
}

// SeedChannels writes the demonstration channels. Ids are derived from the
// slug, so a re-seed is byte-identical rather than merely equivalent.
func SeedChannels(ctx context.Context, channels repository.ChannelWriter, now time.Time) error {
	for _, c := range defaultChannels(now) {
		if _, err := channels.UpsertChannelBySlug(ctx, c); err != nil {
			return fmt.Errorf("seed channel %q: %w", c.Slug, err)
		}
	}
	return nil
}

// SeedChannelID derives the deterministic opaque id of a seeded channel.
func SeedChannelID(slug entity.Slug) entity.ChannelID {
	return entity.ChannelID("ch_" + slug)
}

func defaultChannels(now time.Time) []entity.Channel {
	specs := []struct {
		slug        entity.Slug
		name        string
		description string
	}{
		{
			slug:        "deep-sleep-stories",
			name:        "Deep Sleep Stories",
			description: "Three-hour narrated stories for falling asleep to.",
		},
		{
			slug:        "history-explained",
			name:        "History Explained",
			description: "Long-form narrated histories, one chapter per turning point.",
		},
	}
	out := make([]entity.Channel, 0, len(specs))
	for _, s := range specs {
		c, err := entity.NewChannel(SeedChannelID(s.slug), s.slug, s.name, entity.StyleConfig{}, now)
		if err != nil {
			// Unreachable: the specs above are constants and are covered by a test.
			panic(fmt.Sprintf("invalid seed channel %q: %v", s.slug, err))
		}
		c.Description = s.description
		out = append(out, c)
	}
	return out
}
