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

// Deps is the composition root's wiring record, not a container handlers read
// from: every handler takes its own narrow interfaces, and nothing in this
// package holds a reference to this struct.
type Deps struct {
	Channels      repository.ChannelReader
	ChannelWriter repository.ChannelWriter
	Videos        repository.VideoReader
	VideoWriter   repository.VideoWriter
	VideoStates   repository.VideoStateWriter
	VideoFields   repository.VideoFieldWriter
	Chapters      repository.ChapterReader
	ChapterFields repository.ChapterFieldWriter
	Assets        repository.AssetReader
	AssetWriter   repository.AssetWriter
	Tasks         repository.TaskReader
	Store         provider.AssetStore
	Settings      *service.Settings
	// UploadAuth manages the per-channel grant the uploader publishes with. A
	// port rather than the uploader itself: these four routes never publish, and
	// the publishing path never touches them.
	UploadAuth provider.UploadAuthorizer

	Submitter  app.GraphSubmitter
	Resumer    app.GraphResumer
	Requeuer   app.VideoRequeuer
	Expander   app.GraphExpander
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
	// Dist is the built web UI, embedded in production builds. A nil value serves
	// a clear error instead of a blank page.
	Dist fs.FS
	// Resources is the operator-supplied media directory, served so the browser
	// thumbnail editor can compose against the same background and typefaces
	// the builtin renderer uses. A directory rather than a path, so this layer
	// never learns where resources live on disk.
	Resources fs.FS
}

// Every list this API returns is make()d and never nil, so the client never has
// to null-check one. A package-level knob in huma, so it is set here rather
// than in NewRouter, where it would race with a second router.
func init() { huma.DefaultArrayNullable = false }

// NewRouter assembles the whole HTTP surface: typed API operations, the SSE
// stream, content-addressed asset delivery and the embedded SPA.
func NewRouter(d Deps) (http.Handler, huma.API) {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// RealIP is deliberately absent: it trusts forwarding headers, and this server
	// binds to loopback with nothing in front of it.
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(d.Log))
	r.Use(middleware.Compress(5, "application/json", "text/html", "text/css", "text/javascript", "image/svg+xml"))
	r.Use(idempotency(idempotencyTTL))

	config := huma.DefaultConfig("yt-studio", d.Version)
	config.Info.Description = "Local, single-operator automation for long-form slideshow videos."
	config.DocsPath = "/api/docs"
	config.OpenAPIPath = "/api/openapi"
	api := humachi.New(r, config)

	registerChannelRoutes(api, d.Channels, d.ChannelWriter, d.Videos, d.VideoWriter, d.Forgetter,
		d.Store, d.NewID, d.Now, d.Log)
	registerVideoRoutes(api, d.Videos, d.VideoWriter, d.VideoStates, d.Channels, d.ChannelWriter,
		d.Tasks, d.Chapters, d.Submitter, d.Resumer, d.Requeuer, d.Expander, d.Canceller, d.Approver,
		d.Rejecter, d.Forgetter,
		d.Store, d.Settings, d.NewID, d.Now, d.Log)
	registerYouTubeRoutes(api, d.Channels, d.ChannelWriter, d.UploadAuth, d.Now)
	registerChapterRoutes(api, d.Videos, d.Chapters, d.ChapterFields, d.Notifier, d.ChapRetry,
		d.Prompts, d.StaleMark, d.Rerunner)
	registerThumbnailRoutes(api, d.Videos, d.VideoFields, d.AssetWriter, d.Store,
		d.Tasks, d.Rerunner, d.Now)
	registerTaskRoutes(api, d.Videos, d.Tasks, d.TaskRetry, d.Prompts,
		d.Rerunner, d.StaleRun, d.StaleOK)
	registerSettingRoutes(api, d.Settings, d.Pools, d.Coalescer, d.LogLevel)
	registerAssetRoutes(api, d.Videos, d.Assets)
	registerSchedulerRoutes(api, d.Reporter, d.Version, d.Started, d.SSEClients)

	r.Get("/assets/{id}", assetHandler(d.Assets, d.Store))
	r.Get("/events", eventsHandler(d.Events, d.Log))
	r.Get("/resources/*", resourceHandler(d.Resources))

	r.NotFound(spaHandler(d.Dist))
	return r, api
}

// requestLogger records one structured line per request.
func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The SSE stream is long-lived; a completion line would land hours late.
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
