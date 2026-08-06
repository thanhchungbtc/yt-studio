package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/service"
)

// ApplyPreset writes every row a preset names and returns the ones that changed.
//
// Two things make this more than a loop.
//
// It validates the whole patch before writing any of it. The writes are separate
// statements — the settings table has no batch write, and the live side effects
// below could not be rolled back if it had one — so an illegal value discovered
// at the fifth row would otherwise leave the pipeline running half on one set of
// backends and half on another. Validation is pure, so proving the patch first
// costs nothing and makes that state unreachable.
//
// And it goes through UpdateSetting rather than settings.Set, so a preset that
// names a pool limit resizes the semaphore, and one that names the log level or
// the SSE window applies it. Writing the rows directly would leave the scheduler
// running on the old limits until the next restart — the exact failure this
// server's "a change applies to the next task" promise exists to avoid.
//
// A row already holding the value the preset would write is skipped rather than
// rewritten: re-applying the preset that is already in force should not churn
// every updatedAt and poke the pool limiter seven times.
func ApplyPreset(
	ctx context.Context,
	settings *service.Settings,
	pools PoolLimiter,
	coalesce CoalesceSetter,
	level *slog.LevelVar,
	name string,
) ([]entity.Setting, error) {
	preset, err := entity.PresetByName(name)
	if err != nil {
		return nil, err
	}
	if err := CheckPreset(settings, preset); err != nil {
		return nil, err
	}

	changed := make([]entity.Setting, 0, len(preset.Values))
	for _, v := range preset.Values {
		current, err := settings.Get(v.Key)
		if err != nil {
			return nil, err
		}
		if current.Value == v.Value {
			continue
		}
		updated, err := UpdateSetting(ctx, settings, pools, coalesce, level, v.Key, v.Value)
		if err != nil {
			return nil, fmt.Errorf("apply preset %q at %s: %w", preset.Name, v.Key, err)
		}
		changed = append(changed, updated)
	}
	return changed, nil
}
