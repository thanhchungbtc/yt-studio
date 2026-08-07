// Package app holds the use cases: one exported function per file, named after
// what it does, so an HTTP handler and a CLI command call the same function.
// Each declares the narrow interfaces it uses as separate parameters — there is
// no dependency container, the signature is the dependency list.
package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// ErrValidation is the sentinel every input rejection wraps, so delivery can
// map it to 400 without knowing which field was wrong.
var ErrValidation = errors.New("validation failed")

// ErrConflict is returned when an operation is not valid in the current state.
var ErrConflict = errors.New("conflict")

// ErrBlueprintOffTarget reports an outline whose chapter count fell outside the
// tolerance band. Not an ErrValidation: the input was fine, the roll was not,
// so it is worth another attempt.
var ErrBlueprintOffTarget = errors.New("blueprint chapter count is off target")

// ErrThumbnailPlanOffTarget reports a grid the model came back short on. Not an
// ErrValidation for the same reason: the graph cannot grow to fit a short plan,
// so it is a roll to take again.
var ErrThumbnailPlanOffTarget = errors.New("thumbnail plan does not fill the grid")

// Invalid builds a validation error for one field.
func Invalid(field, message string) error {
	return fmt.Errorf("%w: %s %s", ErrValidation, field, message)
}

// classify turns a provider or repository error into a task outcome. The only
// question is whether another attempt could land differently.
func classify(err error) entity.TaskOutcome {
	switch {
	case err == nil:
		return entity.Success{}
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		// Cancelled or shutting down: resume happens through the task table.
		return entity.Failed{Err: err, Retryable: false}
	case errors.Is(err, provider.ErrUnavailable):
		// Nothing runs until the operator changes something.
		return entity.Failed{Err: err, Retryable: false}
	case errors.Is(err, repository.ErrNotFound),
		errors.Is(err, entity.ErrAssetNotFound),
		errors.Is(err, ErrValidation),
		errors.Is(err, entity.ErrInvalidChapter),
		errors.Is(err, entity.ErrInvalidVideo),
		errors.Is(err, entity.ErrInvalidAsset):
		return entity.Failed{Err: err, Retryable: false}
	default:
		return entity.Failed{Err: err, Retryable: true}
	}
}
