package entity

import (
	"errors"
	"time"
)

// ErrTaskNotFound is returned by task lookups for an unknown id.
var ErrTaskNotFound = errors.New("task not found")

// TaskKind identifies a node type in the per-video DAG.
type TaskKind string

// The complete set of task kinds.
const (
	// TaskKindBlueprint produces the chapter outline for the whole video.
	TaskKindBlueprint TaskKind = "blueprint"
	// TaskKindPrimeImagePrompts occupies a real LLM slot and produces every
	// chapter's image prompts in one call, so there is a batch to coalesce.
	TaskKindPrimeImagePrompts TaskKind = "prime_image_prompts"
	// TaskKindImagePrompts is the per-chapter cache read against that batch.
	TaskKindImagePrompts TaskKind = "image_prompts"
	// TaskKindScript writes one chapter's narration.
	TaskKindScript TaskKind = "script"
	// TaskKindTTS narrates one chapter.
	TaskKindTTS TaskKind = "tts"
	// TaskKindImage generates one still for one chapter.
	TaskKindImage TaskKind = "image"
	// TaskKindClip composes one chapter's audio and stills into a clip.
	TaskKindClip TaskKind = "clip"
	// TaskKindConcat joins every clip into the final render.
	TaskKindConcat TaskKind = "concat"
	// TaskKindMetadata writes the YouTube title, description and tags.
	TaskKindMetadata TaskKind = "metadata"
	// TaskKindUpload publishes the final render.
	TaskKindUpload TaskKind = "upload"
)

// AllTaskKinds lists every TaskKind, for validation, the UI and tests.
var AllTaskKinds = []TaskKind{
	TaskKindBlueprint,
	TaskKindPrimeImagePrompts,
	TaskKindImagePrompts,
	TaskKindScript,
	TaskKindTTS,
	TaskKindImage,
	TaskKindClip,
	TaskKindConcat,
	TaskKindMetadata,
	TaskKindUpload,
}

// Valid reports whether the kind is one of the known constants.
func (k TaskKind) Valid() bool {
	switch k {
	case TaskKindBlueprint, TaskKindPrimeImagePrompts, TaskKindImagePrompts,
		TaskKindScript, TaskKindTTS, TaskKindImage, TaskKindClip,
		TaskKindConcat, TaskKindMetadata, TaskKindUpload:
		return true
	default:
		return false
	}
}

// Pool returns the single pool a task of this kind acquires a slot in.
func (k TaskKind) Pool() Pool {
	switch k {
	case TaskKindBlueprint, TaskKindPrimeImagePrompts, TaskKindScript, TaskKindMetadata:
		return PoolLLM
	case TaskKindImagePrompts:
		return PoolCache
	case TaskKindTTS:
		return PoolTTS
	case TaskKindImage:
		return PoolImage
	case TaskKindClip, TaskKindConcat:
		return PoolCompose
	case TaskKindUpload:
		return PoolUpload
	default:
		return PoolLLM
	}
}

// PerChapter reports whether the DAG contains one node of this kind per chapter
// (as opposed to one per video).
func (k TaskKind) PerChapter() bool {
	switch k {
	case TaskKindScript, TaskKindTTS, TaskKindImagePrompts, TaskKindImage, TaskKindClip:
		return true
	case TaskKindBlueprint, TaskKindPrimeImagePrompts, TaskKindConcat, TaskKindMetadata, TaskKindUpload:
		return false
	default:
		return false
	}
}

// TaskState is the durable state of a single task.
type TaskState string

// The complete set of task states.
const (
	// TaskStateBlocked has unmet dependencies and is not in the ready set.
	TaskStateBlocked TaskState = "blocked"
	// TaskStateReady is runnable and waiting for a pool slot.
	TaskStateReady TaskState = "ready"
	// TaskStateRunning holds a pool slot and is inside a provider call.
	TaskStateRunning TaskState = "running"
	// TaskStateAwaitingApproval succeeded but sits on a gate; it has not released
	// its dependents and consumes nothing.
	TaskStateAwaitingApproval TaskState = "awaiting_approval"
	// TaskStateSucceeded completed and released its dependents.
	TaskStateSucceeded TaskState = "succeeded"
	// TaskStateFailed exhausted its retries.
	TaskStateFailed TaskState = "failed"
	// TaskStateCancelled was stopped with its video.
	TaskStateCancelled TaskState = "cancelled"
)

// AllTaskStates lists every TaskState, for validation, the UI and tests.
var AllTaskStates = []TaskState{
	TaskStateBlocked,
	TaskStateReady,
	TaskStateRunning,
	TaskStateAwaitingApproval,
	TaskStateSucceeded,
	TaskStateFailed,
	TaskStateCancelled,
}

