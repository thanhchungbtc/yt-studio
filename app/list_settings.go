package app

import (
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/service"
)

// ListSettings returns the whole settings table, grouped for the settings
// screen. It reads the validated in-memory cache, not the database.
func ListSettings(settings *service.Settings) []entity.Setting {
	return settings.All()
}
