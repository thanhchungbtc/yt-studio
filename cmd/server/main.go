// Command server is the whole of yt-studio: HTTP handling, the state machine
// and the scheduler, with the built web UI embedded.
//
// It wires config → adapters → app → delivery → serve, and owns every
// long-lived goroutine through one errgroup so shutdown is deterministic.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/tbui/yt-studio/adapters/assetstore"
	"github.com/tbui/yt-studio/adapters/eventbus"
	"github.com/tbui/yt-studio/adapters/ffmpeg"
	mockprovider "github.com/tbui/yt-studio/adapters/mock_provider"
	"github.com/tbui/yt-studio/adapters/ninerouter"
	"github.com/tbui/yt-studio/adapters/registry"
	"github.com/tbui/yt-studio/adapters/runware"
	sampleprovider "github.com/tbui/yt-studio/adapters/sample_provider"
	"github.com/tbui/yt-studio/adapters/sqlite"
	"github.com/tbui/yt-studio/adapters/thumbnail"
	"github.com/tbui/yt-studio/app"
	deliveryhttp "github.com/tbui/yt-studio/delivery/http"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/scheduler"
	"github.com/tbui/yt-studio/domain/service"
	"github.com/tbui/yt-studio/web"
)

// version is stamped by the build (-ldflags "-X main.version=...").
var version = "dev"

// bootstrap holds the only configuration that must exist before the database is
// open. Everything else is a settings row.
type bootstrap struct {
	DB     string `help:"SQLite database file." default:"var/yt-studio.db" env:"YTS_DB" type:"path"`
	Assets string `help:"Content-addressed asset store root." default:"var/assets" env:"YTS_ASSETS" type:"path"`
	//nolint:lll // one flag, one line
	Resources string `help:"Fixed production assets: chalkboard.jpg, bg.mp4, bg.mp3, fonts/." default:"var/resources" env:"YTS_RESOURCES" type:"path"`
	Listen    string `help:"Listen address." default:"127.0.0.1:8080" env:"YTS_LISTEN"`
	//nolint:lll // one flag, one line
	NineRouterURL string `help:"9router gateway base URL." default:"http://127.0.0.1:20128" env:"NINEROUTER_URL"`
	//nolint:lll // one flag, one line
	NineRouterKey string `help:"9router API key. A gateway running with auth off needs none." env:"NINEROUTER_KEY"`
	//nolint:lll // one flag, one line
	RunwareKey string `help:"Runware API key, for the runware image and thumbnail-icon backends." env:"RUNWARE_KEY"`
	//nolint:lll // one flag, one line
	Transcripts string `help:"Where each LLM prompt and response is written for inspection. Empty disables." default:"var/transcripts" env:"YTS_TRANSCRIPTS" type:"path"`
	//nolint:lll // one flag, one line
	LogLevel string `help:"Startup log level; the settings table takes over once loaded." default:"info" env:"YTS_LOG_LEVEL" enum:"debug,info,warn,error"`
}

type serveCmd struct {
	bootstrap
}

type seedCmd struct {
	bootstrap
}

type sweepCmd struct {
	bootstrap
	//nolint:lll // one flag, one line
	Apply bool `help:"Actually delete. Without it the sweep only reports what it would free." default:"false"`
	//nolint:lll // one flag, one line
	Force bool `help:"Sweep even when the database references no assets at all, which normally means --db is wrong." default:"false"`
}

type versionCmd struct{}

type cli struct {
	Serve   serveCmd   `cmd:"" default:"withargs" help:"Run the server."`
	Seed    seedCmd    `cmd:"" help:"Apply migrations and write the default settings and channels, then exit."`
	Sweep   sweepCmd   `cmd:"" help:"Reclaim asset files nothing references. Reports unless --apply is given."`
	Version versionCmd `cmd:"" help:"Print the version."`
}

func main() {
	var root cli
	// Before the parse, not after: kong reads its `env:` tags through os.Getenv
	// while parsing, so anything the file supplies has to be in the environment
	// by now.
	if err := loadEnvFile(); err != nil {
		fmt.Fprintln(os.Stderr, "yt-studio:", err)
		os.Exit(1)
	}
	kctx := kong.Parse(&root,
		kong.Name("yt-studio"),
		kong.Description("Local automation for long-form slideshow videos."),
		kong.UsageOnError(),
	)
	if err := kctx.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "yt-studio:", err)
		os.Exit(1)
	}
}

