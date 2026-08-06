package entity

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// ErrInvalidSetting is returned when a setting key or value is malformed.
var ErrInvalidSetting = errors.New("invalid setting")

// ErrSettingNotFound is returned by setting lookups for an unknown key.
var ErrSettingNotFound = errors.New("setting not found")

// SettingKey is the stable natural key of a runtime configuration value.
type SettingKey string

// String returns the underlying text of the key.
func (k SettingKey) String() string { return string(k) }

// The groups the settings screen renders as sections, in the order it renders
// them: how much runs at once, where the machine stops for a human, who does
// the work, then the pipeline in the order it runs — write, narrate, draw,
// package — then what a new video starts as, and last the knobs that are set
// once and left alone.
//
// A group is a task the operator is doing, not the subsystem that reads the
// row. Those two disagree in exactly one place and the task wins: the thumbnail
// section holds the two rows the built-in renderer reads privately alongside
// the two the icon use case fills into a request, because someone making a
// thumbnail look right wants all four and does not care which side of the port
// each lands on. Backend allows the two to still be told apart.
const (
	GroupPools     = "pools"
	GroupGates     = "gates"
	GroupProviders = "providers"
	GroupWriting   = "writing"
	GroupNarration = "narration"
	GroupSlides    = "slides"
	GroupThumbnail = "thumbnail"
	GroupVideo     = "video"
	GroupRetries   = "retries"
	GroupServer    = "server"
)

// The registry names a backend-scoped row belongs to. A row tagged with one of
// these is read only when that backend is the one selected, which is worth
// saying on the settings screen rather than leaving an operator to wonder why
// an edit changed nothing.
const (
	BackendNineRouter = "9router"
	BackendRunware    = "runware"
	BackendXTTS       = "xtts"
)

