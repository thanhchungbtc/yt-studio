package services

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/tbui/yt-studio/internal/app"
	"github.com/tbui/yt-studio/internal/domain/entity"
	"github.com/tbui/yt-studio/internal/domain/provider"
	"github.com/tbui/yt-studio/internal/domain/repository"
	"github.com/tbui/yt-studio/internal/domain/scheduler"
)

// Error kinds, as the UI receives them in a failed call's cause.
const (
	KindCancelled   = "cancelled"
	KindNotFound    = "not_found"
	KindInvalid     = "invalid"
	KindConflict    = "conflict"
	KindUnavailable = "unavailable"
	KindInternal    = "internal"
)

// Problem is a failed call's cause: the kind of failure.
type Problem struct {
	Kind string `json:"kind"`
}

// MarshalError is the app's application.Options.MarshalError.
func MarshalError(err error) []byte {
	b, _ := json.Marshal(Problem{Kind: Kind(err)})
	return b
}

// Kind classifies a domain or use-case error.
func Kind(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return KindCancelled
	case errors.Is(err, repository.ErrNotFound),
		errors.Is(err, entity.ErrAssetNotFound),
		errors.Is(err, entity.ErrSettingNotFound),
		errors.Is(err, entity.ErrPresetNotFound),
		errors.Is(err, entity.ErrTaskNotFound),
		errors.Is(err, scheduler.ErrUnknownVideo),
		errors.Is(err, scheduler.ErrUnknownTask):
		return KindNotFound
	case errors.Is(err, app.ErrValidation),
		errors.Is(err, provider.ErrRejected),
		errors.Is(err, entity.ErrInvalidSlug),
		errors.Is(err, entity.ErrInvalidRef),
		errors.Is(err, entity.ErrInvalidSetting),
		errors.Is(err, entity.ErrInvalidPreset),
		errors.Is(err, entity.ErrInvalidChannel),
		errors.Is(err, entity.ErrInvalidVideo),
		errors.Is(err, entity.ErrInvalidChapter),
		errors.Is(err, scheduler.ErrInvalidGraph),
		errors.Is(err, scheduler.ErrPoolLimitOutOfRange),
		errors.Is(err, scheduler.ErrUnknownPool):
		return KindInvalid
	case errors.Is(err, app.ErrConflict),
		errors.Is(err, repository.ErrConflict),
		errors.Is(err, provider.ErrUnavailable),
		errors.Is(err, scheduler.ErrNotGated),
		errors.Is(err, scheduler.ErrBlueprintLocked),
		errors.Is(err, scheduler.ErrAlreadyExpanded):
		return KindConflict
	case errors.Is(err, scheduler.ErrSchedulerClosed):
		return KindUnavailable
	default:
		return KindInternal
	}
}
