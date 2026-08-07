// Package provider declares the ports through which the server reaches
// generative backends and the store their output lands in. One port per file,
// named after the file its adapters live in, so finding a backend is a
// directory listing rather than a search.
//
// The rule every backend obeys: a provider call never spans more than one unit
// of work, and all orchestration belongs to the server. The one exception is
// slide prompting, where coalescing happens behind the interface and the DAG
// still holds N individually retryable per-chapter tasks.
package provider

import "errors"

// ErrUnavailable reports a backend that cannot run at all — a missing binary or
// resource, an unconfigured credential. The one failure retrying cannot fix.
var ErrUnavailable = errors.New("backend unavailable")
