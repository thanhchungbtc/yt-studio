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
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/alecthomas/kong"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/tbui/yt-studio/adapters/assetstore"
	"github.com/tbui/yt-studio/adapters/eventbus"
	"github.com/tbui/yt-studio/adapters/provider/ffmpeg"
	"github.com/tbui/yt-studio/adapters/provider/ninerouter"
	"github.com/tbui/yt-studio/adapters/provider/runware"
	"github.com/tbui/yt-studio/adapters/provider/sample"
	"github.com/tbui/yt-studio/adapters/provider/thumbnail"
	"github.com/tbui/yt-studio/adapters/provider/tts/kokoro"
	"github.com/tbui/yt-studio/adapters/provider/tts/xtts"
	"github.com/tbui/yt-studio/adapters/provider/youtube"
	"github.com/tbui/yt-studio/adapters/sqlite"
	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/cmd/server/internal/registry"
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
// open: where the data lives, and how to reach it. Everything else — every
// credential, every endpoint, every knob — is a settings row, edited on the
// settings screen.
//
// One directory rather than four paths. The database, the asset store, the
// resources and the transcripts are one installation, and a desktop app that
// asked an operator to place four of them separately would be asking a question
// with only one sensible answer. `--home var` is what reproduces the layout the
// repository has always used, which is what `make dev` passes.
type bootstrap struct {
	//nolint:lll // one flag, one line
	Home string `help:"Installation directory: db/, assets/, resources/, transcripts/ and log/ live under it." default:"~/.yt-studio" env:"YTS_HOME" type:"path"`
	//nolint:lll // one flag, one line
	Listen string `help:"Listen address. Empty picks a free port, which is what the desktop shell wants." default:"127.0.0.1:8080" env:"YTS_LISTEN"`
	//nolint:lll // one flag, one line
	LogLevel string `help:"Startup log level; the settings table takes over once loaded." default:"info" env:"YTS_LOG_LEVEL" enum:"debug,info,warn,error"`
}

// The layout under Home, which is one directory of directories:
//
//	~/.yt-studio/
//	    db/           yt-studio.db, and the -wal and -shm SQLite keeps beside it
//	    assets/       the content-addressed store
//	    resources/    operator-supplied media: chalkboard.jpg, bg.mp4, fonts/
//	    credentials/  one directory per channel slug: credentials.json, token.json
//	    transcripts/  one file per LLM exchange
//	    log/          server.log, rewritten each run
//
// The database has a directory of its own because it is three files rather than
// one: SQLite writes -wal and -shm beside it, they appear and disappear with the
// connection, and loose at the root they read as debris. Nothing but directories
// at the top means a listing says what the installation is made of.
//
// Named here rather than spelled at each use, so the one place that decides
// where a thing lives is this block.
func (b bootstrap) db() string          { return filepath.Join(b.Home, "db", "yt-studio.db") }
func (b bootstrap) assets() string      { return filepath.Join(b.Home, "assets") }
func (b bootstrap) resources() string   { return filepath.Join(b.Home, "resources") }
func (b bootstrap) transcripts() string { return filepath.Join(b.Home, "transcripts") }

// credentials is the root of the per-channel OAuth directories: one directory
// per channel slug, holding the client the operator downloaded and the token
// the authorization flow writes beside it.
//
// A path here rather than a settings row, unlike every other endpoint and key,
// because these are files an operator places by hand — the same reason
// resources/ is a directory and not a row. It is under Home so that one
// installation is still one directory, and so the 0700 above covers it.
func (b bootstrap) credentials() string { return filepath.Join(b.Home, "credentials") }

