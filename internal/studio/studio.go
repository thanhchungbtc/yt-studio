// Package studio wires yt-studio's backend: store, providers, scheduler, events.
package studio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/tbui/yt-studio/internal/adapters/assetstore"
	"github.com/tbui/yt-studio/internal/adapters/eventbus"
	"github.com/tbui/yt-studio/internal/adapters/llmlog"
	"github.com/tbui/yt-studio/internal/adapters/provider/ffmpeg"
	"github.com/tbui/yt-studio/internal/adapters/provider/ninerouter"
	"github.com/tbui/yt-studio/internal/adapters/provider/runware"
	"github.com/tbui/yt-studio/internal/adapters/provider/sample"
	"github.com/tbui/yt-studio/internal/adapters/provider/thumbnail"
	"github.com/tbui/yt-studio/internal/adapters/provider/tts/kokoro"
	"github.com/tbui/yt-studio/internal/adapters/provider/tts/xtts"
	"github.com/tbui/yt-studio/internal/adapters/provider/youtube"
	"github.com/tbui/yt-studio/internal/adapters/sqlite"
	"github.com/tbui/yt-studio/internal/app"
	"github.com/tbui/yt-studio/internal/domain/entity"
	"github.com/tbui/yt-studio/internal/domain/scheduler"
	"github.com/tbui/yt-studio/internal/domain/service"
	"github.com/tbui/yt-studio/internal/home"
	"github.com/tbui/yt-studio/internal/registry"
)

// Studio is the running backend.
type Studio struct {
	Home      home.Dir
	Store     *sqlite.Store
	Assets    *assetstore.FS
	Settings  *service.Settings
	Providers *registry.Registry
	YouTube   *youtube.Client
	Scheduler *scheduler.Scheduler
	Broker    *eventbus.Broker
	Console   *llmlog.Broker
	LogLevel  *slog.LevelVar
	Log       *slog.Logger
	NewID     func() string
	Now       func() time.Time

	stopWriter context.CancelFunc
	writer     *errgroup.Group
}

