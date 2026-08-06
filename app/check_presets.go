package app

import (
	"fmt"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/service"
)

// CheckPresets proves every built-in preset against the backends this binary
// registered. It runs at startup, after the registry has been filled and the
// settings table loaded.
//
// This is the whole reason presets are resolved on the server rather than
// hardcoded in the browser: a preset naming a backend that was renamed or
// removed fails the boot, in the same breath as an unparsable settings row,
// instead of being discovered when someone clicks it and half the ports move.
func CheckPresets(settings *service.Settings) error {
	for _, preset := range entity.BuiltinPresets() {
		if err := CheckPreset(settings, preset); err != nil {
			return err
		}
	}
	return nil
}

// CheckPreset validates one preset's shape and every value it would write.
//
// The two halves are split by what can see what: entity knows which keys exist,
// and only the settings service knows which backend names are legal, because
// that set comes from the registry at load time and is deliberately absent from
// the database.
func CheckPreset(settings *service.Settings, preset entity.Preset) error {
	if err := preset.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrValidation, err)
	}
	for _, v := range preset.Values {
		if err := settings.Check(v.Key, v.Value); err != nil {
			return fmt.Errorf("%w: preset %q: %w", ErrValidation, preset.Name, err)
		}
	}
	return nil
}
