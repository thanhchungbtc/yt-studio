package app

import (
	"context"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/scheduler"
)

// The scheduler ports below are consumer-defined here: each one names exactly
// the single operation a use case performs, so a handler's blast radius is
// visible in its signature.

// GraphSubmitter admits a freshly built DAG.
type GraphSubmitter interface {
	Submit(ctx context.Context, g *scheduler.Graph) error
}

// GraphExpander splices a video's per-chapter body onto the head graph it was
// enqueued with, once the blueprint has said how many chapters there are.
type GraphExpander interface {
	Expand(ctx context.Context, videoID entity.VideoID, tail scheduler.Tail) error
}

// GraphResumer admits DAGs rebuilt from the database at startup.
type GraphResumer interface {
	Resume(ctx context.Context, graphs []*scheduler.Graph) error
}

// GateApprover releases a gated task's successors.
type GateApprover interface {
	Approve(ctx context.Context, taskID entity.TaskID) error
}

// GateRejecter fails a gated task with an operator-supplied reason.
type GateRejecter interface {
	Reject(ctx context.Context, taskID entity.TaskID, reason string) error
}

// VideoCanceller stops a video and frees its slots.
type VideoCanceller interface {
	Cancel(ctx context.Context, videoID entity.VideoID) error
}

// VideoForgetter drops a video from the scheduler's memory.
type VideoForgetter interface {
	Forget(ctx context.Context, videoID entity.VideoID) error
}

// VideoRequeuer resets every task a video stopped on, for a resume.
type VideoRequeuer interface {
	Requeue(ctx context.Context, videoID entity.VideoID) (int, error)
}

// TaskRetrier resets one task and everything downstream of it.
type TaskRetrier interface {
	RetryTask(ctx context.Context, taskID entity.TaskID) error
}

// ChapterRetrier resets one chapter and everything downstream of it.
type ChapterRetrier interface {
	RetryChapter(ctx context.Context, videoID entity.VideoID, ordinal int) error
}

// TaskRerunner re-runs tasks that already succeeded, flagging their downstream
// stale instead of redoing it. With dryRun it reports the blast radius and
// changes nothing.
type TaskRerunner interface {
	Rerun(ctx context.Context, videoID entity.VideoID, seeds []entity.TaskID, dryRun bool) ([]entity.TaskID, error)
}

// StaleMarker flags everything downstream of the seeds without touching them,
// for an input edited outside the pipeline.
type StaleMarker interface {
	MarkStale(ctx context.Context, videoID entity.VideoID, seeds []entity.TaskID) ([]entity.TaskID, error)
}

// StaleRunner re-runs stale tasks; a nil id list means all of them.
type StaleRunner interface {
	RunStale(ctx context.Context, videoID entity.VideoID, ids []entity.TaskID) (int, error)
}

// StaleAccepter clears the stale flag without re-running: the operator checked
// the artifact and kept it.
type StaleAccepter interface {
	AcceptStale(ctx context.Context, videoID entity.VideoID, ids []entity.TaskID) (int, error)
}

// PoolLimiter applies a pool limit change without a restart.
type PoolLimiter interface {
	SetPoolLimit(ctx context.Context, pool entity.Pool, limit int) error
}

// StatusReporter answers the operator console.
type StatusReporter interface {
	Snapshot() scheduler.Status
}

// ChapterNotifier publishes a chapter delta to connected clients.
type ChapterNotifier interface {
	NotifyChapter(d entity.ChapterDelta)
}

// CoalesceSetter applies an SSE coalescing-window change without a restart.
type CoalesceSetter interface {
	SetCoalesce(d time.Duration)
}

// PromptCacheInvalidator drops a video's coalesced image-prompt batch so a
// retry regenerates it rather than replaying the cached one.
type PromptCacheInvalidator interface {
	Forget(videoID entity.VideoID)
}
