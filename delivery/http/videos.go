package http

import (
	"context"
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
	"github.com/tbui/yt-studio/domain/service"
)

// VideosOutput is the video list response.
type VideosOutput struct {
	Body struct {
		Videos []VideoDTO `json:"videos"`
		Total  int        `json:"total"`
	}
}

// VideoOutput is a single video response.
type VideoOutput struct {
	Body VideoDTO
}

// ListVideosInput filters the video list.
type ListVideosInput struct {
	ChannelID string `query:"channelId" doc:"Filter by channel id"`
	State     string `query:"state" doc:"Filter by lifecycle state" enum:"draft,running,awaiting_approval,blocked,completed,failed,cancelled"`
	Limit     int    `query:"limit" default:"100" minimum:"1" maximum:"500"`
	Offset    int    `query:"offset" default:"0" minimum:"0"`
}

// VideoKeyInput resolves a video by ref or id.
type VideoKeyInput struct {
	Key string `path:"key" doc:"Video ref (e.g. DSS-14) or id"`
}

// CreateVideoInput is the create-video request body.
type CreateVideoInput struct {
	IdempotencyKey string `header:"Idempotency-Key" doc:"Repeating a request with the same key returns the first result"`
	Body           struct {
		Channel          string `json:"channel" required:"true" doc:"Channel slug or id"`
		Title            string `json:"title" required:"true" minLength:"1" maxLength:"200"`
		Topic            string `json:"topic,omitempty" maxLength:"500"`
		ChapterCount     int    `json:"chapterCount,omitempty" minimum:"0" maximum:"500" doc:"Defaults to the video.default_chapter_count setting"`
		ImagesPerChapter int    `json:"imagesPerChapter,omitempty" minimum:"0" maximum:"20" doc:"Defaults to the video.default_images_per_chapter setting"`
		//nolint:lll // one field, one line
		TargetDurationMinutes int  `json:"targetDurationMinutes,omitempty" minimum:"0" maximum:"720" doc:"Planned running time; omit to let it fall out of the chapter count"`
		Start                 bool `json:"start,omitempty" doc:"Enqueue the DAG immediately"`
	}
}

// GateInput is an approval or rejection request.
type GateInput struct {
	Key  string `path:"key" doc:"Video ref or id"`
	Body struct {
		Gate   string `json:"gate,omitempty" enum:"blueprint,upload" doc:"Which gate to act on; omit to act on whichever is open"`
		Reason string `json:"reason,omitempty" maxLength:"500"`
	}
}

// TaskOutput is a single task response.
type TaskOutput struct {
	Body TaskDTO
}

func getVideos(videos repository.VideoReader, tasks repository.TaskReader) func(context.Context, *ListVideosInput) (*VideosOutput, error) {
	return func(ctx context.Context, in *ListVideosInput) (*VideosOutput, error) {
		filter := repository.VideoFilter{
			ChannelID: entity.ChannelID(in.ChannelID),
			Limit:     in.Limit,
			Offset:    in.Offset,
		}
		if in.State != "" {
			filter.States = []entity.VideoState{entity.VideoState(in.State)}
		}
		rows, total, err := app.ListVideos(ctx, videos, tasks, filter)
		if err != nil {
			return nil, mapError(err)
		}
		out := &VideosOutput{}
		out.Body.Videos = make([]VideoDTO, 0, len(rows))
		for _, r := range rows {
			out.Body.Videos = append(out.Body.Videos, videoFrom(r.Video, r.Counts))
		}
		out.Body.Total = total
		return out, nil
	}
}

