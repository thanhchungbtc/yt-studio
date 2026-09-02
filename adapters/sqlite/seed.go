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

// SeedChannels writes the channels a fresh installation starts with. Ids are
// derived from the slug, so a re-seed is byte-identical rather than merely
// equivalent.
//
// A re-seed never regresses a channel an operator has since worked on: the
// upsert is by slug and touches the display fields only, leaving the credential
// status and the video sequence as it found them.
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
			slug:        "sleepy-mind-lab",
			name:        "Sleepy Mind Lab",
			description: "Long-form narrated calm for falling asleep to.",
		},
	}
	out := make([]entity.Channel, 0, len(specs))
	for _, s := range specs {
		// Credentials are left as NewChannel sets them, missing, even where a
		// token already sits on disk under the matching slug. Authorization is
		// established by the flow that writes that token, and a seeded row
		// claiming otherwise would be the one record in the database asserting a
		// grant nothing checked.
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
