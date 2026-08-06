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

// Preset is a named patch over the settings table: the rows to write, and
// nothing at all about the rows it does not name.
//
// A patch rather than a snapshot. A snapshot of the whole table would carry the
// log level, the pool limits and the retry policy along with it, so switching to
// the mock backends would also reset how loudly the server logs — and it would
// make a preset undeclarable for any port whose backend this build does not
// register. Naming only what it means to change is both.
//
// Values is a slice rather than a map so applying one is deterministic: a
// failure names the same row every time, and two applications of the same preset
// log the same sequence.
//
// Which preset is "active" is deliberately not stored anywhere. It is derived —
// a preset is active when every row it names already holds the value it would
// write. A persisted "current preset" would start lying the moment a single
// provider row was edited by hand, and keeping it honest would mean invalidation
// logic in every settings write.
type Preset struct {
	Name        string
	Title       string
	Description string
	Values      []PresetValue
}

// Validate checks the preset's shape.
//
// It deliberately does not check values. What a provider row may legally hold
// depends on which backends this binary registered, which is not visible from
// here; service.Settings does that half, and main runs it over every built-in at
// startup so a preset naming a backend that no longer exists fails the boot
// rather than a task.
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

// BuiltinPresets is every preset the binary ships with.
//
// They live in code rather than in the settings table for the same reason
// Setting.Options is not persisted: which backends exist is a property of the
// running binary, not of the database. Seeded rows would also be clobbered by
// the next seed, and a preset naming a backend that had been deleted would sit
// in the database saying nothing.
//
// Each one names all seven provider ports, including the ports where there is
// only one backend to name. A preset that answers "who does the work" for four
// ports and stays silent about three makes the active indicator ambiguous — the
// reader cannot tell whether the silence means "leave it" or "it already
// matches".
//
// None of them touches upload.dry_run or the gates. A preset says who does the
// work; it does not say what is allowed to escape. Those two rails stay under a
// separate deliberate flip, which is worth the extra click.
func BuiltinPresets() []Preset {
	return []Preset{
		{
			Name:  "mock",
			Title: "Mock",
			//nolint:lll // one description, one line
			Description: "Every port on its mock backend. Nothing leaves the machine and nothing costs a generation; each answer is derived from the request, so re-running a task dedupes onto the asset it produced last time.",
			Values: []PresetValue{
				{SettingProviderLLM, "mock"},
				{SettingProviderTTS, "mock"},
				{SettingProviderSlide, "mock"},
				{SettingProviderComposer, "mock"},
				{SettingProviderThumbnail, "mock"},
				{SettingProviderThumbnailIcon, "mock"},
				{SettingProviderUploader, "mock"},
			},
		},
		{
			Name:  "sample",
			Title: "Sample",
			//nolint:lll // one description, one line
			Description: "The real compose and render path over canned content: ffmpeg cuts the clips and the built-in renderer draws the thumbnail, from sample narration and stills. This is what exercises the layout and the encoder without spending anything.",
			Values: []PresetValue{
				// There is no sample LLM: the words are the cheap part, and a canned
				// blueprint would not have the chapter count the graph is built from.
				{SettingProviderLLM, "mock"},
				{SettingProviderTTS, "sample"},
				{SettingProviderSlide, "sample"},
				{SettingProviderComposer, "ffmpeg"},
				{SettingProviderThumbnail, "builtin"},
				{SettingProviderThumbnailIcon, "sample"},
				{SettingProviderUploader, "mock"},
			},
		},
		{
			Name:  "live",
			Title: "Live",
			//nolint:lll // one description, one line
			Description: "The paid and external backends: words through the 9router gateway, narration on the AllTalk server, images from Runware. Publishing is still simulated, because mock is the only uploader this build registers — flip upload.dry_run yourself when that changes.",
			Values: []PresetValue{
				{SettingProviderLLM, "9router"},
				{SettingProviderTTS, "xtts"},
				{SettingProviderSlide, "runware"},
				{SettingProviderComposer, "ffmpeg"},
				{SettingProviderThumbnail, "builtin"},
				{SettingProviderThumbnailIcon, "runware"},
				// Named rather than omitted so the preset stays a complete answer for
				// all seven ports; it is the only uploader there is.
				{SettingProviderUploader, "mock"},
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