func getVideo(videos repository.VideoReader, tasks repository.TaskReader) func(context.Context, *VideoKeyInput) (*VideoOutput, error) {
	return func(ctx context.Context, in *VideoKeyInput) (*VideoOutput, error) {
		v, err := app.GetVideo(ctx, videos, in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		counts, err := tasks.CountTasksByVideo(ctx, v.ID)
		if err != nil {
			return nil, mapError(err)
		}
		return &VideoOutput{Body: videoFrom(v, counts)}, nil
	}
}

func postVideo(
	channels repository.ChannelReader,
	channelWriter repository.ChannelWriter,
	videoWriter repository.VideoWriter,
	videos repository.VideoReader,
	tasks repository.TaskReader,
	submitter app.GraphSubmitter,
	settings *service.Settings,
	newID func() string,
	now func() time.Time,
) func(context.Context, *CreateVideoInput) (*VideoOutput, error) {
	return func(ctx context.Context, in *CreateVideoInput) (*VideoOutput, error) {
		v, err := app.CreateVideo(ctx, channels, channelWriter, videoWriter, newID, now(),
			settings.Int(entity.SettingVideoDefaultChapters),
			settings.Int(entity.SettingVideoDefaultImages),
			app.CreateVideoInput{
				ChannelKey:            in.Body.Channel,
				Title:                 in.Body.Title,
				Topic:                 in.Body.Topic,
				ChapterCount:          in.Body.ChapterCount,
				ImagesPerChapter:      in.Body.ImagesPerChapter,
				TargetDurationMinutes: in.Body.TargetDurationMinutes,
			})
		if err != nil {
			return nil, mapError(err)
		}
		if in.Body.Start {
			if _, err := app.StartVideo(ctx, videos, tasks, submitter, now(), startOptions(settings), string(v.ID)); err != nil {
				return nil, mapError(err)
			}
		}
		counts, err := tasks.CountTasksByVideo(ctx, v.ID)
		if err != nil {
			return nil, mapError(err)
		}
		return &VideoOutput{Body: videoFrom(v, counts)}, nil
	}
}

func startOptions(settings *service.Settings) app.StartVideoOptions {
	return app.StartVideoOptions{
		MaxAttempts:   settings.Int(entity.SettingTaskMaxAttempts),
		BlueprintGate: settings.GateEnabled(entity.GateBlueprint),
	}
}

// expandOptions are read when a blueprint is accepted rather than when the
// video was enqueued, because that is when the tail carrying the upload gate is
// built.
func expandOptions(settings *service.Settings) app.ExpandOptions {
	return app.ExpandOptions{
		MaxAttempts: settings.Int(entity.SettingTaskMaxAttempts),
		UploadGate:  settings.GateEnabled(entity.GateUpload),
	}
}

func postVideoStart(
	videos repository.VideoReader,
	tasks repository.TaskReader,
	submitter app.GraphSubmitter,
	settings *service.Settings,
	now func() time.Time,
) func(context.Context, *VideoKeyInput) (*VideoOutput, error) {
	return func(ctx context.Context, in *VideoKeyInput) (*VideoOutput, error) {
		v, err := app.StartVideo(ctx, videos, tasks, submitter, now(), startOptions(settings), in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		counts, err := tasks.CountTasksByVideo(ctx, v.ID)
		if err != nil {
			return nil, mapError(err)
		}
		return &VideoOutput{Body: videoFrom(v, counts)}, nil
	}
}

func postVideoCancel(
	videos repository.VideoReader,
	states repository.VideoStateWriter,
	canceller app.VideoCanceller,
	tasks repository.TaskReader,
) func(context.Context, *VideoKeyInput) (*VideoOutput, error) {
	return func(ctx context.Context, in *VideoKeyInput) (*VideoOutput, error) {
		v, err := app.CancelVideo(ctx, videos, states, canceller, in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		counts, err := tasks.CountTasksByVideo(ctx, v.ID)
		if err != nil {
			return nil, mapError(err)
		}
		return &VideoOutput{Body: videoFrom(v, counts)}, nil
	}
}

func postVideoApprove(
	videos repository.VideoReader,
	tasks repository.TaskReader,
	chapters repository.ChapterReader,
	expander app.GraphExpander,
	approver app.GateApprover,
	settings *service.Settings,
	now func() time.Time,
) func(context.Context, *GateInput) (*TaskOutput, error) {
	return func(ctx context.Context, in *GateInput) (*TaskOutput, error) {
		v, err := app.GetVideo(ctx, videos, in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		t, err := app.ApproveGate(ctx, tasks, videos, chapters, expander, approver,
			now(), expandOptions(settings), v.ID, entity.GateKind(in.Body.Gate))
		if err != nil {
			return nil, mapError(err)
		}
		return &TaskOutput{Body: taskFrom(t)}, nil
	}
}

func postVideoReject(
	videos repository.VideoReader,
	tasks repository.TaskReader,
	rejecter app.GateRejecter,
) func(context.Context, *GateInput) (*TaskOutput, error) {
	return func(ctx context.Context, in *GateInput) (*TaskOutput, error) {
		v, err := app.GetVideo(ctx, videos, in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		t, err := app.RejectGate(ctx, tasks, rejecter, v.ID, entity.GateKind(in.Body.Gate), in.Body.Reason)
		if err != nil {
			return nil, mapError(err)
		}
		return &TaskOutput{Body: taskFrom(t)}, nil
	}
}

func deleteVideo(
	videos repository.VideoReader,
	writer repository.VideoWriter,
	forgetter app.VideoForgetter,
	store provider.AssetStore,
	log *slog.Logger,
) func(context.Context, *VideoKeyInput) (*struct{}, error) {
	return func(ctx context.Context, in *VideoKeyInput) (*struct{}, error) {
		if err := app.DeleteVideo(ctx, videos, writer, forgetter, store, log, in.Key); err != nil {
			return nil, mapError(err)
		}
		return nil, nil //nolint:nilnil // huma models 204 as a nil body
	}
}

//nolint:revive // the parameter list is the dependency list
func registerVideoRoutes(
	api huma.API,
	videos repository.VideoReader,
	videoWriter repository.VideoWriter,
	states repository.VideoStateWriter,
	channels repository.ChannelReader,
	channelWriter repository.ChannelWriter,
	taskReader repository.TaskReader,
	chapters repository.ChapterReader,
	submitter app.GraphSubmitter,
	expander app.GraphExpander,
	canceller app.VideoCanceller,
	approver app.GateApprover,
	rejecter app.GateRejecter,
	forgetter app.VideoForgetter,
	store provider.AssetStore,
	settings *service.Settings,
	newID func() string,
	now func() time.Time,
	log *slog.Logger,
) {
	huma.Register(api, huma.Operation{
		OperationID: "listVideos", Method: "GET", Path: "/api/videos",
		Summary: "List videos", Tags: []string{"videos"},
	}, getVideos(videos, taskReader))

	huma.Register(api, huma.Operation{
		OperationID: "getVideo", Method: "GET", Path: "/api/videos/{key}",
		Summary: "Get a video by ref or id", Tags: []string{"videos"},
	}, getVideo(videos, taskReader))

	huma.Register(api, huma.Operation{
		OperationID: "createVideo", Method: "POST", Path: "/api/videos",
		Summary: "Create a video", Tags: []string{"videos"}, DefaultStatus: 201,
	}, postVideo(channels, channelWriter, videoWriter, videos, taskReader, submitter, settings, newID, now))

	huma.Register(api, huma.Operation{
		OperationID: "startVideo", Method: "POST", Path: "/api/videos/{key}/start",
		Summary: "Enqueue a video's DAG", Tags: []string{"videos"},
		Description: "Idempotent: task ids are deterministic, so starting twice schedules nothing new.",
	}, postVideoStart(videos, taskReader, submitter, settings, now))

	huma.Register(api, huma.Operation{
		OperationID: "cancelVideo", Method: "POST", Path: "/api/videos/{key}/cancel",
		Summary: "Cancel a video", Tags: []string{"videos"},
	}, postVideoCancel(videos, states, canceller, taskReader))

	huma.Register(api, huma.Operation{
		OperationID: "approveGate", Method: "POST", Path: "/api/videos/{key}/approve",
		Summary: "Approve the open gate", Tags: []string{"videos"},
	}, postVideoApprove(videos, taskReader, chapters, expander, approver, settings, now))

	huma.Register(api, huma.Operation{
		OperationID: "rejectGate", Method: "POST", Path: "/api/videos/{key}/reject",
		Summary: "Reject the open gate", Tags: []string{"videos"},
	}, postVideoReject(videos, taskReader, rejecter))

	huma.Register(api, huma.Operation{
		OperationID: "deleteVideo", Method: "DELETE", Path: "/api/videos/{key}",
		Summary: "Delete a video", Tags: []string{"videos"}, DefaultStatus: 204,
	}, deleteVideo(videos, videoWriter, forgetter, store, log))
}
