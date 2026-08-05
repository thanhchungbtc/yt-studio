// Package mock implements the LLM port with a local, deterministic backend that
// writes real assets.
//
// It is held to the same standards as a paid backend: valid JSON and text
// output, one unit of work per call, no fan-out inside a provider, and the same
// inputs always producing the same bytes and therefore the same content
// address. Swapping in a real gateway is one type implementing one interface,
// plus a settings row to select it.
//
// The prose generators in text.go live here rather than in mockcore because
// nothing outside the LLM port produces text: an image backend needs the seed,
// not the vocabulary.
package mock

import (
	"context"
	"math/rand/v2"

	"github.com/tbui/yt-studio/adapters/mockcore"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// Tuning is re-exported so a caller wiring the mocks names one type rather than
// reaching into mockcore for it. It is an alias, not a definition: the media
// mocks take the same value, and the wiring builds it once.
type Tuning = mockcore.Tuning

// ErrInjectedFailure is re-exported for the same reason — a test asserting on
// an injected failure should not have to know which package raised it.
var ErrInjectedFailure = mockcore.ErrInjectedFailure

// VideoContext is everything the mock needs to produce output that is coherent
// across a whole video. It is supplied by a lookup the caller wires from the
// repositories, which keeps the provider itself free of database access.
type VideoContext struct {
	Ref              entity.Ref
	Title            string
	Topic            string
	Chapters         []provider.BlueprintChapter
	SlidesPerChapter int
}

// ContextLookup resolves a video's context. Wiring it explicitly is what lets
// SlidePrompts keep the narrow signature the port declares while still having
// the blueprint it needs.
type ContextLookup func(ctx context.Context, videoID entity.VideoID) (VideoContext, error)

// pick chooses one option from a vocabulary list. It stays unexported here
// because only the prose generators use it.
func pick(r *rand.Rand, options []string) string {
	if len(options) == 0 {
		return ""
	}
	return options[r.IntN(len(options))]
}
