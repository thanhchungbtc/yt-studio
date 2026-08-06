// Package mock implements the LLM port with a local, deterministic backend that
// writes real assets.
//
// It is held to the same standards as a paid backend: valid JSON and text
// output, one unit of work per call, no fan-out inside a provider, and the same
// inputs always producing the same bytes and therefore the same content
// address. Swapping in a real gateway is one type implementing one interface,
// plus a settings row to select it.
//
// The prose generators in text.go live here rather than anywhere shared because
// nothing outside the LLM port produces text: an image backend needs the seed,
// not the vocabulary.
package llm

import (
	"context"
	"hash/fnv"
	"math/rand/v2"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

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

// seedOf derives a stable 64-bit seed from its parts. Every piece of generated
// content hangs off one of these, so the same inputs always produce the same
// bytes and therefore the same content address.
func seedOf(parts ...string) uint64 {
	h := fnv.New64a()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	return h.Sum64()
}

// deterministic returns a PRNG seeded only by its inputs.
func deterministic(seed uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15))
}

// pick chooses one option from a vocabulary list.
func pick(r *rand.Rand, options []string) string {
	if len(options) == 0 {
		return ""
	}
	return options[r.IntN(len(options))]
}
