package repository

import (
	"context"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
)

// TaskEdge is one arc of a video's DAG: To may not start until From releases
// it. Edges are persisted so recovery rebuilds the exact graph.
type TaskEdge struct {
	VideoID entity.VideoID
	From    entity.TaskID
	To      entity.TaskID
}

// TaskTransition is one task's durable state change. The scheduler holds the
// authoritative copy and writes whole rows, so this is a description, not a
// patch.
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
	// Stale cuts across the states above rather than partitioning with them: a
	// stale task is usually also a succeeded one.
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

// TaskReader serves the API, recovery and the console. The scheduler never asks
// it "what can run now?" — the in-memory ready set answers that.
type TaskReader interface {
	TaskByID(ctx context.Context, id entity.TaskID) (entity.Task, error)
	ListTasksByVideo(ctx context.Context, videoID entity.VideoID) ([]entity.Task, error)
	CountTasksByVideo(ctx context.Context, videoID entity.VideoID) (TaskCounts, error)
	// ListOpenGraphs returns every video that still has open tasks, so the server
	// can resume rather than restart.
	ListOpenGraphs(ctx context.Context) ([]VideoGraph, error)
	// GraphByVideo returns one video's DAG whatever state its tasks are in, which
	// is the only way back to a video whose tasks are all terminal.
	GraphByVideo(ctx context.Context, videoID entity.VideoID) (VideoGraph, error)
	// ListRecentTasks powers the scheduler console's live table.
	ListRecentTasks(ctx context.Context, limit int) ([]entity.Task, error)
}

// TaskWriter is the scheduler's durable backing.
type TaskWriter interface {
	// InsertGraph writes a whole DAG in one transaction. Task ids are
	// deterministic, so re-enqueueing an existing video is a no-op.
	InsertGraph(ctx context.Context, videoID entity.VideoID, tasks []entity.Task, edges []TaskEdge) error
	// ApplyTransitions commits N transitions in a single transaction.
	ApplyTransitions(ctx context.Context, transitions []TaskTransition) error
}
