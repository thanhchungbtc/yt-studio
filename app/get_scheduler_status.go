package app

import (
	"github.com/tbui/yt-studio/domain/scheduler"
)

// GetSchedulerStatus returns pool utilisation and queue depth. It reads a
// snapshot the dispatch loop publishes atomically, so the operator console
// never blocks the loop it is watching (§9).
func GetSchedulerStatus(reporter StatusReporter) scheduler.Status {
	return reporter.Snapshot()
}
