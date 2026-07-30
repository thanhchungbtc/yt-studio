package repository

import (
	"context"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
)

// TaskEdge is one dependency arc of a video's DAG: To may not start until From
// has released it. Edges are persisted so that recovery after a crash rebuilds
// the exact graph rather than re-deriving it.
type TaskEdge struct {
	VideoID entity.VideoID
	From    entity.TaskID
	To      entity.TaskID
}

// TaskTransition is one durable state change of one task. The scheduler holds
// the authoritative in-memory copy and writes whole rows, so a transition is a
// complete description rather than a patch.
type TaskTransition struct {
	ID            entity.TaskID
	State         entity.TaskState
	Attempt       int
	DepsRemaining int
	Stale         bool
	Error         string
	StartedAt     *time.Time
	FinishedAt    *time.Time
	NotBefore     *time.Time
	UpdatedAt     time.Time
}

// TaskCounts is the per-video aggregate the video list and SSE deltas need
// without loading every row.
type TaskCounts struct {
	Total            int
	Succeeded        int
	Failed           int
	Running          int
	Ready            int
	Blocked          int
	AwaitingApproval int
	Cancelled        int
	// Stale counts tasks whose input changed after they ran. It cuts across the
	// states above rather than partitioning with them: a stale task is usually
	// also a succeeded one.
	Stale             int
	FirstOpenGateKind entity.GateKind
}

// Done returns the number of tasks that will not run again successfully.
func (c TaskCounts) Done() int { return c.Succeeded }

// VideoGraph is a whole video's persisted DAG, as loaded at startup.
type VideoGraph struct {
	VideoID entity.VideoID
	Tasks   []entity.Task
	Edges   []TaskEdge
}

// TaskReader reads the task table. The scheduler never asks it "what can run
// now?" — that is answered by the in-memory ready set. These queries serve
// the API, recovery and the operator console.
type TaskReader interface {
	TaskByID(ctx context.Context, id entity.TaskID) (entity.Task, error)
	ListTasksByVideo(ctx context.Context, videoID entity.VideoID) ([]entity.Task, error)
	CountTasksByVideo(ctx context.Context, videoID entity.VideoID) (TaskCounts, error)
	// ListOpenGraphs returns every video whose DAG still has open tasks, with its
	// edges, so the daemon can resume rather than restart.
	ListOpenGraphs(ctx context.Context) ([]VideoGraph, error)
	// ListRecentTasks powers the scheduler console's live table.
	ListRecentTasks(ctx context.Context, limit int) ([]entity.Task, error)
}

// TaskWriter is the scheduler's durable backing.
type TaskWriter interface {
	// InsertGraph writes a whole DAG in one transaction, idempotently: task ids
	// are deterministic, so re-enqueueing an existing video is a no-op.
	InsertGraph(ctx context.Context, videoID entity.VideoID, tasks []entity.Task, edges []TaskEdge) error
	// ApplyTransitions commits N transitions in a single transaction.
	ApplyTransitions(ctx context.Context, transitions []TaskTransition) error
	// DeleteGraph removes a video's tasks and edges, for a full re-run.
	DeleteGraph(ctx context.Context, videoID entity.VideoID) error
}
