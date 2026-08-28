package entity

import "time"

// EventKind names a delta pushed to clients over SSE. Events carry the delta,
// never a full state dump.
type EventKind string

// The complete set of event kinds.
const (
	// EventKindBatch carries every delta accumulated for one video within a
	// coalescing window, so a 50-chapter render is not hundreds of messages.
	EventKindBatch EventKind = "batch"
	// EventKindScheduler carries pool utilisation for the operator console.
	EventKindScheduler EventKind = "scheduler"
)

// Valid reports whether the kind is one of the known constants.
func (k EventKind) Valid() bool {
	switch k {
	case EventKindBatch, EventKindScheduler:
		return true
	default:
		return false
	}
}

// TaskDelta is the subset of a Task that changes often enough to stream.
type TaskDelta struct {
	ID        TaskID     `json:"id"`
	VideoID   VideoID    `json:"videoId"`
	ChapterID *ChapterID `json:"chapterId,omitempty"`
	Kind      TaskKind   `json:"kind"`
	Ordinal   int        `json:"ordinal"`
	Index     int        `json:"index"`
	State     TaskState  `json:"state"`
	Pool      Pool       `json:"pool"`
	Gate      GateKind   `json:"gate,omitempty"`
	Attempt   int        `json:"attempt"`
	// Percent is how far a long-running task has got, 0-100. It is meaningful
	// only while State is running, and it is never persisted: no delta built
	// from a stored Task carries one. It exists so a concat that takes three
	// minutes can say so without a state change to hang the report on.
	Percent int `json:"percent,omitempty"`
	// Stale rides along so the UI can flag a task without refetching the video.
	Stale bool   `json:"stale"`
	Error string `json:"error,omitempty"`
	// NotBefore rides along because a retryable failure falls back to blocked,
	// and nothing else distinguishes a dependency wait from a backoff timer.
	NotBefore *time.Time `json:"notBefore,omitempty"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// VideoDelta is the subset of a Video that changes often enough to stream. It
// carries the id alone: the scheduler never loads a video row, so a ref here
// would always go out empty and a client keyed off it would drop every update.
type VideoDelta struct {
	ID    VideoID    `json:"id"`
	State VideoState `json:"state"`
	// Done and Total drive the progress bar without a refetch.
	Done      int       `json:"done"`
	Total     int       `json:"total"`
	Failed    int       `json:"failed"`
	Running   int       `json:"running"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ChapterDelta is the subset of a Chapter that changes often enough to stream.
type ChapterDelta struct {
	ID            ChapterID `json:"id"`
	VideoID       VideoID   `json:"videoId"`
	Ordinal       int       `json:"ordinal"`
	Title         string    `json:"title"`
	HasScript     bool      `json:"hasScript"`
	AudioAssetID  *AssetID  `json:"audioAssetId,omitempty"`
	SlideAssetIDs []AssetID `json:"slideAssetIds,omitempty"`
	ClipAssetID   *AssetID  `json:"clipAssetId,omitempty"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// PoolStat is the live utilisation of one pool, for the operator console.
type PoolStat struct {
	Pool     Pool `json:"pool"`
	Limit    int  `json:"limit"`
	InFlight int  `json:"inFlight"`
	Queued   int  `json:"queued"`
}

// SchedulerDelta is the whole pool table plus aggregate counters.
type SchedulerDelta struct {
	Pools    []PoolStat `json:"pools"`
	Ready    int        `json:"ready"`
	Running  int        `json:"running"`
	Blocked  int        `json:"blocked"`
	Videos   int        `json:"videos"`
	Uptime   float64    `json:"uptimeSeconds"`
	UpdatedA time.Time  `json:"updatedAt"`
}

// Event is one message on the multiplexed SSE stream: one stream per client
// carries every video.
type Event struct {
	// ID increases monotonically; a reconnecting client sends it back as
	// Last-Event-ID to resume without a full reload.
	ID      uint64    `json:"id"`
	Kind    EventKind `json:"kind"`
	VideoID VideoID   `json:"videoId,omitempty"`
	At      time.Time `json:"at"`

	Tasks     []TaskDelta     `json:"tasks,omitempty"`
	Video     *VideoDelta     `json:"video,omitempty"`
	Chapters  []ChapterDelta  `json:"chapters,omitempty"`
	Scheduler *SchedulerDelta `json:"scheduler,omitempty"`
}

// Delta returns a streamable projection of a task.
func (t *Task) Delta() TaskDelta {
	return TaskDelta{
		ID:        t.ID,
		VideoID:   t.VideoID,
		ChapterID: t.ChapterID,
		Kind:      t.Kind,
		Ordinal:   t.Ordinal,
		Index:     t.Index,
		State:     t.State,
		Pool:      t.Pool,
		Gate:      t.Gate,
		Attempt:   t.Attempt,
		Stale:     t.Stale,
		Error:     t.Error,
		NotBefore: t.NotBefore,
		UpdatedAt: t.UpdatedAt,
	}
}
