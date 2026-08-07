package app

import (
	"fmt"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/service"
)

// CheckPresets proves every built-in preset against the registered backends, at
// startup. It is why presets live on the server rather than in the browser: one
// naming a renamed backend fails the boot instead of moving half the ports when
// somebody clicks it.
func CheckPresets(settings *service.Settings) error {
	for _, preset := range entity.BuiltinPresets() {
		if err := CheckPreset(settings, preset); err != nil {
			return err
		}
	}
	return nil
}

// CheckPreset validates one preset's shape and every value it would write. The
// halves split by what can see what: entity knows the keys, and only the
// settings service knows which backend names the registry made legal.
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
