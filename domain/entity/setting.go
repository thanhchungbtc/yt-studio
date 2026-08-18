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

// Sections of the settings screen, in render order. A group is the task the
// operator is doing, not the subsystem that reads the row.
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

// Registry names a backend-scoped row may belong to; such a row is read only
// while that backend is selected, which the settings screen shows.
const (
	BackendNineRouter = "9router"
	BackendRunware    = "runware"
	BackendXTTS       = "xtts"
	BackendKokoro     = "kokoro"
)

// The complete set of settings keys. Everything the server needs after the
// database is open is a row here, not a config file.
const (
	SettingPoolLLMLimit     SettingKey = "pool.llm.limit"
	SettingPoolTTSLimit     SettingKey = "pool.tts.limit"
	SettingPoolImageLimit   SettingKey = "pool.image.limit"
	SettingPoolComposeLimit SettingKey = "pool.compose.limit"
	SettingPoolCacheLimit   SettingKey = "pool.cache.limit"
	SettingPoolUploadLimit  SettingKey = "pool.upload.limit"

	SettingGateBlueprintEnabled SettingKey = "gate.blueprint.enabled"
	SettingGateUploadEnabled    SettingKey = "gate.upload.enabled"

	// One row per port in domain/provider: this group grows when a port is added,
	// never when a backend is registered.
	SettingProviderLLM           SettingKey = "provider.llm"
	SettingProviderTTS           SettingKey = "provider.tts"
	SettingProviderSlide         SettingKey = "provider.slide"
	SettingProviderComposer      SettingKey = "provider.composer"
	SettingProviderThumbnail     SettingKey = "provider.thumbnail"
	SettingProviderThumbnailIcon SettingKey = "provider.thumbnail_icon"
	SettingProviderUploader      SettingKey = "provider.uploader"
	// SettingUploadDryRun is an argument to the uploader, not a gate of its own:
	// provider.uploader says who publishes, this says whether it is real.
	SettingUploadDryRun SettingKey = "upload.dry_run"

	// SettingNineRouterURL is the gateway root the 9router backend talks to.
	SettingNineRouterURL SettingKey = "ninerouter.url"
	// SettingNineRouterKey authenticates against that gateway. Empty is
	// meaningful: a gateway running with auth off needs none, which is usual.
	SettingNineRouterKey SettingKey = "ninerouter.key"
	// SettingNineRouterModel picks which upstream the 9router backend routes to.
	SettingNineRouterModel SettingKey = "ninerouter.model"
	// SettingBlueprintChapterTolerancePercent bounds how far an accepted
	// blueprint's chapter count may fall from the briefed target.
	SettingBlueprintChapterTolerancePercent SettingKey = "blueprint.chapter_tolerance_percent"

	// The narration rows are named for the engine because none is portable: a
	// voice is an opaque handle scoped to one server, and a language code or a
	// speed range one engine accepts is not one another does.
	//
	// SettingXTTSURL is the AllTalk server root. The root only: the endpoints are
	// appended to it.
	SettingXTTSURL SettingKey = "xtts.url"
	// SettingXTTSVoice names a voice file on the AllTalk server. Empty is
	// meaningful — it lets the server pick its own default.
	SettingXTTSVoice SettingKey = "xtts.voice"
	// SettingXTTSLanguage is the two-letter code the model is asked to speak in.
	SettingXTTSLanguage SettingKey = "xtts.language"
	// SettingXTTSSpeed is the playback rate asked of the server; 1.0 is unmodified.
	SettingXTTSSpeed SettingKey = "xtts.speed"
	// SettingXTTSChunkMinChars floors a chunk's length: a chapter is split because
	// XTTS degrades on long inputs.
	SettingXTTSChunkMinChars SettingKey = "xtts.chunk.min_chars"
	// SettingKokoroURL is the Kokoro-FastAPI server root. The root only: the
	// endpoints are appended.
	SettingKokoroURL SettingKey = "kokoro.url"
	// SettingKokoroKey is the bearer token, empty for a server with auth off.
	SettingKokoroKey SettingKey = "kokoro.key"
	// SettingKokoroModel names which of the server's model ids to ask for.
	SettingKokoroModel SettingKey = "kokoro.model"
	// SettingKokoroVoice names a voice the server offers, e.g. af_heart. Unlike
	// the XTTS backend's, it may not be empty: the voice's prefix is what selects
	// the language, and the server answers a blank one with a crash.
	SettingKokoroVoice SettingKey = "kokoro.voice"
	// SettingKokoroSpeed is the playback rate asked of the server; 1.0 is
	// unmodified.
	SettingKokoroSpeed SettingKey = "kokoro.speed"

	// SettingXTTSChunkSilenceMillis pads the joins so a sentence boundary is not
	// an audible splice.
	SettingXTTSChunkSilenceMillis SettingKey = "xtts.chunk.silence_ms"

	// SettingRunwareKey authenticates against the Runware API. There is no
	// anonymous access, so an empty one leaves those backends unavailable.
	SettingRunwareKey SettingKey = "runware.key"
	// SettingRunwareModel is the checkpoint the Runware backend draws with, as an
	// AIR identifier. It draws the thumbnail icons too when pointed at that port.
	SettingRunwareModel SettingKey = "runware.model"
	// SettingRunwareWidth and SettingRunwareHeight size slides: the dimensions are
	// a property of the backend drawing them, not of the chapter asking.
	SettingRunwareWidth  SettingKey = "runware.width"
	SettingRunwareHeight SettingKey = "runware.height"

	// SettingThumbnailIconStyle is appended to every icon prompt, so restyling the
	// grid re-runs cheap generations instead of re-rolling the captions.
	SettingThumbnailIconStyle SettingKey = "thumbnail.icon.style"
	// SettingThumbnailIconSize is the square edge each icon is generated at.
	SettingThumbnailIconSize SettingKey = "thumbnail.icon.size"
	// SettingThumbnailFont names the typeface, as a filename under the resources
	// fonts directory. Untagged: the HTML backend still to come sets type too.
	SettingThumbnailFont SettingKey = "thumbnail.font"
	// SettingThumbnailGridRows is how many rows the icons are laid out in;
	// columns follow from the cell count.
	SettingThumbnailGridRows SettingKey = "thumbnail.grid.rows"

	// Read once at creation and then frozen into the video row.
	SettingVideoDefaultChapters SettingKey = "video.default_chapter_count"
	SettingVideoDefaultSlides   SettingKey = "video.default_slides_per_chapter"
	// SettingVideoDefaultThumbnailCells seeds a new video's grid width, which it
	// keeps: the DAG holds one icon task per cell from expansion onward.
	SettingVideoDefaultThumbnailCells SettingKey = "video.default_thumbnail_cells"

	SettingTaskMaxAttempts     SettingKey = "task.max_attempts"
	SettingTaskRetryBaseMillis SettingKey = "task.retry_base_ms"
	SettingTaskRetryMaxMillis  SettingKey = "task.retry_max_ms"

	SettingSSECoalesceMillis SettingKey = "sse.coalesce_ms"
	SettingLogLevel          SettingKey = "log.level"
)

