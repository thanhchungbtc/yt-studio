// Package mockcore is the determinism and latency machinery every mock backend
// shares.
//
// It exists because the mocks are split by capability — one package per port
// group — while the properties that make them useful are global: the same
// inputs always produce the same bytes, one settings edit changes the simulated
// latency everywhere, and one sentinel error means "injected failure" no matter
// which port raised it. Duplicating this per package would give two Tuning
// types the wiring could not share and two ErrInjectedFailure values a test
// could not compare against.
//
// Nothing here is a provider. It is deliberately the smallest surface that both
// halves need, which is why the seeding helpers are exported at all.
package mockcore

import (
	"context"
	"errors"
	"hash/fnv"
	"math/rand/v2"
	"time"
)

// ErrInjectedFailure is the transient error the mocks raise when the failure
// injection setting is above zero, so the retry path is exercised end to end.
var ErrInjectedFailure = errors.New("mock provider: injected transient failure")

// Tuning reports the simulated work per unit and the injected failure rate. It
// is read per call, so a settings edit applies without a restart.
type Tuning func() (latency time.Duration, failureRatePercent int)

// Simulate burns the configured latency and may inject a transient failure. It
// is context-aware: cancelling a video stops its in-flight calls, which is what
// frees the pool slots within 100 ms.
func Simulate(ctx context.Context, tuning Tuning, factor float64) error {
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

// SeedOf derives a stable 64-bit seed from its parts. Every piece of generated
// content hangs off one of these, so the same inputs always produce the same
// bytes and therefore the same content address.
func SeedOf(parts ...string) uint64 {
	h := fnv.New64a()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	return h.Sum64()
}

// Deterministic returns a PRNG seeded only by its inputs.
func Deterministic(seed uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15))
}
