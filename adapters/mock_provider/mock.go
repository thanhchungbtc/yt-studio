// Package mockprovider implements every provider port with local, deterministic
// backends that produce real files.
//
// The mocks are the deliverable for this version, so they are held to real
// standards (§7): valid PNG, WAV and MP4 output, one unit of work per call, no
// fan-out inside a provider, and nothing of the mock leaking outside this
// package. Swapping in a real backend later is one type implementing one
// interface, plus a settings row to select it.
package mockprovider

import (
	"context"
	"errors"
	"hash/fnv"
	"math/rand/v2"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// ErrInjectedFailure is the transient error the mocks raise when the failure
// injection setting is above zero, so the retry path is exercised end to end.
var ErrInjectedFailure = errors.New("mock provider: injected transient failure")

// Tuning reports the simulated work per unit and the injected failure rate. It
// is read per call, so a settings edit applies without a restart (§3).
type Tuning func() (latency time.Duration, failureRatePercent int)

// VideoContext is everything a mock needs to produce output that is coherent
// across a whole video. It is supplied by a lookup the caller wires from the
// repositories, which keeps the provider itself free of database access.
type VideoContext struct {
	Ref              entity.Ref
	Title            string
	Topic            string
	Style            entity.StyleConfig
	Chapters         []provider.BlueprintChapter
	ImagesPerChapter int
}

// ContextLookup resolves a video's context. Wiring it explicitly is what lets
// ImagePrompts keep the narrow signature the port declares while still having
// the blueprint it needs (§4).
type ContextLookup func(ctx context.Context, videoID entity.VideoID) (VideoContext, error)

// simulate burns the configured latency and may inject a transient failure.
// It is context-aware: cancelling a video stops its in-flight calls, which is
// what frees the pool slots within 100 ms (§8.3).
func simulate(ctx context.Context, tuning Tuning, factor float64) error {
	latency, failureRate := 0*time.Millisecond, 0
	if tuning != nil {
		latency, failureRate = tuning()
	}
	if d := time.Duration(float64(latency) * factor); d > 0 {
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if failureRate > 0 && rand.IntN(100) < failureRate { //nolint:gosec // not cryptographic
		return ErrInjectedFailure
	}
	return nil
}

// seedOf derives a stable 64-bit seed from its parts. Every piece of generated
// content hangs off one of these, so the same inputs always produce the same
// bytes and therefore the same content address (§8.4, golden-file tests).
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

func pick(r *rand.Rand, options []string) string {
	if len(options) == 0 {
		return ""
	}
	return options[r.IntN(len(options))]
}