// ensureHome creates the installation directory before anything writes inside
// it, and does so at 0700.
//
// The permission is the point. The settings table holds API keys now, so this
// directory is the credential store; every path below it inherits the fact that
// nobody else can traverse in. It runs before the logger, which would otherwise
// create the same directory at 0755 on its way to the log file.
//
// An existing directory is left as it is, permissions included: quietly
// tightening a directory an operator placed and shared is not this program's
// call to make.
// The resources directory is made here too, empty, because nothing else ever
// makes it: the other four are written to and appear on their own, but this one
// is only ever read, so on a fresh installation it would be missing exactly when
// somebody needs to be told where to put a background video.
//
// The credentials directory is made for that same reason and at 0700 rather
// than 0755: nothing writes into it until an authorization succeeds, so on a
// fresh installation it would be missing exactly when somebody needs to be told
// where to put a credentials.json, and what lands in it afterwards is a client
// secret and a refresh token.
func (b bootstrap) ensureHome() error {
	if err := os.MkdirAll(b.Home, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", b.Home, err)
	}
	if err := os.MkdirAll(b.resources(), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", b.resources(), err)
	}
	if err := os.MkdirAll(b.credentials(), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", b.credentials(), err)
	}
	return nil
}

// logFile is where serve mirrors its log. A bundled application has no terminal
// attached, so stderr goes nowhere an operator can read; this is the only record
// of why a backend reported itself unavailable. The short-lived commands do not
// use it — they are run from a terminal, and clobbering the server's log to say
// what a sweep freed would be a poor trade.
// The name is the process, not the product: inside ~/.yt-studio/log/ the
// product name says nothing, and it leaves the obvious room beside it if the
// desktop window ever needs one of its own.
func (b bootstrap) logFile() string { return filepath.Join(b.Home, "log", "server.log") }

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
	Force bool `help:"Sweep even when the database references no assets at all, which normally means --home is wrong." default:"false"`
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
	// Before anything is wired, because the composer resolves ffmpeg through it.
	widenPath()
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

	_, log, _, err := newLogger(c.LogLevel, "")
	if err != nil {
		return err
	}
	if err := c.ensureHome(); err != nil {
		return err
	}

	store, err := sqlite.Open(ctx, sqlite.Options{Path: c.db()}, log)
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

