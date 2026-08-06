// Package provider declares the ports through which the server reaches
// generative backends and the store their output lands in.
//
// One port per file, named after the file the adapter implementing it lives in,
// so the mapping between a port and its backends is a directory listing rather
// than a search.
//
// The rule every backend obeys: a provider call never spans more than one unit
// of work. No multi-chapter calls, no fan-out inside a provider. All
// orchestration — lifecycle, the cross-chapter DAG, resource pools, retries,
// persistence, gates — belongs to the server.
//
// The one deliberate exception is slide prompting, where coalescing happens
// behind the interface: the DAG still holds N individually retryable
// per-chapter tasks and the provider serves them from one primed batch.
package provider

import "errors"

// ErrUnavailable reports a backend that cannot run at all: a missing binary, a
// missing resource file, an unconfigured credential. It is worth its own
// sentinel because it is the one provider failure that retrying cannot fix —
// the operator has to change something first.
var ErrUnavailable = errors.New("backend unavailable")
