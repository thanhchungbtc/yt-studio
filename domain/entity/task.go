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
	// TaskKindPrimeSlidePrompts occupies a real LLM slot and produces every
	// chapter's slide prompts in one call, so there is a batch to coalesce.
	TaskKindPrimeSlidePrompts TaskKind = "prime_slide_prompts"
	// TaskKindSlidePrompts is the per-chapter cache read against that batch.
	TaskKindSlidePrompts TaskKind = "slide_prompts"
	// TaskKindScript writes one chapter's narration.
	TaskKindScript TaskKind = "script"
	// TaskKindTTS narrates one chapter.
	TaskKindTTS TaskKind = "tts"
	// TaskKindSlide generates one slide for one chapter.
	TaskKindSlide TaskKind = "slide"
	// TaskKindClip composes one chapter's audio and slides into a clip.
	TaskKindClip TaskKind = "clip"
	// TaskKindConcat joins every clip into the final render.
	TaskKindConcat TaskKind = "concat"
	// TaskKindMetadata writes the YouTube title, description and tags.
	TaskKindMetadata TaskKind = "metadata"
	// TaskKindThumbnailPlan writes one caption and one icon prompt per cell.
	TaskKindThumbnailPlan TaskKind = "thumbnail_plan"
	// TaskKindThumbnailIcon generates one cell's icon.
	TaskKindThumbnailIcon TaskKind = "thumbnail_icon"
	// TaskKindThumbnail composes the icons under the metadata task's hook.
	TaskKindThumbnail TaskKind = "thumbnail"
	// TaskKindUpload publishes the final render.
	TaskKindUpload TaskKind = "upload"
)

// AllTaskKinds lists every TaskKind, for validation and the UI.
var AllTaskKinds = []TaskKind{
	TaskKindBlueprint,
	TaskKindPrimeSlidePrompts,
	TaskKindSlidePrompts,
	TaskKindScript,
	TaskKindTTS,
	TaskKindSlide,
	TaskKindClip,
	TaskKindConcat,
	TaskKindMetadata,
	TaskKindThumbnailPlan,
	TaskKindThumbnailIcon,
	TaskKindThumbnail,
	TaskKindUpload,
}

// Valid reports whether the kind is one of the known constants.
func (k TaskKind) Valid() bool {
	switch k {
	case TaskKindBlueprint, TaskKindPrimeSlidePrompts, TaskKindSlidePrompts,
		TaskKindScript, TaskKindTTS, TaskKindSlide, TaskKindClip,
		TaskKindConcat, TaskKindMetadata, TaskKindThumbnailPlan,
		TaskKindThumbnailIcon, TaskKindThumbnail, TaskKindUpload:
		return true
	default:
		return false
	}
}

// Pool returns the single pool a task of this kind acquires a slot in.
func (k TaskKind) Pool() Pool {
	switch k {
	case TaskKindBlueprint, TaskKindPrimeSlidePrompts, TaskKindScript,
		TaskKindMetadata, TaskKindThumbnailPlan:
		return PoolLLM
	case TaskKindSlidePrompts:
		return PoolCache
	case TaskKindTTS:
		return PoolTTS
	case TaskKindSlide, TaskKindThumbnailIcon:
		return PoolImage
	case TaskKindClip, TaskKindConcat, TaskKindThumbnail:
		return PoolCompose
	case TaskKindUpload:
		return PoolUpload
	default:
		return PoolLLM
	}
}

// PerChapter reports whether the DAG holds one node of this kind per chapter
// rather than one per video.
func (k TaskKind) PerChapter() bool {
	switch k {
	case TaskKindScript, TaskKindTTS, TaskKindSlidePrompts, TaskKindSlide, TaskKindClip:
		return true
	case TaskKindBlueprint, TaskKindPrimeSlidePrompts, TaskKindConcat,
		TaskKindMetadata, TaskKindThumbnailPlan, TaskKindThumbnailIcon,
		TaskKindThumbnail, TaskKindUpload:
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
	// TaskStateAwaitingApproval succeeded but sits on a gate, holding its
	// dependents and consuming nothing.
	TaskStateAwaitingApproval TaskState = "awaiting_approval"
	// TaskStateSucceeded completed and released its dependents.
	TaskStateSucceeded TaskState = "succeeded"
	// TaskStateFailed exhausted its retries.
	TaskStateFailed TaskState = "failed"
	// TaskStateCancelled was stopped with its video.
	TaskStateCancelled TaskState = "cancelled"
)

// AllTaskStates lists every TaskState, for validation and the UI.
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

// Open reports whether a restart must carry the task back into memory.
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
// table is the state: an unscheduled successor consumes nothing, and the
// server may restart while a gate is open.
type Task struct {
	ID      TaskID
	VideoID VideoID
	// ChapterID is set for per-chapter kinds and nil for video-level kinds.
	ChapterID *ChapterID
	Kind      TaskKind
	// Ordinal is the chapter ordinal, or -1 for a video-level task.
	Ordinal int
	// Index distinguishes siblings of the same kind, or -1 when there is one.
	Index int

	State TaskState
	Pool  Pool
	// Gate, when set, parks the task in awaiting_approval on success rather than
	// releasing its dependents.
	Gate GateKind

	Attempt     int
	MaxAttempts int
	// DepsRemaining is persisted so recovery is exact rather than recomputed.
	DepsRemaining int

	// Stale marks a task whose output is intact but whose input has changed. A
	// flag rather than a state, because the useful combination is `succeeded`
	// and stale: the artifact may still be correct, so it waits for a decision.
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

// TaskOutcome is a sealed sum type: the unexported marker means no other
// package can add a case. Type switches over it end in a panicking default.
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

// AllTaskOutcomes returns one zero value of every outcome type.
func AllTaskOutcomes() []TaskOutcome {
	return []TaskOutcome{Success{}, Failed{}, AwaitingApproval{}}
}
