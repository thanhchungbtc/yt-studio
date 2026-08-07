package repository

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
)

// SettingReader reads runtime configuration rows.
type SettingReader interface {
	SettingByKey(ctx context.Context, key entity.SettingKey) (entity.Setting, error)
	ListSettings(ctx context.Context) ([]entity.Setting, error)
}

// SettingWriter updates runtime configuration rows. A change applies without a
// server restart.
type SettingWriter interface {
	UpdateSetting(ctx context.Context, key entity.SettingKey, value string) (entity.Setting, error)
	// UpsertSettings is the seed path, idempotent by key.
	UpsertSettings(ctx context.Context, settings []entity.Setting) error
}
