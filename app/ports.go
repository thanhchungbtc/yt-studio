package app

import (
	"context"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/scheduler"
)

// The scheduler ports below are consumer-defined here: each one names exactly
// the single operation a use case performs, so a handler's blast radius is
// visible in its signature (§7).

// GraphSubmitter admits a freshly built DAG.
type GraphSubmitter interface {
	Submit(ctx context.Context, g *scheduler.Graph) error
}

// GraphResumer admits DAGs rebuilt from the database at startup.
type GraphResumer interface {
	Resume(ctx context.Context, graphs []*scheduler.Graph) error
}

// GateApprover releases a gated task's successors (§6).
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

// TaskRetrier resets one task and everything downstream of it.
type TaskRetrier interface {
	RetryTask(ctx context.Context, taskID entity.TaskID) error
}

// ChapterRetrier resets one chapter and everything downstream of it.
type ChapterRetrier interface {
	RetryChapter(ctx context.Context, videoID entity.VideoID, ordinal int) error
}

// PoolLimiter applies a pool limit change without a restart (§5).
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
// retry regenerates it rather than replaying the cached one (§4).
type PromptCacheInvalidator interface {
	Forget(videoID entity.VideoID)
}