// Open builds the backend. Nothing runs until Run.
//
//nolint:funlen // the wiring reads top to bottom
func Open(ctx context.Context, dir home.Dir, level *slog.LevelVar, log *slog.Logger) (*Studio, error) {
	widenPath()

	store, err := sqlite.Open(ctx, sqlite.Options{Path: dir.DB()}, log)
	if err != nil {
		return nil, err
	}
	s := &Studio{
		Home: dir, Store: store, Console: llmlog.New(0),
		LogLevel: level, Log: log, NewID: uuid.NewString, Now: time.Now,
	}
	// The writer outlives ctx so the scheduler's last transitions flush.
	writerCtx, stopWriter := context.WithCancel(context.WithoutCancel(ctx))
	s.stopWriter = stopWriter
	s.writer, _ = errgroup.WithContext(context.Background())
	s.writer.Go(func() error { return store.Run(writerCtx) })

	ok := false
	defer func() {
		if !ok {
			s.Close()
		}
	}()

	if err := sqlite.SeedSettings(ctx, store); err != nil {
		return nil, err
	}
	if err := sqlite.SeedChannels(ctx, store, time.Now().UTC()); err != nil {
		return nil, err
	}
	settings := service.NewSettings(store, store)
	s.Settings = settings

	assets, err := assetstore.New(dir.Assets())
	if err != nil {
		return nil, err
	}
	s.Assets = assets
	// A delete decides what it may reclaim from the ownership rows.
	if repaired, err := app.RepairAssetOwnership(ctx, store, store, store, assets, time.Now().UTC(), log); err != nil {
		return nil, fmt.Errorf("repair asset ownership: %w", err)
	} else if repaired > 0 {
		log.Info("reconstructed asset ownership rows", slog.Int("rows", repaired))
	}

	// Closures, so a settings edit applies to the next generation.
	composer := ffmpeg.New(assets, dir.Resources(), log)
	thumbnails := thumbnail.New(assets, dir.Resources(), func() thumbnail.Options {
		return thumbnail.Options{
			Font:       settings.String(entity.SettingThumbnailFont),
			Rows:       settings.Int(entity.SettingThumbnailGridRows),
			Weight:     settings.Float(entity.SettingThumbnailHeadlineWeight),
			MinorWords: settings.String(entity.SettingThumbnailHeadlineMinorWords),
		}
	}, log)
	samples := sample.NewLibrary(dir.Resources())

	nineRouter, err := ninerouter.New(ninerouter.Config{
		BaseURL:       func() string { return settings.String(entity.SettingNineRouterURL) },
		APIKey:        func() string { return settings.String(entity.SettingNineRouterKey) },
		Model:         modelFor(settings),
		TranscriptDir: dir.Transcripts(),
		Observe:       s.Console.Observe,
	}, assets, nineRouterContextLookup(store))
	if err != nil {
		return nil, err
	}
	runwareClient, err := runware.New(runware.Config{
		APIKey: func() string { return settings.String(entity.SettingRunwareKey) },
		Model:  func() string { return settings.String(entity.SettingRunwareModel) },
		SlideSize: func() (int, int) {
			return settings.Int(entity.SettingRunwareWidth), settings.Int(entity.SettingRunwareHeight)
		},
	}, assets, log)
	if err != nil {
		return nil, err
	}
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
		return nil, err
	}
	kokoroClient, err := kokoro.New(kokoro.Config{
		BaseURL: func() string { return settings.String(entity.SettingKokoroURL) },
		APIKey:  func() string { return settings.String(entity.SettingKokoroKey) },
		Model:   func() string { return settings.String(entity.SettingKokoroModel) },
	}, assets)
	if err != nil {
		return nil, err
	}
	s.YouTube = youtube.New(dir.Credentials(), assets, log)

	providers := registry.New(settings.String)
	providers.RegisterLLM("sample", sample.NewLLM(assets, videoContextLookup(store)))
	providers.RegisterLLM("9router", nineRouter)
	providers.RegisterTTS("sample", sample.NewTTS(samples, assets))
	providers.RegisterTTS("xtts", xttsClient)
	providers.RegisterTTS("kokoro", kokoroClient)
	providers.RegisterSlide("sample", sample.NewSlide(samples, assets))
	providers.RegisterSlide("runware", runware.NewSlide(runwareClient))
	providers.RegisterComposer("sample", sample.NewComposer(samples, assets))
	providers.RegisterComposer("ffmpeg", composer)
	providers.RegisterThumbnail("builtin", thumbnails)
	providers.RegisterThumbnailIcon("sample", sample.NewIcon(samples, assets))
	providers.RegisterThumbnailIcon("runware", runware.NewIcon(runwareClient))
	providers.RegisterUploader("sample", sample.NewUploader(assets, time.Now,
		func() int { return settings.Int(entity.SettingUploadSampleMegabytesPerSecond) }))
	providers.RegisterUploader("youtube", s.YouTube)
	s.Providers = providers

	settings.Constrain(providers.Options())
	suggestions := make(map[entity.SettingKey][]entity.SettingSuggestion, 2)
	suggestions[entity.SettingRunwareModel] = modelSuggestions(runware.Models())
	suggestions[entity.SettingThumbnailFont] = fontSuggestions(thumbnail.Fonts(dir.Resources()))
	settings.Suggest(suggestions)
	if err := settings.Load(ctx); err != nil {
		return nil, err
	}
	if err := app.CheckPresets(settings); err != nil {
		return nil, err
	}
	if parsed, err := app.ParseLogLevel(settings.String(entity.SettingLogLevel)); err == nil {
		level.Set(parsed)
	}

	s.Broker = eventbus.New(settings.Duration(entity.SettingSSECoalesceMillis), log)

	// Availability is only reported; it is fixed on the settings screen.
	go reportBackends(context.WithoutCancel(ctx), log, dir, settings, composer, thumbnails, samples,
		nineRouter, runwareClient, xttsClient, kokoroClient)

	// The credentials directory can change while the app is closed.
	if err := app.ReconcileCredentials(ctx, store, store, s.YouTube, time.Now(), log); err != nil {
		return nil, err
	}

	pools, err := scheduler.NewPools(settings.PoolLimits())
	if err != nil {
		return nil, err
	}
	// The runner and the scheduler need each other.
	expander := &lateExpander{}
	runner := app.NewTaskRunner(
		store, store, store, store, store, store, store,
		assets, providers.LLM(), providers.TTS(), providers.Slide(),
		providers.Composer(), providers.Thumbnail(), providers.ThumbnailIcon(),
		providers.Uploader(), s.Broker, s.Console.Observe,
		expander, s.loadPreparedBlueprint,
		func() app.BlueprintOptions {
			return app.BlueprintOptions{
				ChapterTolerancePercent: settings.Int(entity.SettingBlueprintChapterTolerancePercent),
				MaxAttempts:             settings.Int(entity.SettingTaskMaxAttempts),
				ScriptGate:              settings.GateEnabled(entity.GateScript),
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
	s.Scheduler = scheduler.New(pools, store, runner, store, s.Broker, log, scheduler.Config{
		RetryBase:      settings.Duration(entity.SettingTaskRetryBaseMillis),
		RetryMax:       settings.Duration(entity.SettingTaskRetryMaxMillis),
		SafetyInterval: 30 * time.Second,
	})
	expander.sched = s.Scheduler

	ok = true
	return s, nil
}

// Run runs the broker and scheduler and resumes open videos until ctx ends.
func (s *Studio) Run(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return s.Broker.Run(gctx) })
	g.Go(func() error { return s.Scheduler.Run(gctx) })
	resumed, err := app.ResumeScheduler(ctx, s.Store, s.Scheduler, s.Log)
	if err != nil {
		s.Log.Error("failed to resume open videos", slog.String("error", err.Error()))
	} else if resumed > 0 {
		s.Log.Info("resumed open videos", slog.Int("videos", resumed))
	}
	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// Close flushes the writer and closes the store. Call it after Run returns.
func (s *Studio) Close() {
	s.stopWriter()
	_ = s.writer.Wait()
	_ = s.Store.Close()
}

// SavePreparedBlueprint stores an outline for its video's blueprint task.
func (s *Studio) SavePreparedBlueprint(ref entity.Ref, raw json.RawMessage) error {
	return os.WriteFile(s.blueprintFile(ref), raw, 0o600)
}

// loadPreparedBlueprint is nil for the ordinary video, which plans its own.
func (s *Studio) loadPreparedBlueprint(ref entity.Ref) json.RawMessage {
	raw, err := os.ReadFile(s.blueprintFile(ref))
	if err != nil {
		return nil
	}
	return raw
}

func (s *Studio) blueprintFile(ref entity.Ref) string {
	return filepath.Join(s.Home.Blueprints(), string(ref)+".json")
}
