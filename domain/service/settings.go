// Package service holds domain services: logic that spans entities but belongs
// to the core rather than to a use case.
package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// Settings is the typed, cached accessor over the settings table. The whole
// table is validated once at startup, so an unparsable value fails there rather
// than at first use and every typed getter afterwards cannot fail. Set
// validates, persists and refreshes, so an edit applies without a restart.
//
// The four maps below are properties of this binary rather than the database,
// so they are written once before Load and stamped onto each row at load.
type Settings struct {
	reader repository.SettingReader
	writer repository.SettingWriter

	// options constrains keys whose legal values are known only once the
	// backends have been registered.
	options map[entity.SettingKey][]string

	// optional carries the keys an empty value is legal for.
	optional map[entity.SettingKey]bool

	// suggestions carries the known-good values worth offering for a key.
	suggestions map[entity.SettingKey][]entity.SettingSuggestion

	// backend carries which registry entry reads a row, where only one does.
	backend map[entity.SettingKey]string

	mu    sync.RWMutex
	cache map[entity.SettingKey]entity.Setting
}

// NewSettings wires the service to both halves of the settings port.
func NewSettings(reader repository.SettingReader, writer repository.SettingWriter) *Settings {
	defaults := entity.DefaultSettings()
	optional := make(map[entity.SettingKey]bool, len(defaults))
	backend := make(map[entity.SettingKey]string, len(defaults))
	for _, d := range defaults {
		if d.Optional {
			optional[d.Key] = true
		}
		if d.Backend != "" {
			backend[d.Key] = d.Backend
		}
	}
	return &Settings{
		reader:   reader,
		writer:   writer,
		optional: optional,
		backend:  backend,
		cache:    make(map[entity.SettingKey]entity.Setting, len(defaults)),
	}
}

// Constrain declares the legal values for keys with a fixed set, turning a
// typo into a startup error rather than a silent fallback. Call before Load.
func (s *Settings) Constrain(options map[entity.SettingKey][]string) {
	s.options = options
}

// Suggest declares known-good values for keys whose legal set is too large to
// enumerate. Advisory only, and must be called before Load.
func (s *Settings) Suggest(suggestions map[entity.SettingKey][]entity.SettingSuggestion) {
	s.suggestions = suggestions
}

// constrain stamps a row with what the database does not store.
func (s *Settings) constrain(row entity.Setting) entity.Setting {
	row.Options = s.options[row.Key]
	row.Suggestions = s.suggestions[row.Key]
	row.Optional = s.optional[row.Key]
	row.Backend = s.backend[row.Key]
	return row
}

// Load reads and validates the whole table. Any invalid row is a startup error.
func (s *Settings) Load(ctx context.Context) error {
	rows, err := s.reader.ListSettings(ctx)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	next := make(map[entity.SettingKey]entity.Setting, len(rows))
	for _, r := range rows {
		r = s.constrain(r)
		if err := r.Validate(); err != nil {
			return fmt.Errorf("settings table is invalid: %w", err)
		}
		next[r.Key] = r
	}
	// A missing key would make a typed getter silently fall back, so a seeding
	// bug is caught here instead.
	for _, d := range entity.DefaultSettings() {
		if _, ok := next[d.Key]; !ok {
			return fmt.Errorf("%w: %q is missing; run the seed", entity.ErrSettingNotFound, d.Key)
		}
	}
	s.mu.Lock()
	s.cache = next
	s.mu.Unlock()
	return nil
}

// All returns every setting in seeded order, for the settings screen.
func (s *Settings) All() []entity.Setting {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]entity.Setting, 0, len(s.cache))
	for _, v := range s.cache {
		out = append(out, v)
	}
	sortSettings(out)
	return out
}

// Get returns one setting.
func (s *Settings) Get(key entity.SettingKey) (entity.Setting, error) {
	s.mu.RLock()
	v, ok := s.cache[key]
	s.mu.RUnlock()
	if !ok {
		return entity.Setting{}, fmt.Errorf("%w: %q", entity.ErrSettingNotFound, key)
	}
	return v, nil
}

// Int returns an integer setting, falling back to the seeded default; Load has
// already proved every key is present.
func (s *Settings) Int(key entity.SettingKey) int {
	v, err := s.Get(key)
	if err != nil {
		return defaultInt(key)
	}
	n, err := v.Int()
	if err != nil {
		return defaultInt(key)
	}
	return n
}

