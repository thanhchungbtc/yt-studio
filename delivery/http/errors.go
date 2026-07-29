package http

import (
	"context"
	"errors"

	"github.com/danielgtaylor/huma/v2"

	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
	"github.com/tbui/yt-studio/domain/scheduler"
)

// mapError translates a domain or use-case error into an HTTP status.
//
// Delivery knows nothing about why an operation failed beyond these sentinels,
// which is what keeps handlers free of business logic (§7).
func mapError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return huma.NewError(499, "request cancelled", err)
	case errors.Is(err, repository.ErrNotFound),
		errors.Is(err, entity.ErrAssetNotFound),
		errors.Is(err, entity.ErrSettingNotFound),
		errors.Is(err, entity.ErrTaskNotFound),
		errors.Is(err, scheduler.ErrUnknownVideo),
		errors.Is(err, scheduler.ErrUnknownTask):
		return huma.Error404NotFound(err.Error())
	case errors.Is(err, app.ErrValidation),
		errors.Is(err, entity.ErrInvalidSlug),
		errors.Is(err, entity.ErrInvalidRef),
		errors.Is(err, entity.ErrInvalidSetting),
		errors.Is(err, entity.ErrInvalidChannel),
		errors.Is(err, entity.ErrInvalidVideo),
		errors.Is(err, entity.ErrInvalidChapter),
		errors.Is(err, scheduler.ErrInvalidGraph),
		errors.Is(err, scheduler.ErrPoolLimitOutOfRange),
		errors.Is(err, scheduler.ErrUnknownPool):
		return huma.Error422UnprocessableEntity(err.Error())
	case errors.Is(err, app.ErrConflict),
		errors.Is(err, repository.ErrConflict),
		errors.Is(err, scheduler.ErrNotGated):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, scheduler.ErrSchedulerClosed):
		return huma.Error503ServiceUnavailable("scheduler is not running")
	default:
		return huma.Error500InternalServerError(err.Error())
	}
}
