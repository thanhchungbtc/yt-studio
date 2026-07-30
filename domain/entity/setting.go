package entity

import (
	"errors"
	"fmt"
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

// The complete set of settings keys. Everything the daemon needs after the
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

	SettingProviderLLM      SettingKey = "provider.llm"
	SettingProviderTTS      SettingKey = "provider.tts"
	SettingProviderImage    SettingKey = "provider.image"
	SettingProviderComposer SettingKey = "provider.composer"
	SettingProviderUploader SettingKey = "provider.uploader"

	SettingVideoDefaultChapters SettingKey = "video.default_chapter_count"
	SettingVideoDefaultImages   SettingKey = "video.default_images_per_chapter"
	// SettingVideoChapterTolerancePercent bounds how far an accepted blueprint's
	// chapter count may fall from the one the video was briefed with.
	SettingVideoChapterTolerancePercent SettingKey = "video.chapter_tolerance_percent"

	SettingTaskMaxAttempts     SettingKey = "task.max_attempts"
	SettingTaskRetryBaseMillis SettingKey = "task.retry_base_ms"
	SettingTaskRetryMaxMillis  SettingKey = "task.retry_max_ms"

	SettingSSECoalesceMillis SettingKey = "sse.coalesce_ms"
	SettingLogLevel          SettingKey = "log.level"
	SettingUploadDryRun      SettingKey = "upload.dry_run"

	// SettingMockLatencyMillis scales the mock providers' simulated work so the
	// scheduler can be exercised at realistic pacing without a GPU.
	SettingMockLatencyMillis SettingKey = "mock.latency_ms"
	// SettingMockFailureRatePercent injects transient provider failures so the
	// retry path is exercised end to end.
	SettingMockFailureRatePercent SettingKey = "mock.failure_rate_percent"
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
)

// Valid reports whether the type is one of the known constants.
func (t SettingType) Valid() bool {
	switch t {
	case SettingTypeInt, SettingTypeBool, SettingTypeString:
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
	// Min and Max bound integer settings; they are advisory to the UI and enforced
	// by Validate.
	Min int
	Max int
	// Options constrains the value to a fixed set. It is deliberately not
	// persisted: which backends exist is a property of the running binary, not of
	// the database, so it is supplied at load time by whoever registered them. An
	// empty Options means the value is unconstrained.
	Options   []string
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
		if s.Min != s.Max && (n < s.Min || n > s.Max) {
			return fmt.Errorf("%w %q: %d is outside %d..%d", ErrInvalidSetting, s.Key, n, s.Min, s.Max)
		}
	case SettingTypeBool:
		if _, err := strconv.ParseBool(s.Value); err != nil {
			return fmt.Errorf("%w %q: %q is not a boolean", ErrInvalidSetting, s.Key, s.Value)
		}
	case SettingTypeString:
		if s.Value == "" {
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
func DefaultSettings() []Setting {
	return []Setting{
		{Key: SettingPoolLLMLimit, Value: "2", Type: SettingTypeInt, Group: "pools", Min: 1, Max: MaxPoolLimit, Description: "Concurrent LLM calls across all videos and channels."},
		{Key: SettingPoolTTSLimit, Value: "2", Type: SettingTypeInt, Group: "pools", Min: 1, Max: MaxPoolLimit, Description: "Concurrent narration syntheses."},
		{Key: SettingPoolImageLimit, Value: "2", Type: SettingTypeInt, Group: "pools", Min: 1, Max: MaxPoolLimit, Description: "Concurrent still generations — usually the binding constraint."},
		{Key: SettingPoolComposeLimit, Value: "2", Type: SettingTypeInt, Group: "pools", Min: 1, Max: MaxPoolLimit, Description: "Concurrent clip and concat compositions."},
		{Key: SettingPoolCacheLimit, Value: "32", Type: SettingTypeInt, Group: "pools", Min: 1, Max: MaxPoolLimit, Description: "Concurrent image-prompt cache reads; must never be the bottleneck."},
		{Key: SettingPoolUploadLimit, Value: "1", Type: SettingTypeInt, Group: "pools", Min: 1, Max: MaxPoolLimit, Description: "Concurrent uploads."},

		{Key: SettingGateBlueprintEnabled, Value: "true", Type: SettingTypeBool, Group: "gates", Description: "Pause after the blueprint for human review."},
		{Key: SettingGateUploadEnabled, Value: "true", Type: SettingTypeBool, Group: "gates", Description: "Pause before upload for human review."},

		{Key: SettingProviderLLM, Value: "mock", Type: SettingTypeString, Group: "providers", Description: "Backend for blueprint, script, prompts and metadata."},
		{Key: SettingProviderTTS, Value: "mock", Type: SettingTypeString, Group: "providers", Description: "Backend for narration."},
		{Key: SettingProviderImage, Value: "mock", Type: SettingTypeString, Group: "providers", Description: "Backend for stills."},
		{Key: SettingProviderComposer, Value: "mock", Type: SettingTypeString, Group: "providers", Description: "Backend for clip and concat composition."},
		{Key: SettingProviderUploader, Value: "mock", Type: SettingTypeString, Group: "providers", Description: "Backend for publishing."},

		{Key: SettingVideoDefaultChapters, Value: "50", Type: SettingTypeInt, Group: "video", Min: MinChapterCount, Max: MaxChapterCount, Description: "Chapters created for a new video when unspecified."},
		{Key: SettingVideoDefaultImages, Value: "2", Type: SettingTypeInt, Group: "video", Min: MinImagesPerChapter, Max: MaxImagesPerChapter, Description: "Stills generated per chapter when unspecified."},
		{Key: SettingVideoChapterTolerancePercent, Value: "20", Type: SettingTypeInt, Group: "video", Min: 0, Max: 100, Description: "How far an accepted blueprint's chapter count may fall from the target, as a percentage."},

		{Key: SettingTaskMaxAttempts, Value: "3", Type: SettingTypeInt, Group: "scheduler", Min: 1, Max: 20, Description: "Attempts before a task is permanently failed."},
		{Key: SettingTaskRetryBaseMillis, Value: "250", Type: SettingTypeInt, Group: "scheduler", Min: 1, Max: 60000, Description: "Initial retry backoff."},
		{Key: SettingTaskRetryMaxMillis, Value: "30000", Type: SettingTypeInt, Group: "scheduler", Min: 1, Max: 3600000, Description: "Maximum retry backoff."},

		{Key: SettingSSECoalesceMillis, Value: "50", Type: SettingTypeInt, Group: "server", Min: 1, Max: 5000, Description: "Minimum interval between event batches per video."},
		{Key: SettingLogLevel, Value: "info", Type: SettingTypeString, Group: "server", Description: "debug, info, warn or error; applied without a restart."},
		{Key: SettingUploadDryRun, Value: "true", Type: SettingTypeBool, Group: "server", Description: "Uploads are simulated and produce a local receipt."},

		{Key: SettingMockLatencyMillis, Value: "40", Type: SettingTypeInt, Group: "mock", Min: 0, Max: 600000, Description: "Simulated provider work per unit, scaled per task kind."},
		{Key: SettingMockFailureRatePercent, Value: "0", Type: SettingTypeInt, Group: "mock", Min: 0, Max: 100, Description: "Injected transient failure rate, to exercise retries."},
	}
}