// Float returns a floating-point setting.
func (s *Settings) Float(key entity.SettingKey) float64 {
	v, err := s.Get(key)
	if err != nil {
		return defaultFloat(key)
	}
	f, err := v.Float()
	if err != nil {
		return defaultFloat(key)
	}
	return f
}

// Bool returns a boolean setting.
func (s *Settings) Bool(key entity.SettingKey) bool {
	v, err := s.Get(key)
	if err != nil {
		return defaultBool(key)
	}
	b, err := v.Bool()
	if err != nil {
		return defaultBool(key)
	}
	return b
}

// String returns a string setting.
func (s *Settings) String(key entity.SettingKey) string {
	v, err := s.Get(key)
	if err != nil {
		return defaultString(key)
	}
	return v.Value
}

// Duration returns an integer-milliseconds setting as a duration.
func (s *Settings) Duration(key entity.SettingKey) time.Duration {
	return time.Duration(s.Int(key)) * time.Millisecond
}

// PoolLimits reads every pool's limit in one go, for scheduler construction.
func (s *Settings) PoolLimits() map[entity.Pool]int {
	out := make(map[entity.Pool]int, entity.NumPools)
	for _, p := range entity.AllPools {
		out[p] = s.Int(entity.PoolLimitKey(p))
	}
	return out
}

// GateEnabled reports whether a gate is active.
func (s *Settings) GateEnabled(g entity.GateKind) bool {
	key := entity.GateEnabledKey(g)
	if key == "" {
		return false
	}
	return s.Bool(key)
}

// Check reports whether a value would be accepted, without writing it. It is
// for batched writes: applying four rows of a preset before the fifth turns out
// illegal would leave the pipeline half on one set of backends.
func (s *Settings) Check(key entity.SettingKey, value string) error {
	_, err := s.candidate(key, value)
	return err
}

// candidate builds and validates the row a write would produce. It happens here
// rather than in the store, whose rows come from a database that holds no
// legal-value set.
func (s *Settings) candidate(key entity.SettingKey, value string) (entity.Setting, error) {
	current, err := s.Get(key)
	if err != nil {
		return entity.Setting{}, err
	}
	row := s.constrain(current)
	row.Value = value
	if err := row.Validate(); err != nil {
		return entity.Setting{}, err
	}
	return row, nil
}

// Set validates, persists and caches a new value.
func (s *Settings) Set(ctx context.Context, key entity.SettingKey, value string) (entity.Setting, error) {
	if _, err := s.candidate(key, value); err != nil {
		return entity.Setting{}, err
	}

	updated, err := s.writer.UpdateSetting(ctx, key, value)
	if err != nil {
		return entity.Setting{}, err
	}
	updated = s.constrain(updated)
	s.mu.Lock()
	s.cache[key] = updated
	s.mu.Unlock()
	return updated, nil
}

// sortSettings puts the table in seeded order, falling back to the key so two
// unseeded rows still sort stably. Insertion sort: a few dozen rows, and no
// comparator closure to allocate.
func sortSettings(v []entity.Setting) {
	for i := 1; i < len(v); i++ {
		cur := v[i]
		curOrder := entity.SettingOrder(cur.Key)
		j := i - 1
		for j >= 0 {
			order := entity.SettingOrder(v[j].Key)
			if order < curOrder || (order == curOrder && v[j].Key <= cur.Key) {
				break
			}
			v[j+1] = v[j]
			j--
		}
		v[j+1] = cur
	}
}

func defaultSetting(key entity.SettingKey) (entity.Setting, bool) {
	for _, d := range entity.DefaultSettings() {
		if d.Key == key {
			return d, true
		}
	}
	return entity.Setting{}, false
}

func defaultInt(key entity.SettingKey) int {
	d, ok := defaultSetting(key)
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(d.Value)
	if err != nil {
		return 0
	}
	return n
}

func defaultFloat(key entity.SettingKey) float64 {
	d, ok := defaultSetting(key)
	if !ok {
		return 0
	}
	f, err := strconv.ParseFloat(d.Value, 64)
	if err != nil {
		return 0
	}
	return f
}

func defaultBool(key entity.SettingKey) bool {
	d, ok := defaultSetting(key)
	if !ok {
		return false
	}
	b, err := strconv.ParseBool(d.Value)
	if err != nil {
		return false
	}
	return b
}

func defaultString(key entity.SettingKey) string {
	d, ok := defaultSetting(key)
	if !ok {
		return ""
	}
	return d.Value
}
