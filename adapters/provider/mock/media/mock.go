// Package media implements the media and upload ports with local,
// deterministic backends that produce real files.
//
// The mocks are the deliverable for this version, so they are held to real
// standards: valid PNG, WAV and MP4 output, one unit of work per call, no
// fan-out inside a provider, and nothing of the mock leaking outside this
// package. Swapping in a real backend later is one type implementing one
// interface, plus a settings row to select it.
package media

import (
	"hash/fnv"
	"math/rand/v2"
)

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
