package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
	"github.com/tbui/yt-studio/domain/service"
)

// settingsStore is the settings table in a map, so a preset can be applied
// without a database. It records the order of writes, which is what the
// all-or-nothing guarantee is asserted against.
type settingsStore struct {
	rows   map[entity.SettingKey]entity.Setting
	writes []entity.SettingKey
}

var (
	_ repository.SettingReader = (*settingsStore)(nil)
	_ repository.SettingWriter = (*settingsStore)(nil)
)

func newSettingsStore() *settingsStore {
	rows := make(map[entity.SettingKey]entity.Setting)
	for _, d := range entity.DefaultSettings() {
		rows[d.Key] = d
	}
	return &settingsStore{rows: rows}
}

func (s *settingsStore) SettingByKey(_ context.Context, key entity.SettingKey) (entity.Setting, error) {
	row, ok := s.rows[key]
	if !ok {
		return entity.Setting{}, entity.ErrSettingNotFound
	}
	return row, nil
}

func (s *settingsStore) ListSettings(context.Context) ([]entity.Setting, error) {
	out := make([]entity.Setting, 0, len(s.rows))
	for _, row := range s.rows {
		out = append(out, row)
	}
	return out, nil
}

func (s *settingsStore) UpdateSetting(
	_ context.Context,
	key entity.SettingKey,
	value string,
) (entity.Setting, error) {
	row, ok := s.rows[key]
	if !ok {
		return entity.Setting{}, entity.ErrSettingNotFound
	}
	row.Value = value
	row.UpdatedAt = time.Unix(0, 0)
	s.rows[key] = row
	s.writes = append(s.writes, key)
	return row, nil
}

func (s *settingsStore) UpsertSettings(context.Context, []entity.Setting) error { return nil }

// registered is what main.go's registry reports: the backends this build has.
func registered() map[entity.SettingKey][]string {
	return map[entity.SettingKey][]string{
		entity.SettingProviderLLM:           {"9router", "mock"},
		entity.SettingProviderTTS:           {"mock", "sample", "xtts"},
		entity.SettingProviderSlide:         {"mock", "runware", "sample"},
		entity.SettingProviderComposer:      {"ffmpeg", "mock"},
		entity.SettingProviderThumbnail:     {"builtin", "mock"},
		entity.SettingProviderThumbnailIcon: {"mock", "runware", "sample"},
		entity.SettingProviderUploader:      {"mock"},
	}
}

func loadedSettings(t *testing.T, store *settingsStore, options map[entity.SettingKey][]string) *service.Settings {
	t.Helper()
	settings := service.NewSettings(store, store)
	settings.Constrain(options)
	if err := settings.Load(context.Background()); err != nil {
		t.Fatalf("settings.Load: %v", err)
	}
	return settings
}

// The check main.go runs at startup: every built-in preset names a backend that
// this build actually registered.
func TestCheckPresetsAcceptsEveryBuiltin(t *testing.T) {
	t.Parallel()

	settings := loadedSettings(t, newSettingsStore(), registered())
	if err := app.CheckPresets(settings); err != nil {
		t.Fatalf("CheckPresets: %v", err)
	}
}