func (c *versionCmd) Run() error {
	fmt.Println("yt-studio", version)
	return nil
}

func (c *seedCmd) Run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	_, log := newLogger(c.LogLevel)

	store, err := sqlite.Open(ctx, sqlite.Options{Path: c.DB}, log)
	if err != nil {
		return err
	}
	g, gctx := errgroup.WithContext(ctx)
	writerCtx, stopWriter := context.WithCancel(gctx)
	g.Go(func() error { return store.Run(writerCtx) })

	err = seed(gctx, store, log)
	stopWriter()
	return errors.Join(err, g.Wait(), store.Close())
}

// Run reclaims store files that nothing in the database references.
//
// It repairs asset ownership first, unconditionally. That order is the whole
// safety property: a file a surviving video reaches only through its chapters'
// id lists has no owning row until the repair gives it one, and without the row
// this command would read it as garbage.
func (c *sweepCmd) Run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	_, log := newLogger(c.LogLevel)

	store, err := sqlite.Open(ctx, sqlite.Options{Path: c.DB}, log)
	if err != nil {
		return err
	}
	g, gctx := errgroup.WithContext(ctx)
	writerCtx, stopWriter := context.WithCancel(gctx)
	g.Go(func() error { return store.Run(writerCtx) })

	err = sweep(gctx, store, c.Assets, c.Apply, c.Force, log)
	stopWriter()
	return errors.Join(err, g.Wait(), store.Close())
}

func sweep(
	ctx context.Context,
	store *sqlite.Store,
	root string,
	apply, force bool,
	log *slog.Logger,
) error {
	assets, err := assetstore.New(root)
	if err != nil {
		return err
	}
	if _, err := app.RepairAssetOwnership(ctx, store, store, store, assets, time.Now().UTC(), log); err != nil {
		return fmt.Errorf("repair asset ownership: %w", err)
	}
	report, sweepErr := app.SweepAssets(ctx, store, assets, app.SweepOptions{
		Apply: apply,
		Force: force,
		Now:   time.Now(),
	}, log)
	// The report is printed even when the sweep refused: those numbers are what
	// explain the refusal.
	printSweepReport(report, apply && sweepErr == nil)
	return sweepErr
}

func printSweepReport(report app.SweepReport, applied bool) {
	fmt.Printf("%-14s %d\n", "files", report.Files)
	fmt.Printf("%-14s %d\n", "referenced", report.Referenced)
	fmt.Printf("%-14s %d\n", "unreferenced", report.Unreferenced)
	fmt.Printf("%-14s %d\n", "debris", report.Debris)
	if report.Unrecognised > 0 {
		fmt.Printf("%-14s %d (kept; nothing in the database describes them)\n", "unrecognised", report.Unrecognised)
		for _, rel := range report.UnrecognisedSample {
			fmt.Printf("               %s\n", rel)
		}
	}
	if applied {
		fmt.Printf("%-14s %d files, %s\n", "removed", report.Removed, humanBytes(report.Bytes))
		if report.DirsRemoved > 0 {
			fmt.Printf("%-14s %d empty directories\n", "pruned", report.DirsRemoved)
		}
		return
	}
	if report.Reclaimable() > 0 {
		fmt.Printf("\n%d files can be reclaimed. Re-run with --apply to delete them.\n", report.Reclaimable())
	}
}

// humanBytes formats a byte count for one line of operator output.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PiB", value)
}

func seed(ctx context.Context, store *sqlite.Store, log *slog.Logger) error {
	if err := sqlite.SeedSettings(ctx, store); err != nil {
		return err
	}
	if err := sqlite.SeedChannels(ctx, store, time.Now().UTC()); err != nil {
		return err
	}
	log.Info("seed applied")
	return nil
}