// The complete set of settings keys. Everything the server needs after the
// database is open lives here as a row, not in a config file.
const (
	SettingPoolLLMLimit     SettingKey = "pool.llm.limit"
	SettingPoolTTSLimit     SettingKey = "pool.tts.limit"
	SettingPoolImageLimit   SettingKey = "pool.image.limit"
	SettingPoolComposeLimit SettingKey = "pool.compose.limit"
	SettingPoolCacheLimit   SettingKey = "pool.cache.limit"
	SettingPoolUploadLimit  SettingKey = "pool.upload.limit"

	SettingGateBlueprintEnabled SettingKey = "gate.blueprint.enabled"
	SettingGateUploadEnabled    SettingKey = "gate.upload.enabled"

	// The routing table: one row per port in domain/provider, plus the dry run
	// that rides with the uploader. It grows when a port is added and never when a
	// backend is registered — a backend's own knobs live with the pipeline stage
	// they shape, so this group stays the short answer to "who does each job".
	SettingProviderLLM           SettingKey = "provider.llm"
	SettingProviderTTS           SettingKey = "provider.tts"
	SettingProviderSlide         SettingKey = "provider.slide"
	SettingProviderComposer      SettingKey = "provider.composer"
	SettingProviderThumbnail     SettingKey = "provider.thumbnail"
	SettingProviderThumbnailIcon SettingKey = "provider.thumbnail_icon"
	SettingProviderUploader      SettingKey = "provider.uploader"
	// SettingUploadDryRun rides with the uploader rather than with the gates,
	// because it is an argument to that backend rather than a rail of its own:
	// provider.uploader says who publishes, this says whether the publish is
	// real. A backend asked for a dry run does everything but the irreversible
	// call, which is the rehearsal a local backend cannot give — sample runs none
	// of the code and records every upload as dry whatever this says.
	SettingUploadDryRun SettingKey = "upload.dry_run"

	// SettingNineRouterModel picks which upstream the 9router backend routes to.
	// It is a settings row rather than a flag because it is the knob that gets
	// turned most while prompts are being tuned.
	SettingNineRouterModel SettingKey = "ninerouter.model"
	// SettingBlueprintChapterTolerancePercent bounds how far an accepted
	// blueprint's chapter count may fall from the one the video was briefed with.
	// It is a property of the writing rather than of the video: it decides
	// whether a roll is accepted or re-rolled, and every video is judged by
	// whatever it says at the moment the blueprint lands.
	SettingBlueprintChapterTolerancePercent SettingKey = "blueprint.chapter_tolerance_percent"

	// How a chapter should sound. These three cross the TTS port in the request
	// rather than being read by a backend: a voice belongs to whoever the video
	// is for, and the day a channel wants its own, these become its default
	// rather than a row that has to be renamed.
	//
	// SettingTTSVoice names a voice file on the narration server, e.g.
	// female_01.wav. Empty is meaningful — it lets the server use its own default
	// rather than failing a chapter over a name this end cannot verify.
	SettingTTSVoice SettingKey = "tts.voice"
	// SettingTTSLanguage is the two-letter code the model is asked to speak in.
	SettingTTSLanguage SettingKey = "tts.language"
	// SettingTTSSpeed is the playback rate asked of the server; 1.0 is unmodified.
	// It is the one float in the table: the useful range is 0.5..2.0 and the
	// interesting steps inside it are tenths, which an integer cannot express.
	SettingTTSSpeed SettingKey = "tts.speed"
	// The chunking pair is xtts's own, and named for it: a chapter is split
	// because XTTS degrades on long inputs, which is a fact about that server and
	// not about narration. No other backend inherits these.
	//
	// SettingXTTSChunkMinChars is the floor on a chunk's length in characters.
	// The chunk count follows from it, so it sets the size of the pieces a
	// chapter is synthesised in rather than their number.
	SettingXTTSChunkMinChars SettingKey = "xtts.chunk.min_chars"
	// SettingXTTSChunkSilenceMillis is the pause inserted between chunks when
	// they are rejoined, so a sentence boundary does not become an audible splice.
	SettingXTTSChunkSilenceMillis SettingKey = "xtts.chunk.silence_ms"

	// SettingRunwareModel picks which checkpoint the Runware backend draws with,
	// as an AIR identifier. A row for the same reason as the model above: it is
	// the knob that gets turned while a look is being found. It draws the
	// thumbnail icons too, when that port is pointed at the same backend.
	SettingRunwareModel SettingKey = "runware.model"
	// SettingRunwareWidth and SettingRunwareHeight are the geometry slides are
	// generated at. They exist because provider.SlideRequest carries a size that
	// no use case fills in: the slide's dimensions are a property of the backend
	// drawing it, not of the chapter asking for it.
	SettingRunwareWidth  SettingKey = "runware.width"
	SettingRunwareHeight SettingKey = "runware.height"

	// SettingThumbnailIconStyle is the clause appended to every icon prompt. It
	// lives outside the plan so restyling the whole grid costs the icons rather
	// than the words: change it and ten cheap generations re-run, with no model
	// asked to write captions again.
	SettingThumbnailIconStyle SettingKey = "thumbnail.icon.style"
	// SettingThumbnailIconSize is the square edge each icon is generated at.
	SettingThumbnailIconSize SettingKey = "thumbnail.icon.size"
	// SettingThumbnailFont names the typeface the renderer sets the headline and
	// captions in, as a filename under the resources fonts directory. A row
	// rather than a constant because the face is the loudest thing about a
	// thumbnail and the one worth trying alternatives for. It is untagged
	// deliberately: the HTML backend still to come sets type too.
	SettingThumbnailFont SettingKey = "thumbnail.font"
	// SettingThumbnailGridRows is how many rows the icons are laid out in;
	// columns follow from the cell count.
	SettingThumbnailGridRows SettingKey = "thumbnail.grid.rows"

	// What a new video is created with. These three are read once, at creation,
	// and then frozen into the row — an existing video keeps what it was made
	// with, which is why they are a section of their own rather than mixed in
	// beside knobs that apply to the next task.
	SettingVideoDefaultChapters SettingKey = "video.default_chapter_count"
	SettingVideoDefaultSlides   SettingKey = "video.default_slides_per_chapter"
	// SettingVideoDefaultThumbnailCells seeds a new video's grid width. The DAG
	// holds one icon task per cell from expansion onward, so the width a video
	// was created with is the width it keeps.
	SettingVideoDefaultThumbnailCells SettingKey = "video.default_thumbnail_cells"

	SettingTaskMaxAttempts     SettingKey = "task.max_attempts"
	SettingTaskRetryBaseMillis SettingKey = "task.retry_base_ms"
	SettingTaskRetryMaxMillis  SettingKey = "task.retry_max_ms"

	SettingSSECoalesceMillis SettingKey = "sse.coalesce_ms"
	SettingLogLevel          SettingKey = "log.level"
)

