package entity

import (
	"errors"
	"fmt"
)

// ErrPresetNotFound is returned when no preset carries the requested name.
var ErrPresetNotFound = errors.New("preset not found")

// ErrInvalidPreset reports a preset whose shape is wrong: no name, a key that is
// not a settings key, or the same key written twice.
var ErrInvalidPreset = errors.New("invalid preset")

// PresetValue is one row a preset writes.
type PresetValue struct {
	Key   SettingKey
	Value string
}

// Preset is a named patch over the settings table, not a snapshot: a snapshot
// would drag the log level, pool limits and retry policy along with the
// backends. Values is a slice so applying one is deterministic.
//
// Which preset is active is derived, never stored — a preset is active when
// every row it names already holds the value it would write, and a persisted
// answer would start lying the moment one row was edited by hand.
type Preset struct {
	Name        string
	Title       string
	Description string
	Values      []PresetValue
}

// Validate checks the preset's shape, not its values: what a provider row may
// hold depends on which backends this binary registered, which service.Settings
// checks at startup so a stale preset fails the boot rather than a task.
func (p Preset) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("%w: name must not be empty", ErrInvalidPreset)
	}
	if len(p.Values) == 0 {
		return fmt.Errorf("%w %q: writes no rows", ErrInvalidPreset, p.Name)
	}
	known := make(map[SettingKey]bool, len(DefaultSettings()))
	for _, d := range DefaultSettings() {
		known[d.Key] = true
	}
	seen := make(map[SettingKey]bool, len(p.Values))
	for _, v := range p.Values {
		if !known[v.Key] {
			return fmt.Errorf("%w %q: %q is not a settings key", ErrInvalidPreset, p.Name, v.Key)
		}
		if seen[v.Key] {
			return fmt.Errorf("%w %q: %q is written twice", ErrInvalidPreset, p.Name, v.Key)
		}
		seen[v.Key] = true
	}
	return nil
}

// BuiltinPresets is every preset the binary ships with. They live in code
// because which backends exist is a property of the running binary.
//
// Each names all seven ports, even where only one backend exists: silence on a
// port would make the active indicator ambiguous. None touches upload.dry_run
// or the gates — a preset says who does the work, not what may escape.
func BuiltinPresets() []Preset {
	return []Preset{
		{
			Name:  "sample",
			Title: "Sample",
			//nolint:lll // one description, one line
			Description: "Everything local: ffmpeg cuts the clips and the built-in renderer draws the thumbnail, over operator-supplied narration and stills. Nothing leaves the machine and nothing costs a generation.",
			Values: []PresetValue{
				// The sample LLM generates rather than reads: a canned blueprint
				// could not answer the chapter count the graph is built from.
				{SettingProviderLLM, "sample"},
				{SettingProviderTTS, "sample"},
				{SettingProviderSlide, "sample"},
				{SettingProviderComposer, "ffmpeg"},
				{SettingProviderThumbnail, "builtin"},
				{SettingProviderThumbnailIcon, "sample"},
				{SettingProviderUploader, "sample"},
			},
		},
		{
			Name:  "live",
			Title: "Live",
			//nolint:lll // one description, one line
			Description: "The paid and external backends: words through the 9router gateway, narration on the AllTalk server, images from Runware. Publishing is still simulated, because this build registers no real uploader — flip upload.dry_run yourself when that changes.",
			Values: []PresetValue{
				{SettingProviderLLM, "9router"},
				{SettingProviderTTS, "xtts"},
				{SettingProviderSlide, "runware"},
				{SettingProviderComposer, "ffmpeg"},
				{SettingProviderThumbnail, "builtin"},
				{SettingProviderThumbnailIcon, "runware"},
				// Named rather than omitted, to keep all seven ports answered. No
				// build here registers an uploader that publishes.
				{SettingProviderUploader, "sample"},
			},
		},
	}
}

// PresetByName looks one up.
func PresetByName(name string) (Preset, error) {
	for _, p := range BuiltinPresets() {
		if p.Name == name {
			return p, nil
		}
	}
	return Preset{}, fmt.Errorf("%w: %q", ErrPresetNotFound, name)
}
