package http

import (
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
	"github.com/tbui/yt-studio/domain/service"
)

// Deps is the composition root's wiring record.
//
// It is not a dependency container that handlers read from: every handler and
// every register* function below takes the narrow interfaces it uses as
// separate parameters, and this struct exists only so cmd/ can name what it is
// passing (§7). Nothing in this package holds a reference to it.
type Deps struct {
	Channels      repository.ChannelReader
	ChannelWriter repository.ChannelWriter
	Videos        repository.VideoReader
	VideoWriter   repository.VideoWriter
	VideoStates   repository.VideoStateWriter
	Chapters      repository.ChapterReader
	ChapterFields repository.ChapterFieldWriter
	Assets        repository.AssetReader
	Tasks         repository.TaskReader
	TaskWriter    repository.TaskWriter
	Store         provider.AssetStore
	Settings      *service.Settings

	Submitter  app.GraphSubmitter
	Canceller  app.VideoCanceller
	Approver   app.GateApprover
	Rejecter   app.GateRejecter
	Forgetter  app.VideoForgetter
	TaskRetry  app.TaskRetrier
	ChapRetry  app.ChapterRetrier
	Rerunner   app.TaskRerunner
	StaleMark  app.StaleMarker
	StaleRun   app.StaleRunner
	StaleOK    app.StaleAccepter
	Pools      app.PoolLimiter
	Reporter   app.StatusReporter
	Prompts    app.PromptCacheInvalidator
	Notifier   app.ChapterNotifier
	Coalescer  app.CoalesceSetter
	Events     EventSource
	SSEClients func() int

	LogLevel *slog.LevelVar
	Log      *slog.Logger
	NewID    func() string
	Now      func() time.Time
	Version  string
	Started  time.Time
	// Dist is the built web UI, embedded in production builds. A nil value
	// serves a clear error instead of a blank page.
	Dist fs.FS
}

// Every list this API returns is built with make() and is never left nil, so
// the schema does not declare arrays nullable and the generated client never
// has to null-check one.
//
// This is a package-level knob in huma, so it is set once here rather than per
// router: writing it from NewRouter would race with any other router being
// constructed at the same time.
func init() { huma.DefaultArrayNullable = false }

// NewRouter assembles the whole HTTP surface: typed API operations, the SSE
// stream, content-addressed asset delivery and the embedded SPA.
func NewRouter(d Deps) (http.Handler, huma.API) {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// RealIP is deliberately absent: it trusts forwarding headers, and this
	// daemon binds to loopback with nothing in front of it.
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(d.Log))
	r.Use(middleware.Compress(5, "application/json", "text/html", "text/css", "text/javascript", "image/svg+xml"))
	r.Use(idempotency(idempotencyTTL))

	config := huma.DefaultConfig("yt-studio", d.Version)
	config.Info.Description = "Local, single-operator automation for long-form slideshow videos."
	config.DocsPath = "/api/docs"
	config.OpenAPIPath = "/api/openapi"
	api := humachi.New(r, config)

	registerChannelRoutes(api, d.Channels, d.ChannelWriter, d.Videos, d.TaskWriter, d.Forgetter, d.NewID, d.Now)
	registerVideoRoutes(api, d.Videos, d.VideoWriter, d.VideoStates, d.Channels, d.ChannelWriter,
		d.Tasks, d.TaskWriter, d.Submitter, d.Canceller, d.Approver, d.Rejecter, d.Forgetter,
		d.Settings, d.NewID, d.Now)
	registerChapterRoutes(api, d.Videos, d.Chapters, d.ChapterFields, d.Notifier, d.ChapRetry, d.Prompts, d.StaleMark)
	registerTaskRoutes(api, d.Videos, d.Tasks, d.TaskRetry, d.Prompts,
		d.Rerunner, d.StaleRun, d.StaleOK)
	registerSettingRoutes(api, d.Settings, d.Pools, d.Coalescer, d.LogLevel)
	registerAssetRoutes(api, d.Videos, d.Assets)
	registerSchedulerRoutes(api, d.Reporter, d.Version, d.Started, d.SSEClients)

	r.Get("/assets/{id}", assetHandler(d.Assets, d.Store))
	r.Get("/events", eventsHandler(d.Events, d.Log))

	r.NotFound(spaHandler(d.Dist))
	return r, api
}

// requestLogger records one structured line per request with the correlation
// attributes §8.4 requires.
func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The SSE stream is long-lived; logging it on completion would be
			// noise measured in hours.
			if r.URL.Path == "/events" {
				next.ServeHTTP(w, r)
				return
			}
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			log.Debug("http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", ww.Status()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.String("request_id", middleware.GetReqID(r.Context())),
				slog.Duration("took", time.Since(start)))
		})
	}
}
