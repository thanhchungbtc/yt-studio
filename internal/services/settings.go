package services

import (
	"context"

	"github.com/tbui/yt-studio/internal/app"
	"github.com/tbui/yt-studio/internal/domain/entity"
)

// SettingsService reads and writes the runtime settings.
type SettingsService struct{ b *Backend }

// NewSettingsService creates a SettingsService.
func NewSettingsService(b *Backend) *SettingsService { return &SettingsService{b: b} }

// List returns every setting. Secret values are never returned.
func (s *SettingsService) List() []SettingDTO {
	rows := app.ListSettings(s.b.Settings)
	out := make([]SettingDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, settingFrom(r))
	}
	return out
}

// Update sets one setting; it applies immediately.
func (s *SettingsService) Update(ctx context.Context, key, value string) (SettingDTO, error) {
	row, err := app.UpdateSetting(ctx, s.b.Settings, s.b.Scheduler, s.b.Coalescer, s.b.LogLevel,
		entity.SettingKey(key), value)
	if err != nil {
		return SettingDTO{}, err
	}
	return settingFrom(row), nil
}
