package services

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/tbui/yt-studio/internal/app"
	"github.com/tbui/yt-studio/internal/domain/entity"
	"github.com/tbui/yt-studio/internal/domain/provider"
	"github.com/tbui/yt-studio/internal/domain/repository"
	"github.com/tbui/yt-studio/internal/domain/service"
)

// Store is every repository port the services use.
type Store interface {
	repository.ChannelReader
	repository.ChannelWriter
	repository.VideoReader
	repository.VideoWriter
	repository.VideoStateWriter
	repository.VideoFieldWriter
	repository.ChapterReader
	repository.ChapterFieldWriter
	repository.AssetReader
	repository.AssetWriter
	repository.TaskReader
}

// Scheduler is every scheduler port the services use.
type Scheduler interface {
	app.GraphSubmitter
	app.GraphResumer
	app.VideoRequeuer
	app.GraphExpander
	app.VideoCanceller
	app.GateApprover
	app.GateRejecter
	app.VideoForgetter
	app.TaskRetrier
	app.TaskRerunner
	app.StaleMarker
	app.StaleAccepter
	app.PoolLimiter
	app.StatusReporter
}

// Backend is what the services call into, wired in main.go.
type Backend struct {
	Store      Store
	Scheduler  Scheduler
	Assets     provider.AssetStore
	Settings   *service.Settings
	Uploader   provider.Uploader
	UploadAuth provider.UploadAuthorizer
	Prompts    app.PromptCacheInvalidator
	Notifier   app.ChapterNotifier
	Coalescer  app.CoalesceSetter
	LogLevel   *slog.LevelVar
	Log        *slog.Logger
	NewID      func() string
	Now        func() time.Time
	// SavePreparedBlueprint stores an outline for its video's blueprint task.
	SavePreparedBlueprint func(entity.Ref, json.RawMessage) error
}

// video resolves a video by ref or id.
func (b *Backend) video(ctx context.Context, key string) (entity.Video, error) {
	return app.GetVideo(ctx, b.Store, key)
}

// videoDTO is v with its task census.
func (b *Backend) videoDTO(ctx context.Context, v entity.Video) (VideoDTO, error) {
	counts, err := b.Store.CountTasksByVideo(ctx, v.ID)
	if err != nil {
		return VideoDTO{}, err
	}
	return videoFrom(v, counts), nil
}

func taskDTOs(rows []entity.Task) []TaskDTO {
	out := make([]TaskDTO, 0, len(rows))
	for _, t := range rows {
		out = append(out, taskFrom(t))
	}
	return out
}

func taskIDs(ids []string) []entity.TaskID {
	if len(ids) == 0 {
		return nil
	}
	out := make([]entity.TaskID, 0, len(ids))
	for _, id := range ids {
		out = append(out, entity.TaskID(id))
	}
	return out
}
