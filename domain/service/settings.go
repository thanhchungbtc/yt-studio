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

// Settings is the typed, cached accessor over the settings table.
//
// The whole table is read and validated once at startup, so an unparsable value
// fails loudly then rather than at first use, and every typed getter afterwards
// is a map read that cannot fail. Writes go through Set, which validates,
// persists and refreshes the cache — a change applies without a restart.
type Settings struct {
	reader repository.SettingReader
	writer repository.SettingWriter

	// options constrains the keys whose legal values are only known once the
	// backends have been registered. It is written once, before Load, and read
	// only afterwards.
	options map[entity.SettingKey][]string

	// optional carries the keys an empty value is legal for. Like options, it is
	// a property of the code rather than of the database, so it is stamped onto
	// each row at load rather than stored.
	optional map[entity.SettingKey]bool

	// backend carries which registry entry reads a row, for the rows only one
	// does. Stamped rather than stored for the same reason as the two above: what
	// reads a row is a fact about this binary, and a persisted copy would go
	// stale the first time a backend was rewritten.
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

// Constrain declares the legal values for keys that have a fixed set, which is
// what turns a free-text field into a dropdown and a typo into a startup error
// rather than a silent fallback. It must be called before Load.
func (s *Settings) Constrain(options map[entity.SettingKey][]string) {
	s.options = options
}

// constrain stamps a row with its legal values, so validation and the settings
// screen both see them without the database having to store them.
func (s *Settings) constrain(row entity.Setting) entity.Setting {
	row.Options = s.options[row.Key]
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
	// Every key the code reads must exist, or a typed getter would silently fall
	// back. Missing keys are a seeding bug and are caught here.
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

// All returns every setting in seeded order — grouped, and within a group in
// the order the rows were written — for the settings screen.
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

// Int returns an integer setting, falling back to the seeded default if the key
// is somehow absent. Load has already proved every key is present.
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

// Check reports whether a value would be accepted for a key, without writing it.
//
// It exists for the writes that come in a batch: a preset names half a dozen
// rows, and applying four of them before the fifth turns out to be illegal
// leaves the pipeline half on one set of backends and half on another. Because
// validation is pure, the whole patch can be proved first and only then written.
func (s *Settings) Check(key entity.SettingKey, value string) error {
	_, err := s.candidate(key, value)
	return err
}

// candidate builds the row a write would produce and validates it.
//
// The constraint is checked here rather than in the store, because the store
// reads its row from the database and the legal values are not in there.
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

// sortSettings puts the table in seeded order — which is grouped, and within a
// group is the order the rows were written rather than the order their keys
// sort in. Two unseeded rows fall back to their keys so the result is stable.
func sortSettings(v []entity.Setting) {
	// Insertion sort: the table is a few dozen rows and this avoids pulling in a
	// comparator closure allocation on a path called by the settings screen.
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