//nolint:funlen // this is the composition root; splitting it would only hide the wiring
func (c *serveCmd) Run() error {
	started := time.Now()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	level, log := newLogger(c.LogLevel)
	if envFileLoaded != "" {
		// The path and the count, never the values: a key in a log line is a
		// leaked key.
		log.Info("loaded environment file",
			slog.String("path", envFileLoaded),
			slog.Int("vars", envVarsLoaded))
	}

	// --- adapters -----------------------------------------------------------
	store, err := sqlite.Open(ctx, sqlite.Options{Path: c.DB}, log)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	// The writer goroutine has its own cancel so it outlives the request context
	// long enough to flush the scheduler's final transitions.
	writerCtx, stopWriter := context.WithCancel(context.WithoutCancel(ctx))
	writerGroup, _ := errgroup.WithContext(context.Background())
	writerGroup.Go(func() error { return store.Run(writerCtx) })
	defer func() {
		stopWriter()
		_ = writerGroup.Wait()
	}()

	if err := seed(ctx, store, log); err != nil {
		return err
	}

	settings := service.NewSettings(store, store)

	assets, err := assetstore.New(c.Assets)
	if err != nil {
		return err
	}

	// Before anything can serve a delete, every reference to a stored file must
	// have the row that says who owns it: that is what a delete consults to decide
	// which files it may reclaim. Normally this finds nothing and costs one query.
	if repaired, err := app.RepairAssetOwnership(ctx, store, store, store, assets, time.Now().UTC(), log); err != nil {
		return fmt.Errorf("repair asset ownership: %w", err)
	} else if repaired > 0 {
		log.Info("reconstructed asset ownership rows", slog.Int("rows", repaired))
	}

	// Backends are registered before the settings are loaded, so a row naming one
	// that does not exist fails at startup rather than at first task.
	tuning := mockprovider.Tuning(func() (time.Duration, int) {
		return settings.Duration(entity.SettingMockLatencyMillis),
			settings.Int(entity.SettingMockFailureRatePercent)
	})
	ffmpegComposer := ffmpeg.New(assets, c.Resources, log)
	thumbnails := thumbnail.New(assets, c.Resources, func() thumbnail.Options {
		return thumbnail.Options{
			Font: settings.String(entity.SettingThumbnailFont),
			Rows: settings.Int(entity.SettingThumbnailGridRows),
		}
	}, log)
	samples := sampleprovider.NewLibrary(c.Resources)

	// The model is read through a closure rather than captured: settings are not
	// loaded until after every backend has registered, and a model picked on the
	// settings screen has to apply to the next generation rather than the next
	// restart.
	nineRouter, err := ninerouter.New(ninerouter.Config{
		BaseURL:       c.NineRouterURL,
		APIKey:        c.NineRouterKey,
		Model:         func() string { return settings.String(entity.SettingNineRouterModel) },
		TranscriptDir: c.Transcripts,
	}, assets, nineRouterContextLookup(store))
	if err != nil {
		return err
	}

	// Same closure treatment, and for the same reason: nothing here may read a
	// settings value before Load, and the size is picked on the settings screen.
	runwareClient, err := runware.New(runware.Config{
		APIKey: c.RunwareKey,
		Model:  func() string { return settings.String(entity.SettingRunwareModel) },
		StillSize: func() (int, int) {
			return settings.Int(entity.SettingRunwareWidth), settings.Int(entity.SettingRunwareHeight)
		},
	}, assets, log)
	if err != nil {
		return err
	}

	providers := registry.New(settings.String)
	providers.RegisterLLM("mock", mockprovider.NewLLM(assets, videoContextLookup(store), tuning))
	providers.RegisterLLM("9router", nineRouter)
	providers.RegisterTTS("mock", mockprovider.NewTTS(assets, tuning))
	providers.RegisterTTS("sample", sampleprovider.NewTTS(samples, assets))
	providers.RegisterSlide("mock", mockprovider.NewSlide(assets, tuning))
	providers.RegisterSlide("sample", sampleprovider.NewSlide(samples, assets))
	providers.RegisterSlide("runware", runware.NewSlide(runwareClient))
	providers.RegisterComposer("mock", mockprovider.NewComposer(assets, tuning))
	providers.RegisterComposer("ffmpeg", ffmpegComposer)
	providers.RegisterThumbnail("mock", mockprovider.NewThumbnail(assets, tuning))
	providers.RegisterThumbnail("builtin", thumbnails)
	providers.RegisterThumbnailIcon("mock", mockprovider.NewIcon(assets, tuning))
	providers.RegisterThumbnailIcon("sample", sampleprovider.NewIcon(samples, assets))
	providers.RegisterThumbnailIcon("runware", runware.NewIcon(runwareClient))
	providers.RegisterUploader("mock", mockprovider.NewUploader(assets, tuning, time.Now))

	settings.Constrain(providers.Options())
	if err := settings.Load(ctx); err != nil {
		return err
	}
	if parsed, err := app.ParseLogLevel(settings.String(entity.SettingLogLevel)); err == nil {
		level.Set(parsed)
	}

	broker := eventbus.New(settings.Duration(entity.SettingSSECoalesceMillis), log)

	if err := ffmpegComposer.Check(); err != nil {
		log.Info("ffmpeg composer is not available",
			slog.String("reason", err.Error()),
			slog.String("resources", c.Resources))
	}
	if err := thumbnails.Check(); err != nil {
		log.Info("the built-in thumbnail renderer is not available",
			slog.String("reason", err.Error()),
			slog.String("resources", c.Resources))
	}
	if err := samples.Check(); err != nil {
		log.Info("sample backends are not available",
			slog.String("reason", err.Error()),
			slog.String("dir", samples.Dir()))
	}
	if err := nineRouter.Check(ctx); err != nil {
		log.Info("9router is not available",
			slog.String("reason", err.Error()),
			slog.String("url", c.NineRouterURL))
	} else {
		log.Info("9router is available",
			slog.String("url", c.NineRouterURL),
			slog.String("model", nineRouter.Model()),
			slog.String("transcripts", c.Transcripts))
	}
	if err := runwareClient.Check(); err != nil {
		log.Info("runware image backends are not available",
			slog.String("reason", err.Error()))
	} else {
		log.Info("runware image backends are available",
			slog.String("model", runwareClient.Model()),
			slog.Int("width", settings.Int(entity.SettingRunwareWidth)),
			slog.Int("height", settings.Int(entity.SettingRunwareHeight)))
	}

	// --- scheduler ----------------------------------------------------------
	pools, err := scheduler.NewPools(settings.PoolLimits())
	if err != nil {
		return err
	}
	// An ungated blueprint expands its own video's DAG, so the runner needs the
	// scheduler and the scheduler needs the runner. The reference is filled in
	// below, before anything can run.
	expander := &lateExpander{}
	runner := app.NewTaskRunner(
		store, store, store, store, store, store, store,
		assets, providers.LLM(), providers.TTS(), providers.Slide(),
		providers.Composer(), providers.Thumbnail(), providers.ThumbnailIcon(),
		providers.Uploader(), broker,
		expander,
		func() app.BlueprintOptions {
			return app.BlueprintOptions{
				ChapterTolerancePercent: settings.Int(entity.SettingVideoChapterTolerancePercent),
				MaxAttempts:             settings.Int(entity.SettingTaskMaxAttempts),
				UploadGate:              settings.GateEnabled(entity.GateUpload),
			}
		},
		func() app.IconOptions {
			return app.IconOptions{
				Style: settings.String(entity.SettingThumbnailIconStyle),
				Size:  settings.Int(entity.SettingThumbnailIconSize),
			}
		},
		func() bool { return settings.Bool(entity.SettingUploadDryRun) },
		time.Now, log,
	)
	sched := scheduler.New(pools, store, runner, store, broker, log, scheduler.Config{
		RetryBase:      settings.Duration(entity.SettingTaskRetryBaseMillis),
		RetryMax:       settings.Duration(entity.SettingTaskRetryMaxMillis),
		SafetyInterval: 30 * time.Second,
	})
	expander.sched = sched

	// --- delivery -----------------------------------------------------------
	dist, err := web.Dist()
	if err != nil {
		log.Warn("web UI is not embedded", slog.String("error", err.Error()))
		dist = nil
	}
	handler, _ := deliveryhttp.NewRouter(deliveryhttp.Deps{
		Channels:      store,
		ChannelWriter: store,
		Videos:        store,
		VideoWriter:   store,
		VideoStates:   store,
		VideoFields:   store,
		Chapters:      store,
		ChapterFields: store,
		Assets:        store,
		Tasks:         store,
		Store:         assets,
		Settings:      settings,

		Submitter:  sched,
		Resumer:    sched,
		Requeuer:   sched,
		Expander:   sched,
		Canceller:  sched,
		Approver:   sched,
		Rejecter:   sched,
		Forgetter:  sched,
		TaskRetry:  sched,
		ChapRetry:  sched,
		Rerunner:   sched,
		StaleMark:  sched,
		StaleRun:   sched,
		StaleOK:    sched,
		Pools:      sched,
		Reporter:   sched,
		Prompts:    providers.PromptCache(),
		Notifier:   broker,
		Coalescer:  broker,
		Events:     broker,
		SSEClients: broker.Subscribers,

		LogLevel: level,
		Log:      log,
		NewID:    uuid.NewString,
		Now:      time.Now,
		Version:  version,
		Started:  started,
		Dist:     dist,
	})

	srv := &http.Server{
		Addr:              c.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: the SSE stream is deliberately long-lived.
		IdleTimeout: 120 * time.Second,
		BaseContext: func(net.Listener) context.Context { return context.WithoutCancel(ctx) },
	}
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", c.Listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", c.Listen, err)
	}

	// --- run ----------------------------------------------------------------
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return broker.Run(gctx) })
	g.Go(func() error { return sched.Run(gctx) })
	g.Go(func() error {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	g.Go(func() error {
		<-gctx.Done()
		shutdownCtx, done := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer done()
		return srv.Shutdown(shutdownCtx)
	})

	resumed, err := app.ResumeScheduler(ctx, store, sched, log)
	if err != nil {
		log.Error("failed to resume open videos", slog.String("error", err.Error()))
	}

	log.Info("yt-studio serving",
		slog.String("version", version),
		slog.String("addr", listener.Addr().String()),
		slog.String("db", c.DB),
		slog.String("assets", assets.Root()),
		slog.Int("resumed_videos", resumed),
		slog.Duration("startup", time.Since(started)))

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	log.Info("yt-studio stopped")
	return nil
}

// videoContextLookup gives the mock LLM the blueprint context its coalesced
// prompt call needs, without the provider itself touching the database.
// nineRouterContextLookup resolves a video id into the plan its slides
// illustrate. Only SlidePrompts needs it: the port hands that method an id and
// nothing else, and a provider may never read the database itself.
func nineRouterContextLookup(store *sqlite.Store) ninerouter.ContextLookup {
	return func(ctx context.Context, videoID entity.VideoID) (ninerouter.VideoContext, error) {
		v, err := store.VideoByID(ctx, videoID)
		if err != nil {
			return ninerouter.VideoContext{}, err
		}
		outline, err := chapterOutline(ctx, store, videoID)
		if err != nil {
			return ninerouter.VideoContext{}, err
		}
		return ninerouter.VideoContext{
			BlueprintOutline: provider.BlueprintOutline{
				Title: v.Title, Summary: v.Topic, Chapters: outline,
			},
			SlidesPerChapter: v.SlidesPerChapter,
		}, nil
	}
}

// chapterOutline projects a video's chapters, for the two context lookups that
// both need it.
func chapterOutline(
	ctx context.Context,
	store *sqlite.Store,
	videoID entity.VideoID,
) ([]provider.BlueprintChapter, error) {
	rows, err := store.ListChaptersByVideo(ctx, videoID)
	if err != nil {
		return nil, err
	}
	out := make([]provider.BlueprintChapter, 0, len(rows))
	for _, c := range rows {
		out = append(out, provider.BlueprintChapter{
			Ordinal:        c.Ordinal,
			Title:          c.Title,
			Summary:        c.Summary,
			EstimatedWords: c.EstimatedWords,
		})
	}
	return out, nil
}

func videoContextLookup(store *sqlite.Store) mockprovider.ContextLookup {
	return func(ctx context.Context, videoID entity.VideoID) (mockprovider.VideoContext, error) {
		v, err := store.VideoByID(ctx, videoID)
		if err != nil {
			return mockprovider.VideoContext{}, err
		}
		outline, err := chapterOutline(ctx, store, videoID)
		if err != nil {
			return mockprovider.VideoContext{}, err
		}
		return mockprovider.VideoContext{
			Ref:              v.Ref,
			Title:            v.Title,
			Topic:            v.Topic,
			Chapters:         outline,
			SlidesPerChapter: v.SlidesPerChapter,
		}, nil
	}
}

// lateExpander breaks the one cycle in the wiring: an ungated blueprint expands
// its own video's DAG, so the task runner needs the scheduler, and the
// scheduler is constructed from the task runner.
//
// The reference is written once during wiring, strictly before scheduler.Run
// starts the goroutines that read it, so the assignment happens-before every
// read without a lock.
type lateExpander struct{ sched *scheduler.Scheduler }

var _ app.GraphExpander = (*lateExpander)(nil)

func (e *lateExpander) Expand(ctx context.Context, videoID entity.VideoID, tail scheduler.Tail) error {
	return e.sched.Expand(ctx, videoID, tail)
}

func newLogger(startupLevel string) (*slog.LevelVar, *slog.Logger) {
	level := &slog.LevelVar{}
	if parsed, err := app.ParseLogLevel(startupLevel); err == nil {
		level.Set(parsed)
	}
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	log := slog.New(handler)
	slog.SetDefault(log)
	return level, log
}
