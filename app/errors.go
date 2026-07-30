// Package app holds the use cases: one exported function per file, named after
// what it does. This is where the real logic lives, so delivery layers stay
// thin — an HTTP handler and a CLI command call the same function.
//
// Every function declares exactly the narrow interfaces it uses as separate
// parameters. There is no container struct of dependencies anywhere in this
// package: the signature is the whole dependency list.
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

// Invalid builds a validation error for one field.
func Invalid(field, message string) error {
	return fmt.Errorf("%w: %s %s", ErrValidation, field, message)
}

// classify turns an error from a provider or repository into a task outcome.
//
// The distinction that matters is transient versus permanent: a provider that
// timed out should be retried with backoff, a chapter that does not exist never
// will be.
func classify(err error) entity.TaskOutcome {
	switch {
	case err == nil:
		return entity.Success{}
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		// The video was cancelled or the daemon is shutting down. Retrying is
		// pointless; resume happens through the task table, not through a retry.
		return entity.Failed{Err: err, Retryable: false}
	case errors.Is(err, provider.ErrUnavailable):
		// The backend cannot run until the operator installs something. Three
		// attempts would only take three times as long to say so.
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
