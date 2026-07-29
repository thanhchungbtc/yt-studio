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
	mockprovider "github.com/tbui/yt-studio/adapters/mock_provider"
	"github.com/tbui/yt-studio/adapters/sqlite"
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
// open. Everything else is a settings row (§3).
type bootstrap struct {
	DB       string `help:"SQLite database file." default:"var/yt-studio.db" env:"YTS_DB" type:"path"`
	Assets   string `help:"Content-addressed asset store root." default:"var/assets" env:"YTS_ASSETS" type:"path"`
	Listen   string `help:"Listen address." default:"127.0.0.1:8080" env:"YTS_LISTEN"`
	LogLevel string `help:"Startup log level; the settings table takes over once loaded." default:"info" env:"YTS_LOG_LEVEL" enum:"debug,info,warn,error"`
}

type serveCmd struct {
	bootstrap
}

type seedCmd struct {
	bootstrap
}

type versionCmd struct{}

type cli struct {
	Serve   serveCmd   `cmd:"" default:"withargs" help:"Run the daemon."`
	Seed    seedCmd    `cmd:"" help:"Apply migrations and write the default settings and channels, then exit."`
	Version versionCmd `cmd:"" help:"Print the version."`
}

func main() {
	var root cli
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

	// --- adapters -----------------------------------------------------------
	store, err := sqlite.Open(ctx, sqlite.Options{Path: c.DB}, log)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	// The writer goroutine has its own cancel so it outlives the request
	// context long enough to flush the scheduler's final transitions.
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
	if err := settings.Load(ctx); err != nil {
		return err
	}
	if parsed, err := app.ParseLogLevel(settings.String(entity.SettingLogLevel)); err == nil {
		level.Set(parsed)
	}

	assets, err := assetstore.New(c.Assets)
	if err != nil {
		return err
	}

	broker := eventbus.New(settings.Duration(entity.SettingSSECoalesceMillis), log)

	tuning := mockprovider.Tuning(func() (time.Duration, int) {
		return settings.Duration(entity.SettingMockLatencyMillis),
			settings.Int(entity.SettingMockFailureRatePercent)
	})
	llm := mockprovider.NewLLM(assets, videoContextLookup(store), tuning)
	tts := mockprovider.NewTTS(assets, tuning)
	images := mockprovider.NewImage(assets, tuning)
	composer := mockprovider.NewComposer(assets, tuning)
	uploader := mockprovider.NewUploader(assets, tuning, time.Now)

	// --- scheduler ----------------------------------------------------------
	pools, err := scheduler.NewPools(settings.PoolLimits())
	if err != nil {
		return err
	}
	runner := app.NewTaskRunner(
		store, store, store, store, store, store, store,
		assets, llm, tts, images, composer, uploader, broker,
		func() bool { return settings.Bool(entity.SettingUploadDryRun) },
		time.Now, log,
	)
	sched := scheduler.New(pools, store, runner, store, broker, log, scheduler.Config{
		RetryBase:      settings.Duration(entity.SettingTaskRetryBaseMillis),
		RetryMax:       settings.Duration(entity.SettingTaskRetryMaxMillis),
		SafetyInterval: 30 * time.Second,
	})

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
		Chapters:      store,
		ChapterFields: store,
		Assets:        store,
		Tasks:         store,
		TaskWriter:    store,
		Store:         assets,
		Settings:      settings,

		Submitter:  sched,
		Canceller:  sched,
		Approver:   sched,
		Rejecter:   sched,
		Forgetter:  sched,
		TaskRetry:  sched,
		ChapRetry:  sched,
		Pools:      sched,
		Reporter:   sched,
		Prompts:    llm,
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
// prompt call needs, without the provider itself touching the database (§4).
func videoContextLookup(store *sqlite.Store) mockprovider.ContextLookup {
	return func(ctx context.Context, videoID entity.VideoID) (mockprovider.VideoContext, error) {
		v, err := store.VideoByID(ctx, videoID)
		if err != nil {
			return mockprovider.VideoContext{}, err
		}
		ch, err := store.ChannelByID(ctx, v.ChannelID)
		if err != nil {
			return mockprovider.VideoContext{}, err
		}
		rows, err := store.ListChaptersByVideo(ctx, videoID)
		if err != nil {
			return mockprovider.VideoContext{}, err
		}
		outline := make([]provider.BlueprintChapter, 0, len(rows))
		for _, c := range rows {
			outline = append(outline, provider.BlueprintChapter{
				Ordinal: c.Ordinal,
				Title:   c.Title,
				Summary: c.Summary,
			})
		}
		return mockprovider.VideoContext{
			Ref:              v.Ref,
			Title:            v.Title,
			Topic:            v.Topic,
			Style:            ch.Style,
			Chapters:         outline,
			ImagesPerChapter: v.ImagesPerChapter,
		}, nil
	}
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
