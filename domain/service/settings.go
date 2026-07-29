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

// Settings is the typed, cached accessor over the settings table (§3).
//
// The whole table is read and validated once at startup, so an unparsable value
// fails loudly then rather than at first use, and every typed getter afterwards
// is a map read that cannot fail. Writes go through Set, which validates,
// persists and refreshes the cache — a change applies without a restart.
type Settings struct {
	reader repository.SettingReader
	writer repository.SettingWriter

	mu    sync.RWMutex
	cache map[entity.SettingKey]entity.Setting
}

// NewSettings wires the service to both halves of the settings port.
func NewSettings(reader repository.SettingReader, writer repository.SettingWriter) *Settings {
	return &Settings{
		reader: reader,
		writer: writer,
		cache:  make(map[entity.SettingKey]entity.Setting, len(entity.DefaultSettings())),
	}
}

// Load reads and validates the whole table. Any invalid row is a startup error.
func (s *Settings) Load(ctx context.Context) error {
	rows, err := s.reader.ListSettings(ctx)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	next := make(map[entity.SettingKey]entity.Setting, len(rows))
	for _, r := range rows {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("settings table is invalid: %w", err)
		}
		next[r.Key] = r
	}
	// Every key the code reads must exist, or a typed getter would silently
	// fall back. Missing keys are a seeding bug and are caught here.
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

// All returns every setting, ordered by group then key, for the settings screen.
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

// GateEnabled reports whether a gate is active (§6).
func (s *Settings) GateEnabled(g entity.GateKind) bool {
	key := entity.GateEnabledKey(g)
	if key == "" {
		return false
	}
	return s.Bool(key)
}

// Set validates, persists and caches a new value.
func (s *Settings) Set(ctx context.Context, key entity.SettingKey, value string) (entity.Setting, error) {
	updated, err := s.writer.UpdateSetting(ctx, key, value)
	if err != nil {
		return entity.Setting{}, err
	}
	s.mu.Lock()
	s.cache[key] = updated
	s.mu.Unlock()
	return updated, nil
}

func sortSettings(v []entity.Setting) {
	// Insertion sort: the table is a few dozen rows and this avoids pulling in
	// a comparator closure allocation on a path called by the settings screen.
	for i := 1; i < len(v); i++ {
		cur := v[i]
		j := i - 1
		for j >= 0 && (v[j].Group > cur.Group || (v[j].Group == cur.Group && v[j].Key > cur.Key)) {
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