// SettingType is the declared type of a setting's text value. Every value is
// stored as text and read through a typed accessor that parses and validates
// it; an unparsable value fails loudly at startup.
type SettingType string

// The complete set of setting value types.
const (
	SettingTypeInt    SettingType = "int"
	SettingTypeBool   SettingType = "bool"
	SettingTypeString SettingType = "string"
	SettingTypeFloat  SettingType = "float"
)

// Valid reports whether the type is one of the known constants.
func (t SettingType) Valid() bool {
	switch t {
	case SettingTypeInt, SettingTypeBool, SettingTypeString, SettingTypeFloat:
		return true
	default:
		return false
	}
}

// Setting is a single runtime configuration value keyed by a stable key.
type Setting struct {
	Key         SettingKey
	Value       string
	Type        SettingType
	Group       string
	Description string
	// Min and Max bound numeric settings; they are advisory to the UI and enforced
	// by Validate. They are float64 so one pair of bounds serves both int and
	// float keys — a speed of 0.5..2.0 has no integer expression, and a second
	// pair of fields would be one more thing to keep in step.
	Min float64
	Max float64
	// Options constrains the value to a fixed set. It is deliberately not
	// persisted: which backends exist is a property of the running binary, not of
	// the database, so it is supplied at load time by whoever registered them. An
	// empty Options means the value is unconstrained.
	Options []string
	// Optional allows a string setting to be empty, for the keys where empty is a
	// meaningful value rather than a missing one.
	Optional bool
	// Backend names the registry entry that reads this row, empty when the row is
	// not backend-specific. Like Options it is a property of the running binary
	// and is not persisted: what reads a row is a fact about the code, and a
	// stored copy would go stale the first time a backend was rewritten.
	//
	// It exists so the settings screen can say "xtts only" on a row and dim it
	// when that backend is not the one selected. A row whose backend is idle
	// still validates and still holds its value — it is simply not being read,
	// and that is the thing worth showing rather than hiding.
	Backend   string
	UpdatedAt time.Time
}

// AllowsValue reports whether v satisfies the Options constraint.
func (s Setting) AllowsValue(v string) bool {
	if len(s.Options) == 0 {
		return true
	}
	for _, o := range s.Options {
		if o == v {
			return true
		}
	}
	return false
}

// Validate parses the value against the declared type and bounds. It is called
// on every write and on the whole table at startup.
func (s Setting) Validate() error {
	if strings.TrimSpace(string(s.Key)) == "" {
		return fmt.Errorf("%w: key must not be empty", ErrInvalidSetting)
	}
	switch s.Type {
	case SettingTypeInt:
		n, err := strconv.Atoi(s.Value)
		if err != nil {
			return fmt.Errorf("%w %q: %q is not an integer", ErrInvalidSetting, s.Key, s.Value)
		}
		if s.Min != s.Max && (float64(n) < s.Min || float64(n) > s.Max) {
			return fmt.Errorf("%w %q: %d is outside %g..%g", ErrInvalidSetting, s.Key, n, s.Min, s.Max)
		}
	case SettingTypeFloat:
		f, err := strconv.ParseFloat(s.Value, 64)
		if err != nil {
			return fmt.Errorf("%w %q: %q is not a number", ErrInvalidSetting, s.Key, s.Value)
		}
		// NaN fails every comparison below, so it would slip past a bounds check
		// written as "outside the range" and reach a backend as a speed.
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return fmt.Errorf("%w %q: %q is not a finite number", ErrInvalidSetting, s.Key, s.Value)
		}
		if s.Min != s.Max && (f < s.Min || f > s.Max) {
			return fmt.Errorf("%w %q: %g is outside %g..%g", ErrInvalidSetting, s.Key, f, s.Min, s.Max)
		}
	case SettingTypeBool:
		if _, err := strconv.ParseBool(s.Value); err != nil {
			return fmt.Errorf("%w %q: %q is not a boolean", ErrInvalidSetting, s.Key, s.Value)
		}
	case SettingTypeString:
		if s.Value == "" && !s.Optional {
			return fmt.Errorf("%w %q: value must not be empty", ErrInvalidSetting, s.Key)
		}
	default:
		return fmt.Errorf("%w %q: unknown type %q", ErrInvalidSetting, s.Key, s.Type)
	}
	if !s.AllowsValue(s.Value) {
		return fmt.Errorf("%w %q: %q is not one of %s",
			ErrInvalidSetting, s.Key, s.Value, strings.Join(s.Options, ", "))
	}
	return nil
}

