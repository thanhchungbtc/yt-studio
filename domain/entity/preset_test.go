package entity_test

import (
	"errors"
	"testing"

	"github.com/tbui/yt-studio/domain/entity"
)

// Every provider port a preset must answer for. A preset that leaves one out
// makes the active indicator ambiguous: the reader cannot tell whether the
// silence means "leave this one alone" or "it already matches".
var providerKeys = []entity.SettingKey{
	entity.SettingProviderLLM,
	entity.SettingProviderTTS,
	entity.SettingProviderSlide,
	entity.SettingProviderComposer,
	entity.SettingProviderThumbnail,
	entity.SettingProviderThumbnailIcon,
	entity.SettingProviderUploader,
}

func TestBuiltinPresetsAreWellFormed(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool)
	for _, p := range entity.BuiltinPresets() {
		if err := p.Validate(); err != nil {
			t.Errorf("preset %q: %v", p.Name, err)
		}
		if seen[p.Name] {
			t.Errorf("preset %q is declared twice", p.Name)
		}
		seen[p.Name] = true
		if p.Title == "" || p.Description == "" {
			t.Errorf("preset %q has no title or description; the settings screen shows both", p.Name)
		}
	}
}

func TestBuiltinPresetsAnswerEveryProviderPort(t *testing.T) {
	t.Parallel()

	for _, p := range entity.BuiltinPresets() {
		named := make(map[entity.SettingKey]bool, len(p.Values))
		for _, v := range p.Values {
			named[v.Key] = true
		}
		for _, key := range providerKeys {
			if !named[key] {
				t.Errorf("preset %q does not name %s", p.Name, key)
			}
		}
	}
}

// The two rails a preset must not touch. Which backend does the work is one
// question; what is allowed to leave the machine is another, and a one-click
// switch must not answer the second by accident.
func TestBuiltinPresetsLeaveTheSafetyRailsAlone(t *testing.T) {
	t.Parallel()

	forbidden := map[entity.SettingKey]bool{
		entity.SettingUploadDryRun:         true,
		entity.SettingGateBlueprintEnabled: true,
		entity.SettingGateUploadEnabled:    true,
	}
	for _, p := range entity.BuiltinPresets() {
		for _, v := range p.Values {
			if forbidden[v.Key] {
				t.Errorf("preset %q writes %s, which is a safety rail, not a backend", p.Name, v.Key)
			}
		}
	}
}

func TestPresetValidateRejectsMalformedPresets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		preset entity.Preset
	}{
		{"no name", entity.Preset{Values: []entity.PresetValue{{Key: entity.SettingProviderLLM, Value: "sample"}}}},
		{"writes nothing", entity.Preset{Name: "empty"}},
		{
			"unknown key",
			entity.Preset{Name: "x", Values: []entity.PresetValue{{Key: "provider.telepathy", Value: "sample"}}},
		},
		{
			"same key twice",
			entity.Preset{Name: "x", Values: []entity.PresetValue{
				{Key: entity.SettingProviderLLM, Value: "sample"},
				{Key: entity.SettingProviderLLM, Value: "9router"},
			}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.preset.Validate(); !errors.Is(err, entity.ErrInvalidPreset) {
				t.Fatalf("err = %v, want ErrInvalidPreset", err)
			}
		})
	}
}

func TestPresetByNameReportsUnknownNames(t *testing.T) {
	t.Parallel()

	if _, err := entity.PresetByName("sample"); err != nil {
		t.Fatalf("sample: %v", err)
	}
	if _, err := entity.PresetByName("nonesuch"); !errors.Is(err, entity.ErrPresetNotFound) {
		t.Fatalf("err = %v, want ErrPresetNotFound", err)
	}
}