// Run reclaims store files that nothing in the database references. It repairs
// asset ownership first, unconditionally: a file reachable only through a
// chapter's id list has no owning row until then, and would read as garbage.
func (c *sweepCmd) Run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	_, log, _, err := newLogger(c.LogLevel, "")
	if err != nil {
		return err
	}
	if err := c.ensureHome(); err != nil {
		return err
	}

	store, err := sqlite.Open(ctx, sqlite.Options{Path: c.db()}, log)
	if err != nil {
		return err
	}
	g, gctx := errgroup.WithContext(ctx)
	writerCtx, stopWriter := context.WithCancel(gctx)
	g.Go(func() error { return store.Run(writerCtx) })

	err = sweep(gctx, store, c.assets(), c.Apply, c.Force, log)
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

	if err := c.ensureHome(); err != nil {
		return err
	}

	level, log, closeLog, err := newLogger(c.LogLevel, c.logFile())
	if err != nil {
		return err
	}
	defer closeLog()

	// --- adapters -----------------------------------------------------------
	store, err := sqlite.Open(ctx, sqlite.Options{Path: c.db()}, log)
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

	assets, err := assetstore.New(c.assets())
	if err != nil {
		return err
	}

	// A delete decides what it may reclaim from the ownership rows, so they must
	// exist before one can be served. Normally this finds nothing.
	if repaired, err := app.RepairAssetOwnership(ctx, store, store, store, assets, time.Now().UTC(), log); err != nil {
		return fmt.Errorf("repair asset ownership: %w", err)
	} else if repaired > 0 {
		log.Info("reconstructed asset ownership rows", slog.Int("rows", repaired))
	}

	// Backends are registered before the settings are loaded, so a row naming one
	// that does not exist fails at startup rather than at first task.
	ffmpegComposer := ffmpeg.New(assets, c.resources(), log)
	thumbnails := thumbnail.New(assets, c.resources(), func() thumbnail.Options {
		return thumbnail.Options{
			Font: settings.String(entity.SettingThumbnailFont),
			Rows: settings.Int(entity.SettingThumbnailGridRows),
		}
	}, log)
	samples := sample.NewLibrary(c.resources())

	// Closures rather than captured values: settings load after registration, so
	// nothing here has a value yet, and an address or a key entered on the screen
	// applies to the next generation rather than the next restart. That is also
	// why none of these constructors validates an address — an empty one is what
	// they would see, and the real check belongs to Check and to the request.
	nineRouter, err := ninerouter.New(ninerouter.Config{
		BaseURL:       func() string { return settings.String(entity.SettingNineRouterURL) },
		APIKey:        func() string { return settings.String(entity.SettingNineRouterKey) },
		Model:         func() string { return settings.String(entity.SettingNineRouterModel) },
		TranscriptDir: c.transcripts(),
	}, assets, nineRouterContextLookup(store))
	if err != nil {
		return err
	}

	runwareClient, err := runware.New(runware.Config{
		APIKey: func() string { return settings.String(entity.SettingRunwareKey) },
		Model:  func() string { return settings.String(entity.SettingRunwareModel) },
		SlideSize: func() (int, int) {
			return settings.Int(entity.SettingRunwareWidth), settings.Int(entity.SettingRunwareHeight)
		},
	}, assets, log)
	if err != nil {
		return err
	}

	// Only this backend's own knobs: how a chapter should sound arrives on the
	// request, from app.NarrationOptions below.
	xttsClient, err := xtts.New(xtts.Config{
		BaseURL: func() string { return settings.String(entity.SettingXTTSURL) },
		Options: func() xtts.Options {
			return xtts.Options{
				ChunkMinChars:      settings.Int(entity.SettingXTTSChunkMinChars),
				ChunkSilenceMillis: settings.Int(entity.SettingXTTSChunkSilenceMillis),
			}
		},
	}, assets)
	if err != nil {
		return err
	}

	// Kokoro has no knobs of its own: it speaks a chapter in one request, so the
	// chunking the backend above is configured for has nothing to configure here.
	kokoroClient, err := kokoro.New(kokoro.Config{
		BaseURL: func() string { return settings.String(entity.SettingKokoroURL) },
		APIKey:  func() string { return settings.String(entity.SettingKokoroKey) },
		Model:   func() string { return settings.String(entity.SettingKokoroModel) },
	}, assets)
	if err != nil {
		return err
	}

	// One client for both halves of publishing: the port the scheduler uploads
	// through, and the port the dialog authorizes through.
	youtubeClient := youtube.New(c.credentials(), assets, log)

	providers := registry.New(settings.String)
	providers.RegisterLLM("sample", sample.NewLLM(assets, videoContextLookup(store)))
	providers.RegisterLLM("9router", nineRouter)
	providers.RegisterTTS("sample", sample.NewTTS(samples, assets))
	providers.RegisterTTS("xtts", xttsClient)
	providers.RegisterTTS("kokoro", kokoroClient)
	providers.RegisterSlide("sample", sample.NewSlide(samples, assets))
	providers.RegisterSlide("runware", runware.NewSlide(runwareClient))
	providers.RegisterComposer("sample", sample.NewComposer(samples, assets))
	providers.RegisterComposer("ffmpeg", ffmpegComposer)
	providers.RegisterThumbnail("builtin", thumbnails)
	providers.RegisterThumbnailIcon("sample", sample.NewIcon(samples, assets))
	providers.RegisterThumbnailIcon("runware", runware.NewIcon(runwareClient))
	providers.RegisterUploader("sample", sample.NewUploader(assets, time.Now,
		func() int { return settings.Int(entity.SettingUploadSampleMegabytesPerSecond) }))
	providers.RegisterUploader("youtube", youtubeClient)

	settings.Constrain(providers.Options())
	// By assignment rather than a literal: a map keyed by a settings key is
	// checked for exhaustiveness, and only a few rows have a shortlist.
	suggestions := make(map[entity.SettingKey][]entity.SettingSuggestion, 1)
	suggestions[entity.SettingRunwareModel] = modelSuggestions(runware.Models())
	suggestions[entity.SettingThumbnailFont] = fontSuggestions(thumbnail.Fonts(c.resources()))
	settings.Suggest(suggestions)
	if err := settings.Load(ctx); err != nil {
		return err
	}
	// Every preset is proved against what was just registered, so one naming a
	// renamed backend fails here rather than when somebody clicks it.
	if err := app.CheckPresets(settings); err != nil {
		return err
	}
	if parsed, err := app.ParseLogLevel(settings.String(entity.SettingLogLevel)); err == nil {
		level.Set(parsed)
	}

	broker := eventbus.New(settings.Duration(entity.SettingSSECoalesceMillis), log)

	if err := ffmpegComposer.Check(); err != nil {
		log.Info("ffmpeg composer is not available",
			slog.String("reason", err.Error()),
			slog.String("resources", c.resources()))
	}
	if err := thumbnails.Check(); err != nil {
		log.Info("the built-in thumbnail renderer is not available",
			slog.String("reason", err.Error()),
			slog.String("resources", c.resources()))
	}
	if err := samples.Check(); err != nil {
		log.Info("sample backends are not available",
			slog.String("reason", err.Error()),
			slog.String("dir", samples.Dir()))
	}
	if err := nineRouter.Check(ctx); err != nil {
		log.Info("9router is not available",
			slog.String("reason", err.Error()),
			slog.String("url", nineRouter.BaseURL()))
	} else {
		log.Info("9router is available",
			slog.String("url", nineRouter.BaseURL()),
			slog.String("model", nineRouter.Model()),
			slog.String("transcripts", c.transcripts()))
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
	if err := xttsClient.Check(ctx); err != nil {
		log.Info("xtts narration is not available",
			slog.String("reason", err.Error()),
			slog.String("url", xttsClient.BaseURL()))
	} else {
		log.Info("xtts narration is available",
			slog.String("url", xttsClient.BaseURL()),
			slog.String("voice", settings.String(entity.SettingXTTSVoice)),
			slog.Float64("speed", settings.Float(entity.SettingXTTSSpeed)))
	}
	// The voice is passed rather than resolved: this backend can say whether the
	// server offers it, which is the difference between finding out now and
	// finding out on the first chapter.
	if err := kokoroClient.Check(ctx, settings.String(entity.SettingKokoroVoice)); err != nil {
		log.Info("kokoro narration is not available",
			slog.String("reason", err.Error()),
			slog.String("url", kokoroClient.BaseURL()))
	} else {
		log.Info("kokoro narration is available",
			slog.String("url", kokoroClient.BaseURL()),
			slog.String("model", kokoroClient.Model()),
			slog.String("voice", settings.String(entity.SettingKokoroVoice)),
			slog.Float64("speed", settings.Float(entity.SettingKokoroSpeed)))
	}

	// The channels table's credentials field is a copy of what the credentials
	// directory holds, and the directory can change while this program is not
	// running: an operator placing a token by hand, a grant revoked from
	// Google's side. Reconciled once here, so the upload gate is reading
	// something true by the time anything can reach it.
	if err := app.ReconcileCredentials(ctx, store, store, youtubeClient, time.Now(), log); err != nil {
		return err
	}
	log.Info("youtube publishing is configured per channel",
		slog.String("credentials", youtubeClient.Root()))

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
				ChapterTolerancePercent: settings.Int(entity.SettingBlueprintChapterTolerancePercent),
				MaxAttempts:             settings.Int(entity.SettingTaskMaxAttempts),
				UploadGate:              settings.GateEnabled(entity.GateUpload),
			}
		},
		func() app.NarrationOptions { return narrationOptions(settings) },
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
		AssetWriter:   store,
		Tasks:         store,
		Store:         assets,
		Settings:      settings,
		UploadAuth:    youtubeClient,

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
		// The same background and typefaces the builtin renderer draws with, so
		// the browser editor composes what the operator will actually get.
		Resources: os.DirFS(c.resources()),
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
		slog.String("db", c.db()),
		slog.String("assets", assets.Root()),
		slog.Int("resumed_videos", resumed),
		slog.Duration("startup", time.Since(started)))

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	log.Info("yt-studio stopped")
	return nil
}

