package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/adapters/sqlite/sqlcgen"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

var (
	_ repository.SettingReader = (*Store)(nil)
	_ repository.SettingWriter = (*Store)(nil)
)

// SettingByKey reads one runtime configuration row.
func (s *Store) SettingByKey(ctx context.Context, key entity.SettingKey) (entity.Setting, error) {
	row, err := s.rq.GetSetting(ctx, string(key))
	if err != nil {
		return entity.Setting{}, wrapNotFound(err, "setting", string(key))
	}
	return settingFromRow(row), nil
}

// ListSettings reads the whole settings table, grouped for the settings screen.
func (s *Store) ListSettings(ctx context.Context) ([]entity.Setting, error) {
	rows, err := s.rq.ListSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list settings: %w", err)
	}
	out := make([]entity.Setting, 0, len(rows))
	for _, r := range rows {
		out = append(out, settingFromRow(r))
	}
	return out, nil
}

// UpdateSetting validates the new value against the row's declared type and
// bounds, then writes it. The change applies without a daemon restart.
func (s *Store) UpdateSetting(ctx context.Context, key entity.SettingKey, value string) (entity.Setting, error) {
	current, err := s.SettingByKey(ctx, key)
	if err != nil {
		return entity.Setting{}, err
	}
	candidate := current
	candidate.Value = value
	if err := candidate.Validate(); err != nil {
		return entity.Setting{}, err
	}
	now := time.Now()
	if err := s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.UpdateSettingValue(ctx, sqlcgen.UpdateSettingValueParams{
			Value:     value,
			UpdatedAt: toUnix(now),
			Key:       string(key),
		})
	}); err != nil {
		return entity.Setting{}, fmt.Errorf("update setting %q: %w", key, err)
	}
	candidate.UpdatedAt = now
	return candidate, nil
}

// UpsertSettings is the seed path: idempotent by key, so a fresh database and a
// ten-times-seeded database end up in the same state. An existing row keeps its
// operator-set value; only the metadata around it is refreshed.
func (s *Store) UpsertSettings(ctx context.Context, settings []entity.Setting) error {
	now := toUnix(time.Now())
	for _, st := range settings {
		if err := st.Validate(); err != nil {
			return fmt.Errorf("seed setting: %w", err)
		}
	}
	return s.doTx(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		for _, st := range settings {
			if err := q.UpsertSetting(ctx, sqlcgen.UpsertSettingParams{
				Key:         string(st.Key),
				Value:       st.Value,
				Type:        string(st.Type),
				Grp:         st.Group,
				Description: st.Description,
				MinValue:    int64(st.Min),
				MaxValue:    int64(st.Max),
				UpdatedAt:   now,
			}); err != nil {
				return fmt.Errorf("upsert setting %q: %w", st.Key, err)
			}
		}
		return nil
	})
}
