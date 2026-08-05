// Package mock implements the media and upload ports with local,
// deterministic backends that produce real files.
//
// The mocks are the deliverable for this version, so they are held to real
// standards: valid PNG, WAV and MP4 output, one unit of work per call, no
// fan-out inside a provider, and nothing of the mock leaking outside this
// package. Swapping in a real backend later is one type implementing one
// interface, plus a settings row to select it.
//
// The seeding and latency machinery is in adapters/mockcore, shared with the
// LLM mock: one Tuning type so the wiring builds it once, and one
// ErrInjectedFailure so a test need not know which port raised it.
package mock

import (
	"github.com/tbui/yt-studio/adapters/mockcore"
)

// Tuning is re-exported so a caller wiring the mocks names one type rather than
// reaching into mockcore for it. It is an alias, not a definition: the LLM mock
// takes the same value.
type Tuning = mockcore.Tuning

// ErrInjectedFailure is re-exported for the same reason — a test asserting on an
// injected failure should not have to know which package raised it.
var ErrInjectedFailure = mockcore.ErrInjectedFailure