// Valid reports whether the state is one of the known constants.
func (s TaskState) Valid() bool {
	switch s {
	case TaskStateBlocked, TaskStateReady, TaskStateRunning, TaskStateAwaitingApproval,
		TaskStateSucceeded, TaskStateFailed, TaskStateCancelled:
		return true
	default:
		return false
	}
}

// Terminal reports whether the task will not run again without operator action.
func (s TaskState) Terminal() bool {
	switch s {
	case TaskStateSucceeded, TaskStateFailed, TaskStateCancelled:
		return true
	case TaskStateBlocked, TaskStateReady, TaskStateRunning, TaskStateAwaitingApproval:
		return false
	default:
		return false
	}
}

// Open reports whether the scheduler must still carry the task in memory after
// a restart.
func (s TaskState) Open() bool {
	switch s {
	case TaskStateBlocked, TaskStateReady, TaskStateRunning, TaskStateAwaitingApproval:
		return true
	case TaskStateSucceeded, TaskStateFailed, TaskStateCancelled:
		return false
	default:
		return false
	}
}

// GateKind names a human approval gate.
type GateKind string

// The complete set of gates. GateNone means the task releases its dependents
// itself.
const (
	GateNone      GateKind = ""
	GateBlueprint GateKind = "blueprint"
	GateUpload    GateKind = "upload"
)

// AllGateKinds lists every real gate, for validation and the UI.
var AllGateKinds = []GateKind{GateBlueprint, GateUpload}

// Valid reports whether the gate is one of the known constants.
func (g GateKind) Valid() bool {
	switch g {
	case GateNone, GateBlueprint, GateUpload:
		return true
	default:
		return false
	}
}

// Task is one node of one video's DAG and the scheduler's unit of work. The
// table is the state: an unscheduled successor consumes nothing and the daemon
// may restart freely while a gate is open.
type Task struct {
	ID      TaskID
	VideoID VideoID
	// ChapterID is set for per-chapter kinds and nil for video-level kinds.
	ChapterID *ChapterID
	Kind      TaskKind
	// Ordinal is the chapter ordinal, or -1 for a video-level task.
	Ordinal int
	// Index distinguishes siblings of the same kind for one chapter — the image
	// index within a chapter — or -1 when there is only one.
	Index int

	State TaskState
	Pool  Pool
	// Gate, when set, means: on success, park in awaiting_approval and do not
	// release dependents until a human approves.
	Gate GateKind

	Attempt     int
	MaxAttempts int
	// DepsRemaining is the count of unsatisfied dependencies. It is persisted so
	// that recovery after a crash is exact rather than recomputed.
	DepsRemaining int

	// Stale marks a task whose own output is intact but whose input has since
	// changed — an upstream task was re-run, or a chapter script was edited.
	//
	// A flag rather than a state, because the useful combination is `succeeded`
	// *and* stale: the artifact is still there and may still be correct. A stale
	// task never runs on its own; it waits for the operator to run or accept it.
	Stale bool

	Error      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	StartedAt  *time.Time
	FinishedAt *time.Time
	// NotBefore delays a retry without polling: the scheduler arms one timer.
	NotBefore *time.Time
}

// Retryable reports whether another attempt is permitted.
func (t *Task) Retryable() bool { return t.Attempt < t.MaxAttempts }

// TaskOutcome is a sealed sum type: the unexported marker method means no other
// package can add a case. Every type switch over it must end with a default
// that panics, and a table-driven test asserts each site handles every case.
type TaskOutcome interface{ isTaskOutcome() }

// Success is the outcome of a task that produced its assets.
type Success struct {
	Assets []AssetID
}

// Failed is the outcome of a task whose provider call returned an error.
// Retryable distinguishes a transient failure from a permanent one.
type Failed struct {
	Err       error
	Retryable bool
}

// AwaitingApproval is the outcome of a gated task: it succeeded, but a human
// must release its dependents.
type AwaitingApproval struct {
	Gate   GateKind
	Assets []AssetID
}

func (Success) isTaskOutcome()          {}
func (Failed) isTaskOutcome()           {}
func (AwaitingApproval) isTaskOutcome() {}

// AllTaskOutcomes returns one zero value of every outcome type. The
// exhaustiveness tests iterate it against every type-switch site.
func AllTaskOutcomes() []TaskOutcome {
	return []TaskOutcome{Success{}, Failed{}, AwaitingApproval{}}
}
