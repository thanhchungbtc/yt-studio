package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/service"
)

// UpdateSetting writes one settings row and applies its side effects live.
//
// Changing a pool limit, the SSE coalescing window or the log level is a row
// update applied without restarting the daemon. Each side effect is dispatched
// through a narrow port that is named in the signature, so it is obvious from
// here what a settings edit can reach.
func UpdateSetting(
	ctx context.Context,
	settings *service.Settings,
	pools PoolLimiter,
	coalesce CoalesceSetter,
	level *slog.LevelVar,
	key entity.SettingKey,
	value string,
) (entity.Setting, error) {
	updated, err := settings.Set(ctx, key, value)
	if err != nil {
		return entity.Setting{}, fmt.Errorf("%w: %w", ErrValidation, err)
	}

	for _, pool := range entity.AllPools {
		if entity.PoolLimitKey(pool) != key {
			continue
		}
		limit, err := updated.Int()
		if err != nil {
			return entity.Setting{}, fmt.Errorf("%w: %w", ErrValidation, err)
		}
		if pools != nil {
			if err := pools.SetPoolLimit(ctx, pool, limit); err != nil {
				return entity.Setting{}, err
			}
		}
		return updated, nil
	}

	// Only the keys with a live side effect appear here; everything else is
	// already applied by virtue of being read from the cache on next use.
	switch key { //nolint:exhaustive // side effects are the exception, not the rule
	case entity.SettingSSECoalesceMillis:
		if coalesce != nil {
			ms, err := updated.Int()
			if err != nil {
				return entity.Setting{}, fmt.Errorf("%w: %w", ErrValidation, err)
			}
			coalesce.SetCoalesce(time.Duration(ms) * time.Millisecond)
		}
	case entity.SettingLogLevel:
		if level != nil {
			parsed, err := ParseLogLevel(updated.Value)
			if err != nil {
				return entity.Setting{}, err
			}
			level.Set(parsed)
		}
	}
	return updated, nil
}

// ParseLogLevel maps a settings value to a slog level.
func ParseLogLevel(v string) (slog.Level, error) {
	switch v {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, Invalid("log.level", "must be debug, info, warn or error")
	}
}
