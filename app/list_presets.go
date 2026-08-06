package app

import "github.com/tbui/yt-studio/domain/entity"

// ListPresets returns every preset this build ships with.
//
// It takes no settings reader on purpose: a preset is the values it would
// write, and whether it is the one currently in force is a comparison the caller
// already holds both sides of. Computing "active" here would mean the answer
// were a snapshot taken at request time, which is exactly the staleness the
// derived-not-stored rule on entity.Preset avoids.
func ListPresets() []entity.Preset {
	return entity.BuiltinPresets()
}
