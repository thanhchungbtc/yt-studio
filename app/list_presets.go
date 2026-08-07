package app

import "github.com/tbui/yt-studio/domain/entity"

// ListPresets returns every preset this build ships with. It takes no settings
// reader on purpose: which one is in force is a comparison the caller already
// holds both sides of, and answering it here would be a snapshot.
func ListPresets() []entity.Preset {
	return entity.BuiltinPresets()
}
