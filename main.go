// Command yt-studio turns a topic into a long-form slideshow video on YouTube.
package main

import (
	"context"
	"embed"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"

	"github.com/tbui/yt-studio/internal/desktop"
	"github.com/tbui/yt-studio/internal/domain/entity"
	"github.com/tbui/yt-studio/internal/home"
	"github.com/tbui/yt-studio/internal/services"
	"github.com/tbui/yt-studio/internal/studio"
	"github.com/tbui/yt-studio/internal/update"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is set by make release (-ldflags -X main.version); dev builds don't update.
var version = "dev"

const appName = "yt-studio"

func init() {
	// Registered events get typed TypeScript bindings.
	application.RegisterEvent[entity.Event](services.PipelineEvent)
	application.RegisterEvent[services.Resync](services.PipelineResyncEvent)
	application.RegisterEvent[services.ConsoleBatch](services.ConsoleEvent)
	application.RegisterEvent[services.AccessibilityOptions](services.AccessibilityEvent)
	application.RegisterEvent[services.UpdateState](services.UpdateEvent)
}

func main() {
	// An update restart relaunches this binary as the bundle-swapping helper.
	updater.HandleHelperMode()
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir, err := home.Default()
	if err != nil {
		return err
	}
	if err := dir.Ensure(); err != nil {
		return err
	}
	level, logger, closeLog, err := newLogger(dir.Log())
	if err != nil {
		return err
	}

	backend, err := studio.Open(ctx, dir, level, logger)
	if err != nil {
		return err
	}
	// On macOS quitting exits the process after OnShutdown, so teardown happens there.
	done := make(chan struct{})
	var stop sync.Once
	shutdown := func() {
		stop.Do(func() {
			cancel()
			<-done
			backend.Close()
			logger.Info("yt-studio stopped")
			closeLog()
		})
	}
	defer shutdown()

	app := application.New(application.Options{
		Name:        appName,
		Description: "Long-form slideshow videos, from topic to YouTube",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
			// Stored artifacts and operator resources are served from disk.
			Middleware: services.AssetMiddleware(backend.Store, backend.Assets, os.DirFS(dir.Resources())),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		MarshalError: services.MarshalError,
		OnShutdown:   shutdown,
		Logger:       logger,
	})
	platform := desktop.New(app, dir.Look())

	b := &services.Backend{
		Store:                 backend.Store,
		Scheduler:             backend.Scheduler,
		Assets:                backend.Assets,
		Settings:              backend.Settings,
		Uploader:              backend.Providers.Uploader(),
		UploadAuth:            backend.YouTube,
		Prompts:               backend.Providers.PromptCache(),
		Notifier:              backend.Broker,
		Coalescer:             backend.Broker,
		LogLevel:              level,
		Log:                   logger,
		NewID:                 backend.NewID,
		Now:                   backend.Now,
		SavePreparedBlueprint: backend.SavePreparedBlueprint,
	}
	app.RegisterService(application.NewService(services.NewChannelService(b)))
	app.RegisterService(application.NewService(services.NewVideoService(b)))
	app.RegisterService(application.NewService(services.NewChapterService(b)))
	app.RegisterService(application.NewService(services.NewTaskService(b)))
	app.RegisterService(application.NewService(services.NewThumbnailService(b)))
	app.RegisterService(application.NewService(services.NewSettingsService(b)))
	app.RegisterService(application.NewService(services.NewSchedulerService(b)))
	app.RegisterService(application.NewService(services.NewConsoleService(backend.Console, platform)))
	app.RegisterService(application.NewService(services.NewSystemService(platform, version, string(dir), func() services.AccessibilityOptions {
		return services.AccessibilityOptions(platform.Accessibility())
	})))
	platform.OnAccessibilityChange(func(a desktop.Accessibility) {
		platform.Emit(services.AccessibilityEvent, services.AccessibilityOptions(a))
	})
	updates := newUpdateService(app, dir.Releases(), platform, logger)
	app.RegisterService(application.NewService(updates))

	app.Menu.Set(appMenu())
	winOpts := mainWindowOptions()
	desktop.ApplyLook(desktop.ReadLook(dir.Look()), &winOpts)
	desktop.ApplySavedBounds(dir.Window(), &winOpts)
	win := app.Window.NewWithOptions(winOpts)
	desktop.RememberBounds(dir.Window(), win)
	desktop.MaybeRunSnapshots(app, win) // debug builds only (-tags snapshot)

	go func() {
		defer close(done)
		if err := backend.Run(ctx); err != nil {
			logger.Error("backend stopped", "err", err)
			app.Quit()
		}
	}()
	go services.ForwardPipeline(ctx, backend.Broker, platform)
	go services.RunUpdates(ctx, updates)
	logger.Info("yt-studio started", "version", version, "home", string(dir))

	return app.Run()
}

func appMenu() *application.Menu {
	menu := application.NewMenu()
	menu.AddRole(application.AppMenu)
	menu.AddRole(application.EditMenu)
	menu.AddRole(application.WindowMenu)
	return menu
}

func mainWindowOptions() application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Name:      "main",
		Title:     appName,
		Width:     1440,
		Height:    920,
		MinWidth:  960,
		MinHeight: 600,
		Mac: application.MacWindow{
			TitleBar:                application.MacTitleBarHiddenInsetUnified,
			InvisibleTitleBarHeight: 0,
			Backdrop:                application.MacBackdropLiquidGlass,
			LiquidGlass: application.MacLiquidGlass{
				Style:    application.LiquidGlassStyleAutomatic,
				Material: application.NSVisualEffectMaterialAuto,
			},
		},
		BackgroundType:   application.BackgroundTypeTransparent,
		BackgroundColour: application.NewRGBA(0, 0, 0, 0),
		URL:              "/",
	}
}

// newUpdateService follows releases in dir; dev builds and unbundled runs don't update.
func newUpdateService(app *application.App, dir string, emit services.Emitter, logger *slog.Logger) *services.UpdateService {
	var installer services.Installer
	exe, _ := os.Executable()
	if update.Valid(version) && strings.Contains(exe, ".app/Contents/MacOS/") {
		err := app.Updater.Init(updater.Config{
			CurrentVersion: version,
			Providers:      []updater.Provider{update.Source{Dir: dir}},
			Window:         updater.WindowNone,
		})
		if err != nil {
			logger.Warn("updates unavailable", "err", err)
		} else {
			installer = app.Updater
		}
	}
	updates := services.NewUpdateService(installer, version, emit, logger)
	if installer != nil {
		if _, err := services.WatchReleases(updates, dir); err != nil {
			logger.Warn("not watching for releases", "dir", dir, "err", err)
		}
	}
	return updates
}

// newLogger logs to stderr and to path, truncated each launch.
func newLogger(path string) (*slog.LevelVar, *slog.Logger, func(), error) {
	level := &slog.LevelVar{}
	if v := os.Getenv("YTS_LOG_LEVEL"); v != "" {
		_ = level.UnmarshalText([]byte(v))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, nil, fmt.Errorf("log directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("log file: %w", err)
	}
	logger := slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, file), &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
	return level, logger, func() { _ = file.Close() }, nil
}
