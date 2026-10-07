package studio

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/tbui/yt-studio/internal/adapters/provider/ffmpeg"
	"github.com/tbui/yt-studio/internal/adapters/provider/ninerouter"
	"github.com/tbui/yt-studio/internal/adapters/provider/runware"
	"github.com/tbui/yt-studio/internal/adapters/provider/sample"
	"github.com/tbui/yt-studio/internal/adapters/provider/thumbnail"
	"github.com/tbui/yt-studio/internal/adapters/provider/tts/kokoro"
	"github.com/tbui/yt-studio/internal/adapters/provider/tts/xtts"
	"github.com/tbui/yt-studio/internal/adapters/sqlite"
	"github.com/tbui/yt-studio/internal/app"
	"github.com/tbui/yt-studio/internal/domain/entity"
	"github.com/tbui/yt-studio/internal/domain/provider"
	"github.com/tbui/yt-studio/internal/domain/scheduler"
	"github.com/tbui/yt-studio/internal/domain/service"
	"github.com/tbui/yt-studio/internal/home"
)

// reportBackends logs which backends can serve right now.
func reportBackends(
	ctx context.Context,
	log *slog.Logger,
	dir home.Dir,
	settings *service.Settings,
	composer *ffmpeg.Composer,
	thumbnails *thumbnail.Renderer,
	samples *sample.Library,
	nineRouter *ninerouter.Client,
	runwareClient *runware.Client,
	xttsClient *xtts.Client,
	kokoroClient *kokoro.Client,
) {
	if err := composer.Check(); err != nil {
		log.Info("ffmpeg composer is not available",
			slog.String("reason", err.Error()), slog.String("resources", dir.Resources()))
	}
	if err := thumbnails.Check(); err != nil {
		log.Info("the built-in thumbnail renderer is not available",
			slog.String("reason", err.Error()), slog.String("resources", dir.Resources()))
	}
	if err := samples.Check(); err != nil {
		log.Info("sample backends are not available",
			slog.String("reason", err.Error()), slog.String("dir", samples.Dir()))
	}
	if err := nineRouter.Check(ctx); err != nil {
		log.Info("9router is not available",
			slog.String("reason", err.Error()), slog.String("url", nineRouter.BaseURL()))
	} else {
		log.Info("9router is available",
			slog.String("url", nineRouter.BaseURL()), slog.String("model", nineRouter.Model()))
	}
	if err := runwareClient.Check(); err != nil {
		log.Info("runware image backends are not available", slog.String("reason", err.Error()))
	} else {
		log.Info("runware image backends are available", slog.String("model", runwareClient.Model()))
	}
	if err := xttsClient.Check(ctx); err != nil {
		log.Info("xtts narration is not available",
			slog.String("reason", err.Error()), slog.String("url", xttsClient.BaseURL()))
	} else {
		log.Info("xtts narration is available", slog.String("url", xttsClient.BaseURL()))
	}
	if err := kokoroClient.Check(ctx, settings.String(entity.SettingKokoroVoice)); err != nil {
		log.Info("kokoro narration is not available",
			slog.String("reason", err.Error()), slog.String("url", kokoroClient.BaseURL()))
	} else {
		log.Info("kokoro narration is available", slog.String("url", kokoroClient.BaseURL()))
	}
}

func modelSuggestions(models []runware.Model) []entity.SettingSuggestion {
	out := make([]entity.SettingSuggestion, 0, len(models))
	for _, m := range models {
		out = append(out, entity.SettingSuggestion{Value: m.AIR, Label: m.Name})
	}
	return out
}

func fontSuggestions(files []string) []entity.SettingSuggestion {
	out := make([]entity.SettingSuggestion, 0, len(files))
	for _, name := range files {
		out = append(out, entity.SettingSuggestion{Value: name, Label: fontLabel(name)})
	}
	return out
}

// fontLabel is a font file's face name: "CabinSketch-Bold.ttf" is "Cabin Sketch Bold".
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

// narrationOptions reads the selected narration backend's rows.
func narrationOptions(settings *service.Settings) app.NarrationOptions {
	backend := settings.String(entity.SettingProviderTTS)
	return app.NarrationOptions{
		Voice:    settings.String(entity.SettingKey(backend + ".voice")),
		Language: settings.String(entity.SettingKey(backend + ".language")),
		Speed:    settings.Float(entity.SettingKey(backend + ".speed")),
	}
}

// modelFor picks the model for a kind of generation, falling back to the default.
func modelFor(settings *service.Settings) func(kind string) string {
	overrides := map[string]entity.SettingKey{
		ninerouter.KindBlueprint:     entity.SettingModelBlueprint,
		ninerouter.KindScript:        entity.SettingModelScript,
		ninerouter.KindSlidePrompts:  entity.SettingModelSlidePrompts,
		ninerouter.KindMetadata:      entity.SettingModelMetadata,
		ninerouter.KindThumbnailPlan: entity.SettingModelThumbnailPlan,
	}
	return func(kind string) string {
		if key, ok := overrides[kind]; ok {
			if model := strings.TrimSpace(settings.String(key)); model != "" {
				return model
			}
		}
		return settings.String(entity.SettingNineRouterModel)
	}
}

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
			BlueprintOutline: provider.BlueprintOutline{Title: v.Title, Summary: v.Topic, Chapters: outline},
			SlidesPerChapter: v.SlidesPerChapter,
		}, nil
	}
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

func chapterOutline(ctx context.Context, store *sqlite.Store, videoID entity.VideoID) ([]provider.BlueprintChapter, error) {
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

// lateExpander breaks the runner/scheduler cycle; set before the scheduler runs.
type lateExpander struct{ sched *scheduler.Scheduler }

var _ app.GraphExpander = (*lateExpander)(nil)

func (e *lateExpander) Expand(ctx context.Context, videoID entity.VideoID, tail scheduler.Tail) error {
	return e.sched.Expand(ctx, videoID, tail)
}