// modelSuggestions adapts a backend's shortlist to the settings type. The
// backend names its own checkpoints and knows nothing of the settings table;
// the mapping between the two is wiring, so it happens here.
func modelSuggestions(models []runware.Model) []entity.SettingSuggestion {
	out := make([]entity.SettingSuggestion, 0, len(models))
	for _, m := range models {
		out = append(out, entity.SettingSuggestion{Value: m.AIR, Label: m.Name})
	}
	return out
}

// fontSuggestions labels each typeface with its name rather than its filename:
// the row stores `CabinSketch-Bold.ttf` because that is what the renderer opens,
// and "Cabin Sketch Bold" is what an operator is choosing between.
func fontSuggestions(files []string) []entity.SettingSuggestion {
	out := make([]entity.SettingSuggestion, 0, len(files))
	for _, name := range files {
		out = append(out, entity.SettingSuggestion{Value: name, Label: fontLabel(name)})
	}
	return out
}

// fontLabel turns a filename into the face it holds: drop the extension, and
// split the CamelCase and hyphenated halves foundries write filenames in.
func fontLabel(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	base = strings.NewReplacer("-", " ", "_", " ").Replace(base)
	var b strings.Builder
	b.Grow(len(base) + 4)
	for i, r := range base {
		if i > 0 && unicode.IsUpper(r) && !unicode.IsUpper(rune(base[i-1])) && base[i-1] != ' ' {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// narrationOptions reads how a chapter should sound from the rows belonging to
// the narration backend currently selected.
//
// Which rows those are is the one question that has to be answered by name, and
// this is the only place that may: a use case must not know that `xtts` exists,
// and a backend reading its own rows could never be given a channel's voice
// instead. Resolving by prefix here keeps both true, and a key the running
// build does not seed reads as empty — which for a voice already means "let the
// server pick".
func narrationOptions(settings *service.Settings) app.NarrationOptions {
	backend := settings.String(entity.SettingProviderTTS)
	return app.NarrationOptions{
		Voice:    settings.String(entity.SettingKey(backend + ".voice")),
		Language: settings.String(entity.SettingKey(backend + ".language")),
		Speed:    settings.Float(entity.SettingKey(backend + ".speed")),
	}
}

// videoContextLookup gives the sample LLM the blueprint context its coalesced
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

func videoContextLookup(store *sqlite.Store) sample.ContextLookup {
	return func(ctx context.Context, videoID entity.VideoID) (sample.VideoContext, error) {
		v, err := store.VideoByID(ctx, videoID)
		if err != nil {
			return sample.VideoContext{}, err
		}
		outline, err := chapterOutline(ctx, store, videoID)
		if err != nil {
			return sample.VideoContext{}, err
		}
		return sample.VideoContext{
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

// newLogger builds the logger, optionally mirroring it to a file.
//
// An empty path means stderr alone, which is what the short-lived commands
// want. A path is written as well as stderr, not instead of it: `make
// dev:desktop` runs from a terminal and should still print.
//
// The file is truncated at startup rather than rotated. One run's worth is what
// answers "why did that not render", and a desktop application that quietly
// grew a log forever would be a worse bug than the one it was kept for.
func newLogger(startupLevel, path string) (*slog.LevelVar, *slog.Logger, func(), error) {
	level := &slog.LevelVar{}
	if parsed, err := app.ParseLogLevel(startupLevel); err == nil {
		level.Set(parsed)
	}

	var out io.Writer = os.Stderr
	closeLog := func() {}
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, nil, nil, fmt.Errorf("log directory: %w", err)
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("log file: %w", err)
		}
		out = io.MultiWriter(os.Stderr, file)
		closeLog = func() { _ = file.Close() }
	}

	log := slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)
	return level, log, closeLog, nil
}