// Int parses the value as an integer.
func (s Setting) Int() (int, error) {
	n, err := strconv.Atoi(s.Value)
	if err != nil {
		return 0, fmt.Errorf("%w %q: %q is not an integer", ErrInvalidSetting, s.Key, s.Value)
	}
	return n, nil
}

// Float parses the value as a floating-point number.
func (s Setting) Float() (float64, error) {
	f, err := strconv.ParseFloat(s.Value, 64)
	if err != nil {
		return 0, fmt.Errorf("%w %q: %q is not a number", ErrInvalidSetting, s.Key, s.Value)
	}
	return f, nil
}

// Bool parses the value as a boolean.
func (s Setting) Bool() (bool, error) {
	b, err := strconv.ParseBool(s.Value)
	if err != nil {
		return false, fmt.Errorf("%w %q: %q is not a boolean", ErrInvalidSetting, s.Key, s.Value)
	}
	return b, nil
}

// Duration parses an integer-milliseconds value as a duration.
func (s Setting) Duration() (time.Duration, error) {
	n, err := s.Int()
	if err != nil {
		return 0, err
	}
	return time.Duration(n) * time.Millisecond, nil
}

// PoolLimitKey maps a pool to the settings key holding its limit.
func PoolLimitKey(p Pool) SettingKey {
	switch p {
	case PoolLLM:
		return SettingPoolLLMLimit
	case PoolTTS:
		return SettingPoolTTSLimit
	case PoolImage:
		return SettingPoolImageLimit
	case PoolCompose:
		return SettingPoolComposeLimit
	case PoolCache:
		return SettingPoolCacheLimit
	case PoolUpload:
		return SettingPoolUploadLimit
	default:
		return ""
	}
}

// GateEnabledKey maps a gate to the settings key that turns it on or off.
func GateEnabledKey(g GateKind) SettingKey {
	switch g {
	case GateBlueprint:
		return SettingGateBlueprintEnabled
	case GateUpload:
		return SettingGateUploadEnabled
	case GateNone:
		return ""
	default:
		return ""
	}
}

