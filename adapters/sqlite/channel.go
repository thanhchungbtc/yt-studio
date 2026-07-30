package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tbui/yt-studio/adapters/sqlite/sqlcgen"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// Compile-time proof that the store satisfies both halves of the port.
var (
	_ repository.ChannelReader = (*Store)(nil)
	_ repository.ChannelWriter = (*Store)(nil)
)

// ChannelByID reads one channel by its opaque id.
func (s *Store) ChannelByID(ctx context.Context, id entity.ChannelID) (entity.Channel, error) {
	row, err := s.rq.GetChannelByID(ctx, string(id))
	if err != nil {
		return entity.Channel{}, wrapNotFound(err, "channel", string(id))
	}
	return channelFromRow(row), nil
}

// ChannelBySlug reads one channel by its natural key.
func (s *Store) ChannelBySlug(ctx context.Context, slug entity.Slug) (entity.Channel, error) {
	row, err := s.rq.GetChannelBySlug(ctx, string(slug))
	if err != nil {
		return entity.Channel{}, wrapNotFound(err, "channel", string(slug))
	}
	return channelFromRow(row), nil
}

// ListChannels reads every channel, ordered by display name.
func (s *Store) ListChannels(ctx context.Context) ([]entity.Channel, error) {
	rows, err := s.rq.ListChannels(ctx)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	out := make([]entity.Channel, 0, len(rows))
	for _, r := range rows {
		out = append(out, channelFromRow(r))
	}
	return out, nil
}

// CreateChannel inserts a new channel, failing on a taken slug.
func (s *Store) CreateChannel(ctx context.Context, c entity.Channel) error {
	err := s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.CreateChannel(ctx, sqlcgen.CreateChannelParams{
			ID:              string(c.ID),
			Slug:            string(c.Slug),
			Name:            c.Name,
			Description:     c.Description,
			Tone:            c.Style.Tone,
			Voice:           c.Style.Voice,
			ImageStyle:      c.Style.ImageStyle,
			Language:        c.Style.Language,
			WordsPerChapter: int64(c.Style.WordsPerChapter),
			WordsPerMinute:  int64(c.Style.WordsPerMinute),
			Credentials:     string(c.Credentials),
			VideoSeq:        int64(c.VideoSeq),
			CreatedAt:       toUnix(c.CreatedAt),
			UpdatedAt:       toUnix(c.UpdatedAt),
		})
	})
	if err != nil {
		return wrapConflict(err, "channel", string(c.Slug))
	}
	return nil
}

// UpdateChannel writes back mutable channel fields. The slug is immutable and
// is deliberately absent from the statement.
func (s *Store) UpdateChannel(ctx context.Context, c entity.Channel) error {
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.UpdateChannel(ctx, sqlcgen.UpdateChannelParams{
			Name:            c.Name,
			Description:     c.Description,
			Tone:            c.Style.Tone,
			Voice:           c.Style.Voice,
			ImageStyle:      c.Style.ImageStyle,
			Language:        c.Style.Language,
			WordsPerChapter: int64(c.Style.WordsPerChapter),
			WordsPerMinute:  int64(c.Style.WordsPerMinute),
			Credentials:     string(c.Credentials),
			UpdatedAt:       toUnix(c.UpdatedAt),
			ID:              string(c.ID),
		})
	})
}

// DeleteChannel removes a channel and, by cascade, its videos and chapters.
func (s *Store) DeleteChannel(ctx context.Context, id entity.ChannelID) error {
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.DeleteChannel(ctx, string(id))
	})
}

// NextVideoSeq atomically mints the next per-channel video number.
func (s *Store) NextVideoSeq(ctx context.Context, id entity.ChannelID) (int, error) {
	var seq int64
	err := s.doTx(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		if err := q.IncrementVideoSeq(ctx, sqlcgen.IncrementVideoSeqParams{
			UpdatedAt: toUnix(time.Now()),
			ID:        string(id),
		}); err != nil {
			return err
		}
		n, err := q.GetVideoSeq(ctx, string(id))
		seq = n
		return err
	})
	if err != nil {
		return 0, wrapNotFound(err, "channel", string(id))
	}
	return int(seq), nil
}

// UpsertChannelBySlug is the seed path: idempotent by natural key, so a fresh
// database and a ten-times-seeded database end up in the same state.
func (s *Store) UpsertChannelBySlug(ctx context.Context, c entity.Channel) (entity.Channel, error) {
	err := s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.UpsertChannelBySlug(ctx, sqlcgen.UpsertChannelBySlugParams{
			ID:              string(c.ID),
			Slug:            string(c.Slug),
			Name:            c.Name,
			Description:     c.Description,
			Tone:            c.Style.Tone,
			Voice:           c.Style.Voice,
			ImageStyle:      c.Style.ImageStyle,
			Language:        c.Style.Language,
			WordsPerChapter: int64(c.Style.WordsPerChapter),
			WordsPerMinute:  int64(c.Style.WordsPerMinute),
			Credentials:     string(c.Credentials),
			CreatedAt:       toUnix(c.CreatedAt),
			UpdatedAt:       toUnix(c.UpdatedAt),
		})
	})
	if err != nil {
		return entity.Channel{}, fmt.Errorf("upsert channel %q: %w", c.Slug, err)
	}
	return s.ChannelBySlug(ctx, c.Slug)
}

func wrapNotFound(err error, kind, key string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s %q: %w", kind, key, repository.ErrNotFound)
	}
	return fmt.Errorf("%s %q: %w", kind, key, err)
}

func wrapConflict(err error, kind, key string) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return fmt.Errorf("%s %q: %w", kind, key, repository.ErrConflict)
	}
	return fmt.Errorf("%s %q: %w", kind, key, err)
}
