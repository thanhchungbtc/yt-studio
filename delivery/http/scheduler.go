package http

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/tbui/yt-studio/app"
)

// SchedulerOutput is the operator console's snapshot.
type SchedulerOutput struct {
	Body SchedulerStatusDTO
}

// HealthOutput reports liveness and versions.
type HealthOutput struct {
	Body struct {
		Status  string    `json:"status"`
		Version string    `json:"version"`
		Started time.Time `json:"startedAt"`
		Clients int       `json:"sseClients"`
	}
}

func getSchedulerStatus(reporter app.StatusReporter) func(context.Context, *struct{}) (*SchedulerOutput, error) {
	return func(_ context.Context, _ *struct{}) (*SchedulerOutput, error) {
		return &SchedulerOutput{Body: statusFrom(app.GetSchedulerStatus(reporter))}, nil
	}
}

func getHealth(version string, started time.Time, clients func() int) func(context.Context, *struct{}) (*HealthOutput, error) {
	return func(_ context.Context, _ *struct{}) (*HealthOutput, error) {
		out := &HealthOutput{}
		out.Body.Status = "ok"
		out.Body.Version = version
		out.Body.Started = started
		if clients != nil {
			out.Body.Clients = clients()
		}
		return out, nil
	}
}

func registerSchedulerRoutes(
	api huma.API,
	reporter app.StatusReporter,
	version string,
	started time.Time,
	clients func() int,
) {
	huma.Register(api, huma.Operation{
		OperationID: "getSchedulerStatus", Method: "GET", Path: "/api/scheduler",
		Summary: "Pool utilisation and queue depth", Tags: []string{"scheduler"},
	}, getSchedulerStatus(reporter))

	huma.Register(api, huma.Operation{
		OperationID: "getHealth", Method: "GET", Path: "/api/health",
		Summary: "Liveness", Tags: []string{"system"},
	}, getHealth(version, started, clients))
}