// And the reason it runs there: a backend that goes away must fail the boot,
// not a click.
func TestCheckPresetsRejectsAnUnregisteredBackend(t *testing.T) {
	t.Parallel()

	options := registered()
	options[entity.SettingProviderSlide] = []string{"mock", "sample"} // runware, removed
	settings := loadedSettings(t, newSettingsStore(), options)

	err := app.CheckPresets(settings)
	if !errors.Is(err, app.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}

func TestApplyPresetWritesEveryRowItNames(t *testing.T) {
	t.Parallel()

	store := newSettingsStore()
	settings := loadedSettings(t, store, registered())

	changed, err := app.ApplyPreset(context.Background(), settings, nil, nil, nil, "sample")
	if err != nil {
		t.Fatalf("ApplyPreset: %v", err)
	}
	// The table seeds every provider at mock, so sample moves all but the two it
	// leaves there.
	if len(changed) != 5 {
		t.Fatalf("changed %d rows, want 5: %v", len(changed), changed)
	}
	for _, want := range [][2]string{
		{"provider.tts", "sample"},
		{"provider.slide", "sample"},
		{"provider.composer", "ffmpeg"},
		{"provider.thumbnail", "builtin"},
		{"provider.thumbnail_icon", "sample"},
		{"provider.llm", "mock"},
		{"provider.uploader", "mock"},
	} {
		if got := settings.String(entity.SettingKey(want[0])); got != want[1] {
			t.Errorf("%s = %q, want %q", want[0], got, want[1])
		}
	}
}

// Re-applying the preset already in force is a no-op, not seven rewrites: an
// updatedAt that churns says something changed when nothing did.
func TestApplyPresetSkipsRowsAlreadyAtTheTarget(t *testing.T) {
	t.Parallel()

	store := newSettingsStore()
	settings := loadedSettings(t, store, registered())

	if _, err := app.ApplyPreset(context.Background(), settings, nil, nil, nil, "live"); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	first := len(store.writes)

	changed, err := app.ApplyPreset(context.Background(), settings, nil, nil, nil, "live")
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if len(changed) != 0 {
		t.Errorf("second apply changed %d rows, want none", len(changed))
	}
	if len(store.writes) != first {
		t.Errorf("second apply wrote %d more rows, want none", len(store.writes)-first)
	}
}

// The guarantee that makes a preset safe to click: an illegal value anywhere in
// the patch means nothing is written, rather than the pipeline ending up half on
// one set of backends and half on another.
func TestApplyPresetWritesNothingWhenOneValueIsIllegal(t *testing.T) {
	t.Parallel()

	// The sample icon backend goes away. "mock" stays legal, so the seeded table
	// still loads and the preset is the only thing that breaks.
	options := registered()
	options[entity.SettingProviderThumbnailIcon] = []string{"mock", "runware"}
	store := newSettingsStore()
	settings := loadedSettings(t, store, options)

	// provider.thumbnail_icon is the sixth of the seven rows "sample" names, so
	// four earlier rows would already be written by a loop that validated as it
	// went — which is the state this test exists to prove unreachable.
	_, err := app.ApplyPreset(context.Background(), settings, nil, nil, nil, "sample")
	if !errors.Is(err, app.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
	if len(store.writes) != 0 {
		t.Fatalf("wrote %v, want nothing", store.writes)
	}
	if got := settings.String(entity.SettingProviderComposer); got != "mock" {
		t.Errorf("provider.composer = %q, want the untouched mock", got)
	}
}

func TestApplyPresetReportsAnUnknownName(t *testing.T) {
	t.Parallel()

	settings := loadedSettings(t, newSettingsStore(), registered())
	_, err := app.ApplyPreset(context.Background(), settings, nil, nil, nil, "nonesuch")
	if !errors.Is(err, entity.ErrPresetNotFound) {
		t.Fatalf("err = %v, want ErrPresetNotFound", err)
	}
}

// A preset that names a pool limit must resize the live semaphore, which is why
// ApplyPreset goes through UpdateSetting rather than writing rows directly.
func TestApplyPresetRunsTheLiveSideEffects(t *testing.T) {
	t.Parallel()

	store := newSettingsStore()
	settings := loadedSettings(t, store, registered())
	pools := &recordingPools{}

	// Built-in presets deliberately name only provider rows, so the side effect is
	// exercised through the same path a pool row would take.
	if _, err := app.UpdateSetting(context.Background(), settings, pools, nil, nil,
		entity.SettingPoolImageLimit, "6"); err != nil {
		t.Fatalf("UpdateSetting: %v", err)
	}
	if len(pools.limits) != 1 || pools.limits[0] != 6 {
		t.Fatalf("pool limits = %v, want one resize to 6", pools.limits)
	}
}

type recordingPools struct{ limits []int }

var _ app.PoolLimiter = (*recordingPools)(nil)

func (p *recordingPools) SetPoolLimit(_ context.Context, _ entity.Pool, limit int) error {
	p.limits = append(p.limits, limit)
	return nil
}
