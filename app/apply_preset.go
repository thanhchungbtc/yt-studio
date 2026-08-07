package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/service"
)

// ApplyPreset writes every row a preset names and returns the ones that
// changed. Two things make it more than a loop.
//
// It validates the whole patch first, because the writes are separate
// statements with side effects that cannot be rolled back: an illegal value
// found at the fifth row would otherwise leave the pipeline half on one set of
// backends. Validation is pure, so proving it up front costs nothing.
//
// And it goes through UpdateSetting rather than settings.Set, so a preset
// naming a pool limit resizes the semaphore and one naming the log level
// applies it, instead of taking effect at the next restart.
//
// A row already holding the value is skipped, so re-applying the preset in
// force does not churn every updatedAt.
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