// DefaultSettings is the complete seeded settings table. The seed is an upsert
// by key, so a fresh database and a ten-times-seeded database end up in the
// same state.
//
// The order here is the order the settings screen shows: rows are sorted by
// their position in this slice rather than by key, because "voice, language,
// speed" is how an operator thinks about narration and "chunk, chunk, language,
// speed, voice" is only how the key names happen to sort.
func DefaultSettings() []Setting {
	return []Setting{
		{Key: SettingPoolLLMLimit, Value: "2", Type: SettingTypeInt, Group: GroupPools, Min: 1, Max: MaxPoolLimit, Description: "Concurrent LLM calls across all videos and channels."},
		{Key: SettingPoolTTSLimit, Value: "2", Type: SettingTypeInt, Group: GroupPools, Min: 1, Max: MaxPoolLimit, Description: "Concurrent narration syntheses."},
		{Key: SettingPoolImageLimit, Value: "2", Type: SettingTypeInt, Group: GroupPools, Min: 1, Max: MaxPoolLimit, Description: "Concurrent slide generations — usually the binding constraint."},
		{Key: SettingPoolComposeLimit, Value: "2", Type: SettingTypeInt, Group: GroupPools, Min: 1, Max: MaxPoolLimit, Description: "Concurrent clip and concat compositions."},
		{Key: SettingPoolCacheLimit, Value: "32", Type: SettingTypeInt, Group: GroupPools, Min: 1, Max: MaxPoolLimit, Description: "Concurrent slide-prompt cache reads; must never be the bottleneck."},
		{Key: SettingPoolUploadLimit, Value: "1", Type: SettingTypeInt, Group: GroupPools, Min: 1, Max: MaxPoolLimit, Description: "Concurrent uploads."},

		{Key: SettingGateBlueprintEnabled, Value: "true", Type: SettingTypeBool, Group: GroupGates, Description: "Pause after the blueprint for human review."},
		{Key: SettingGateUploadEnabled, Value: "true", Type: SettingTypeBool, Group: GroupGates, Description: "Pause before upload for human review."},

		{Key: SettingProviderLLM, Value: "sample", Type: SettingTypeString, Group: GroupProviders, Description: "Backend for blueprint, script, prompts and metadata."},
		{Key: SettingProviderTTS, Value: "sample", Type: SettingTypeString, Group: GroupProviders, Description: "Backend for narration."},
		{Key: SettingProviderSlide, Value: "sample", Type: SettingTypeString, Group: GroupProviders, Description: "Backend for slides."},
		{Key: SettingProviderComposer, Value: "ffmpeg", Type: SettingTypeString, Group: GroupProviders, Description: "Backend for clip and concat composition."},
		{Key: SettingProviderThumbnail, Value: "builtin", Type: SettingTypeString, Group: GroupProviders, Description: "Backend that renders the thumbnail image."},
		{Key: SettingProviderThumbnailIcon, Value: "sample", Type: SettingTypeString, Group: GroupProviders, Description: "Backend for the thumbnail's grid icons; selected apart from slides."},
		{Key: SettingProviderUploader, Value: "sample", Type: SettingTypeString, Group: GroupProviders, Description: "Backend for publishing."},
		//nolint:lll // one row, one line
		{Key: SettingUploadDryRun, Value: "true", Type: SettingTypeBool, Group: GroupProviders, Description: "The uploader does everything but the irreversible call, and produces a local receipt. Turning this off is what makes a publish real."},

		//nolint:lll // one row, one line
		{Key: SettingNineRouterModel, Value: "ag/gemini-3-flash", Type: SettingTypeString, Group: GroupWriting, Backend: BackendNineRouter, Description: "Which upstream the 9router backend routes to, e.g. ag/gemini-3-flash. See GET /v1/models on the gateway."},
		//nolint:lll // one row, one line
		{Key: SettingBlueprintChapterTolerancePercent, Value: "20", Type: SettingTypeInt, Group: GroupWriting, Min: 0, Max: 100, Description: "How far an accepted blueprint's chapter count may fall from the target, as a percentage. A roll outside it is rejected and written again."},

		//nolint:lll // one row, one line
		{Key: SettingTTSVoice, Value: "", Type: SettingTypeString, Group: GroupNarration, Optional: true, Description: "Voice file on the narration server, e.g. female_01.wav. Empty lets the server pick its own default."},
		{Key: SettingTTSLanguage, Value: "en", Type: SettingTypeString, Group: GroupNarration, Description: "Two-letter code the narration model is asked to speak."},
		{Key: SettingTTSSpeed, Value: "1.0", Type: SettingTypeFloat, Group: GroupNarration, Min: 0.5, Max: 2.0, Description: "Playback rate asked of the narration server; 1.0 is unmodified."},
		//nolint:lll // one row, one line
		{Key: SettingXTTSChunkMinChars, Value: "250", Type: SettingTypeInt, Group: GroupNarration, Backend: BackendXTTS, Min: 50, Max: 5000, Description: "Floor on a narration chunk's length in characters. A chapter is synthesised in pieces at least this long, because XTTS degrades on long inputs."},
		//nolint:lll // one row, one line
		{Key: SettingXTTSChunkSilenceMillis, Value: "200", Type: SettingTypeInt, Group: GroupNarration, Backend: BackendXTTS, Min: 0, Max: 2000, Description: "Pause inserted between narration chunks when they are rejoined, so a sentence boundary is not an audible splice."},

		//nolint:lll // one row, one line
		{Key: SettingRunwareModel, Value: "runware:100@1", Type: SettingTypeString, Group: GroupSlides, Backend: BackendRunware, Description: "Checkpoint the Runware backend draws with, as an AIR identifier, e.g. runware:100@1. It draws the thumbnail icons too, when that port is pointed at it."},
		//nolint:lll // one row, one line
		{Key: SettingRunwareWidth, Value: "1344", Type: SettingTypeInt, Group: GroupSlides, Backend: BackendRunware, Min: 128, Max: 2048, Description: "Width slides are generated at. The composer frames them at 1344x768, so anything else is resampled."},
		//nolint:lll // one row, one line
		{Key: SettingRunwareHeight, Value: "768", Type: SettingTypeInt, Group: GroupSlides, Backend: BackendRunware, Min: 128, Max: 2048, Description: "Height slides are generated at. Most checkpoints require both edges to be a multiple of 64."},

		{Key: SettingThumbnailIconStyle, Value: "", Type: SettingTypeString, Group: GroupThumbnail, Optional: true, Description: "Appended to every thumbnail icon prompt."},
		{Key: SettingThumbnailIconSize, Value: "512", Type: SettingTypeInt, Group: GroupThumbnail, Min: 64, Max: 2048, Description: "Square edge, in pixels, each thumbnail icon is generated at."},
		//nolint:lll // one row, one line
		{Key: SettingThumbnailFont, Value: "CabinSketch-Bold.ttf", Type: SettingTypeString, Group: GroupThumbnail, Description: "Typeface for the thumbnail headline and captions, from the resources fonts directory."},
		//nolint:lll // one row, one line
		{Key: SettingThumbnailGridRows, Value: "2", Type: SettingTypeInt, Group: GroupThumbnail, Min: 1, Max: 4, Description: "Rows the thumbnail's icon grid is laid out in; the columns follow from the tile count."},

		//nolint:lll // one row, one line
		{Key: SettingVideoDefaultChapters, Value: "50", Type: SettingTypeInt, Group: GroupVideo, Min: MinChapterCount, Max: MaxChapterCount, Description: "Chapters created for a new video when unspecified."},
		//nolint:lll // one row, one line
		{Key: SettingVideoDefaultSlides, Value: "2", Type: SettingTypeInt, Group: GroupVideo, Min: MinSlidesPerChapter, Max: MaxSlidesPerChapter, Description: "Slides generated per chapter when unspecified."},
		//nolint:lll // one row, one line
		{Key: SettingVideoDefaultThumbnailCells, Value: "12", Type: SettingTypeInt, Group: GroupVideo, Min: MinThumbnailCells, Max: MaxThumbnailCells, Description: "Tiles in a new video's thumbnail grid; one icon is generated per tile. Twelve is two rows of six."},

		//nolint:lll // one row, one line
		{Key: SettingTaskMaxAttempts, Value: "1", Type: SettingTypeInt, Group: GroupRetries, Min: 1, Max: 20, Description: "Attempts before a task is permanently failed. One means a failure surfaces immediately rather than costing a second generation; raise it once the prompts are settled."},
		{Key: SettingTaskRetryBaseMillis, Value: "250", Type: SettingTypeInt, Group: GroupRetries, Min: 1, Max: 60000, Description: "Initial retry backoff."},
		{Key: SettingTaskRetryMaxMillis, Value: "30000", Type: SettingTypeInt, Group: GroupRetries, Min: 1, Max: 3600000, Description: "Maximum retry backoff."},

		{Key: SettingSSECoalesceMillis, Value: "50", Type: SettingTypeInt, Group: GroupServer, Min: 1, Max: 5000, Description: "Minimum interval between event batches per video."},
		{Key: SettingLogLevel, Value: "info", Type: SettingTypeString, Group: GroupServer, Description: "debug, info, warn or error; applied without a restart."},
	}
}

// settingOrder is each key's position in DefaultSettings, so a section can be
// presented in the order it was written. Built once: the table is read on every
// settings screen and the slice is rebuilt on every call to DefaultSettings.
var settingOrder = func() map[SettingKey]int {
	defaults := DefaultSettings()
	order := make(map[SettingKey]int, len(defaults))
	for i, d := range defaults {
		order[d.Key] = i
	}
	return order
}()

// SettingOrder returns a key's seeded position. A key that is not seeded sorts
// after every one that is, rather than being dropped: an unknown row in the
// table is a thing to show the operator, not to hide from them.
func SettingOrder(k SettingKey) int {
	if i, ok := settingOrder[k]; ok {
		return i
	}
	return len(settingOrder)
}
