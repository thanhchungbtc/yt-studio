package services

import (
	"github.com/tbui/yt-studio/internal/app"
	"github.com/tbui/yt-studio/internal/domain/entity"
)

// SchedulerService reports what the pipeline is doing.
type SchedulerService struct{ b *Backend }

// NewSchedulerService creates a SchedulerService.
func NewSchedulerService(b *Backend) *SchedulerService { return &SchedulerService{b: b} }

// Snapshot is the pool table now; PipelineEvent carries it as it changes.
func (s *SchedulerService) Snapshot() entity.SchedulerDelta {
	st := app.GetSchedulerStatus(s.b.Scheduler)
	return entity.SchedulerDelta{
		Pools:   st.Pools,
		Ready:   st.Ready,
		Running: st.Running,
		Blocked: st.Blocked,
		Videos:  st.Videos,
		Uptime:  st.UptimeSeconds,
	}
}