// SettingType is the declared type of a setting's text value. Every value is
// stored as text; an unparsable one fails at startup.
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

// SettingSuggestion is a known-good value with the name a human uses for it:
// `runware:100@1` and "FLUX.1 Dev" are not derivable from each other.
type SettingSuggestion struct {
	Value string
	Label string
}

// Setting is a single runtime configuration value keyed by a stable key.
type Setting struct {
	Key         SettingKey
	Value       string
	Type        SettingType
	Group       string
	Description string
	// Min and Max bound numeric settings. float64 so one pair serves both int and
	// float keys — a speed of 0.5..2.0 has no integer expression.
	Min float64
	Max float64
	// Options constrains the value to a fixed set; empty means unconstrained. Not
	// persisted — which backends exist is a property of the binary, so it is
	// stamped at load.
	Options []string
	// Optional allows a string setting to be empty, where empty is meaningful
	// rather than missing.
	Optional bool
	// Secret marks a value that may be written but never read back: the API sends
	// an empty string and a "configured" flag in its place. A credential that
	// round-trips through the client is a credential in every cache and
	// screenshot, and nothing needs to read one except the backend using it.
	// Not persisted — which rows are secret is a property of the binary.
	Secret bool
	// Suggestions are advisory where Options are binding: a checkpoint catalogue
	// lives on someone else's server, and a stale copy here would refuse an
	// identifier the API would have drawn. Not persisted.
	Suggestions []SettingSuggestion
	// Backend names the registry entry that reads this row, empty when the row is
	// not backend-specific, so the screen can dim a row whose backend is idle.
	// Not persisted.
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

// Validate parses the value against the declared type and bounds, on every
// write and on the whole table at startup.
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
		// NaN fails every comparison, so it slips past a bounds check written
		// as "outside the range".
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

// DefaultSettings is the complete seeded settings table; the seed upserts by
// key. This slice's order is the order the settings screen shows, because
// "voice, language, speed" is how narration is thought about and alphabetical
// is not.
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
		{Key: SettingNineRouterURL, Value: "http://127.0.0.1:20128", Type: SettingTypeString, Group: GroupWriting, Backend: BackendNineRouter, Description: "Gateway root, e.g. http://127.0.0.1:20128. The root only — /v1/chat/completions is appended."},
		//nolint:lll // one row, one line
		{Key: SettingNineRouterKey, Value: "", Type: SettingTypeString, Group: GroupWriting, Backend: BackendNineRouter, Optional: true, Secret: true, Description: "Bearer token for the gateway. Empty is usual: a gateway running locally with auth off needs none."},
		//nolint:lll // one row, one line
		{Key: SettingNineRouterModel, Value: "ag/gemini-3-flash", Type: SettingTypeString, Group: GroupWriting, Backend: BackendNineRouter, Description: "Which upstream the 9router backend routes to, e.g. ag/gemini-3-flash. See GET /v1/models on the gateway."},
		//nolint:lll // one row, one line
		{Key: SettingBlueprintChapterTolerancePercent, Value: "20", Type: SettingTypeInt, Group: GroupWriting, Min: 0, Max: 100, Description: "How far an accepted blueprint's chapter count may fall from the target, as a percentage. A roll outside it is rejected and written again."},

		//nolint:lll // one row, one line
		{Key: SettingXTTSURL, Value: "http://127.0.0.1:7851", Type: SettingTypeString, Group: GroupNarration, Backend: BackendXTTS, Description: "AllTalk server root, e.g. http://127.0.0.1:7851. The root only — not /api/tts-generate, which is appended."},
		//nolint:lll // one row, one line
		{Key: SettingXTTSVoice, Value: "", Type: SettingTypeString, Group: GroupNarration, Backend: BackendXTTS, Optional: true, Description: "Voice file on the AllTalk server, e.g. female_01.wav. Empty lets the server pick its own default."},
		//nolint:lll // one row, one line
		{Key: SettingXTTSLanguage, Value: "en", Type: SettingTypeString, Group: GroupNarration, Backend: BackendXTTS, Description: "Two-letter code the narration model is asked to speak."},
		//nolint:lll // one row, one line
		{Key: SettingXTTSSpeed, Value: "1.0", Type: SettingTypeFloat, Group: GroupNarration, Backend: BackendXTTS, Min: 0.5, Max: 2.0, Description: "Playback rate asked of the narration server; 1.0 is unmodified."},
		//nolint:lll // one row, one line
		{Key: SettingXTTSChunkMinChars, Value: "250", Type: SettingTypeInt, Group: GroupNarration, Backend: BackendXTTS, Min: 50, Max: 5000, Description: "Floor on a narration chunk's length in characters. A chapter is synthesised in pieces at least this long, because XTTS degrades on long inputs."},
		//nolint:lll // one row, one line
		{Key: SettingXTTSChunkSilenceMillis, Value: "200", Type: SettingTypeInt, Group: GroupNarration, Backend: BackendXTTS, Min: 0, Max: 2000, Description: "Pause inserted between narration chunks when they are rejoined, so a sentence boundary is not an audible splice."},

		//nolint:lll // one row, one line
		{Key: SettingKokoroURL, Value: "http://127.0.0.1:8880", Type: SettingTypeString, Group: GroupNarration, Backend: BackendKokoro, Description: "Kokoro-FastAPI server root, e.g. http://127.0.0.1:8880. The root only — not /v1, which is appended."},
		//nolint:lll // one row, one line
		{Key: SettingKokoroKey, Value: "", Type: SettingTypeString, Group: GroupNarration, Backend: BackendKokoro, Optional: true, Secret: true, Description: "Bearer token for the narration server. Empty is usual: one running locally with auth off needs none."},
		//nolint:lll // one row, one line
		{Key: SettingKokoroModel, Value: "kokoro", Type: SettingTypeString, Group: GroupNarration, Backend: BackendKokoro, Description: "Which of the server's model ids to ask for, e.g. kokoro. See GET /v1/models on the server."},
		//nolint:lll // one row, one line
		{Key: SettingKokoroVoice, Value: "af_heart", Type: SettingTypeString, Group: GroupNarration, Backend: BackendKokoro, Description: "Voice the server speaks as, e.g. af_heart. See GET /v1/audio/voices. The prefix picks the language — af_ and am_ are American English, bf_ British, jf_ Japanese, zf_ Mandarin — so there is no language setting beside this one."},
		//nolint:lll // one row, one line
		{Key: SettingKokoroSpeed, Value: "1.0", Type: SettingTypeFloat, Group: GroupNarration, Backend: BackendKokoro, Min: 0.5, Max: 2.0, Description: "Playback rate asked of the narration server; 1.0 is unmodified. The server accepts up to 4.0, which no narration wants."},

		//nolint:lll // one row, one line
		{Key: SettingRunwareKey, Value: "", Type: SettingTypeString, Group: GroupSlides, Backend: BackendRunware, Optional: true, Secret: true, Description: "API key from my.runware.ai/keys. There is no anonymous access: without it the Runware slide and icon backends are unavailable."},
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

// settingOrder is each key's position in DefaultSettings, built once because
// DefaultSettings rebuilds its slice on every call.
var settingOrder = func() map[SettingKey]int {
	defaults := DefaultSettings()
	order := make(map[SettingKey]int, len(defaults))
	for i, d := range defaults {
		order[d.Key] = i
	}
	return order
}()

// SettingOrder returns a key's seeded position. An unseeded key sorts last
// rather than being dropped — an unknown row is worth showing.
func SettingOrder(k SettingKey) int {
	if i, ok := settingOrder[k]; ok {
		return i
	}
	return len(settingOrder)
}
