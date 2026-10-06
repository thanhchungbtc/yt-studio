package services

import (
	"context"
	"encoding/json"
	"unicode/utf8"

	"github.com/tbui/yt-studio/internal/app"
	"github.com/tbui/yt-studio/internal/domain/entity"
	"github.com/tbui/yt-studio/internal/domain/repository"
)

// VideoService runs a video's lifecycle.
type VideoService struct{ b *Backend }

// NewVideoService creates a VideoService.
func NewVideoService(b *Backend) *VideoService { return &VideoService{b: b} }

// NewVideo is a video to create; zero counts take the settings' defaults.
type NewVideo struct {
	Channel               string `json:"channel"`
	Title                 string `json:"title"`
	Topic                 string `json:"topic,omitempty"`
	ChapterCount          int    `json:"chapterCount,omitempty"`
	SlidesPerChapter      int    `json:"slidesPerChapter,omitempty"`
	ThumbnailCells        int    `json:"thumbnailCells,omitempty"`
	TargetDurationMinutes int    `json:"targetDurationMinutes,omitempty"`
	// Start enqueues the DAG straight away.
	Start bool `json:"start,omitempty"`
	// Blueprint is a prepared outline; the video then skips the blueprint gate.
	Blueprint json.RawMessage `json:"blueprint,omitempty"`
}

// VideoBrief is a video's editable brief, whole.
type VideoBrief struct {
	Title                 string `json:"title"`
	Topic                 string `json:"topic"`
	ChapterCount          int    `json:"chapterCount"`
	SlidesPerChapter      int    `json:"slidesPerChapter"`
	ThumbnailCells        int    `json:"thumbnailCells"`
	TargetDurationMinutes int    `json:"targetDurationMinutes"`
}

// Republished is what a republish disowned: the video left on YouTube.
type Republished struct {
	PreviousURL     string `json:"previousUrl"`
	PreviousVideoID string `json:"previousVideoId"`
}

func checkBrief(title, topic string, chapters, slides, cells, minutes, minCount int) error {
	switch n := utf8.RuneCountInString(title); {
	case n == 0 || n > 200:
		return app.Invalid("title", "must be 1 to 200 characters")
	case utf8.RuneCountInString(topic) > 5000:
		return app.Invalid("topic", "must be at most 5000 characters")
	case chapters < minCount || chapters > 500:
		return app.Invalid("chapterCount", "is out of range")
	case slides < minCount || slides > 20:
		return app.Invalid("slidesPerChapter", "is out of range")
	case cells < minCount || cells > 24:
		return app.Invalid("thumbnailCells", "is out of range")
	case minutes < 0 || minutes > 720:
		return app.Invalid("targetDurationMinutes", "is out of range")
	}
	return nil
}

// List returns every video, without thumbnail designs.
func (s *VideoService) List(ctx context.Context) ([]VideoDTO, error) {
	rows, _, err := app.ListVideos(ctx, s.b.Store, s.b.Store, repository.VideoFilter{Limit: 500})
	if err != nil {
		return nil, err
	}
	out := make([]VideoDTO, 0, len(rows))
	for _, r := range rows {
		dto := videoFrom(r.Video, r.Counts)
		dto.ThumbnailDesign = nil
		out = append(out, dto)
	}
	return out, nil
}

// Get returns a video by ref (e.g. DSS-14) or id.
func (s *VideoService) Get(ctx context.Context, key string) (VideoDTO, error) {
	v, err := s.b.video(ctx, key)
	if err != nil {
		return VideoDTO{}, err
	}
	return s.b.videoDTO(ctx, v)
}

// Create creates a video, and starts it when asked.
func (s *VideoService) Create(ctx context.Context, in NewVideo) (VideoDTO, error) {
	if err := checkBrief(in.Title, in.Topic, in.ChapterCount, in.SlidesPerChapter,
		in.ThumbnailCells, in.TargetDurationMinutes, 0); err != nil {
		return VideoDTO{}, err
	}
	settings := s.b.Settings
	v, err := app.CreateVideo(ctx, s.b.Store, s.b.Store, s.b.Store, s.b.NewID, s.b.Now(),
		settings.Int(entity.SettingVideoDefaultChapters),
		settings.Int(entity.SettingVideoDefaultSlides),
		settings.Int(entity.SettingVideoDefaultThumbnailCells),
		app.CreateVideoInput{
			ChannelKey:            in.Channel,
			Title:                 in.Title,
			Topic:                 in.Topic,
			ChapterCount:          in.ChapterCount,
			SlidesPerChapter:      in.SlidesPerChapter,
			ThumbnailCells:        in.ThumbnailCells,
			TargetDurationMinutes: in.TargetDurationMinutes,
		})
	if err != nil {
		return VideoDTO{}, err
	}
	prepared := len(in.Blueprint) > 0 && string(in.Blueprint) != "null"
	if prepared {
		if err := s.b.SavePreparedBlueprint(v.Ref, in.Blueprint); err != nil {
			return VideoDTO{}, err
		}
	}
	if in.Start {
		opts := s.startOptions()
		// A pasted outline needs no blueprint review.
		if prepared {
			opts.BlueprintGate = false
		}
		if _, err := app.StartVideo(ctx, s.b.Store, s.b.Store, s.b.Scheduler, s.b.Scheduler, s.b.Scheduler,
			s.b.Now(), opts, string(v.ID)); err != nil {
			return VideoDTO{}, err
		}
	}
	return s.b.videoDTO(ctx, v)
}

// Update replaces a video's brief; nothing re-runs.
func (s *VideoService) Update(ctx context.Context, key string, brief VideoBrief) (VideoDTO, error) {
	if err := checkBrief(brief.Title, brief.Topic, brief.ChapterCount, brief.SlidesPerChapter,
		brief.ThumbnailCells, brief.TargetDurationMinutes, 1); err != nil {
		return VideoDTO{}, err
	}
	v, err := app.UpdateVideo(ctx, s.b.Store, s.b.Store, s.b.Now(), key, app.UpdateVideoInput{
		Title:                 brief.Title,
		Topic:                 brief.Topic,
		ChapterCount:          brief.ChapterCount,
		SlidesPerChapter:      brief.SlidesPerChapter,
		ThumbnailCells:        brief.ThumbnailCells,
		TargetDurationMinutes: brief.TargetDurationMinutes,
	})
	if err != nil {
		return VideoDTO{}, err
	}
	return s.b.videoDTO(ctx, v)
}

func (s *VideoService) startOptions() app.StartVideoOptions {
	return app.StartVideoOptions{
		MaxAttempts:   s.b.Settings.Int(entity.SettingTaskMaxAttempts),
		BlueprintGate: s.b.Settings.GateEnabled(entity.GateBlueprint),
	}
}

// Start enqueues a draft, or requeues what a stopped video stopped on.
func (s *VideoService) Start(ctx context.Context, key string) (VideoDTO, error) {
	v, err := app.StartVideo(ctx, s.b.Store, s.b.Store, s.b.Scheduler, s.b.Scheduler, s.b.Scheduler,
		s.b.Now(), s.startOptions(), key)
	if err != nil {
		return VideoDTO{}, err
	}
	return s.b.videoDTO(ctx, v)
}

// Cancel stops a video.
func (s *VideoService) Cancel(ctx context.Context, key string) (VideoDTO, error) {
	v, err := app.CancelVideo(ctx, s.b.Store, s.b.Store, s.b.Scheduler, key)
	if err != nil {
		return VideoDTO{}, err
	}
	return s.b.videoDTO(ctx, v)
}

// Republish uploads a published video again as a new YouTube video.
func (s *VideoService) Republish(ctx context.Context, key string) (Republished, error) {
	previous, err := app.RepublishVideo(ctx, s.b.Store, s.b.Store, s.b.Store, s.b.Scheduler, s.b.Scheduler, key)
	if err != nil {
		return Republished{}, err
	}
	return Republished{PreviousURL: previous.URL, PreviousVideoID: previous.VideoID}, nil
}

// Delete removes a video, its chapters, its tasks and the files only it uses.
func (s *VideoService) Delete(ctx context.Context, key string) error {
	return app.DeleteVideo(ctx, s.b.Store, s.b.Store, s.b.Scheduler, s.b.Assets, s.b.Log, key)
}

// Approve approves a gate ("" for whichever is open).
func (s *VideoService) Approve(ctx context.Context, key, gate string) (TaskDTO, error) {
	v, err := s.b.video(ctx, key)
	if err != nil {
		return TaskDTO{}, err
	}
	t, err := app.ApproveGate(ctx, s.b.Store, s.b.Store, s.b.Store, s.b.Scheduler, s.b.Scheduler,
		s.b.Now(), app.ExpandOptions{
			MaxAttempts: s.b.Settings.Int(entity.SettingTaskMaxAttempts),
			ScriptGate:  s.b.Settings.GateEnabled(entity.GateScript),
			UploadGate:  s.b.Settings.GateEnabled(entity.GateUpload),
		}, v.ID, entity.GateKind(gate))
	if err != nil {
		return TaskDTO{}, err
	}
	return taskFrom(t), nil
}

// Reject rejects a gate ("" for whichever is open).
func (s *VideoService) Reject(ctx context.Context, key, gate, reason string) (TaskDTO, error) {
	if utf8.RuneCountInString(reason) > 500 {
		return TaskDTO{}, app.Invalid("reason", "must be at most 500 characters")
	}
	v, err := s.b.video(ctx, key)
	if err != nil {
		return TaskDTO{}, err
	}
	t, err := app.RejectGate(ctx, s.b.Store, s.b.Scheduler, v.ID, entity.GateKind(gate), reason)
	if err != nil {
		return TaskDTO{}, err
	}
	return taskFrom(t), nil
}

// SaveMetadata replaces a video's YouTube listing. Nothing re-runs.
func (s *VideoService) SaveMetadata(ctx context.Context, key string, m MetadataDTO) (VideoDTO, error) {
	v, err := s.b.video(ctx, key)
	if err != nil {
		return VideoDTO{}, err
	}
	v, err = app.UpdateVideoMetadata(ctx, s.b.Store, s.b.Store, v.ID, entity.Metadata{
		Title:         m.Title,
		Description:   m.Description,
		Tags:          m.Tags,
		ThumbnailText: m.ThumbnailText,
		CategoryID:    m.CategoryID,
		Privacy:       m.Privacy,
	})
	if err != nil {
		return VideoDTO{}, err
	}
	return s.b.videoDTO(ctx, v)
}

// PushMetadata sends a published video's listing to YouTube again.
func (s *VideoService) PushMetadata(ctx context.Context, key string) (VideoDTO, error) {
	v, err := s.b.video(ctx, key)
	if err != nil {
		return VideoDTO{}, err
	}
	v, err = app.PushVideoMetadata(ctx, s.b.Store, s.b.Store, s.b.Store, s.b.Uploader, v.ID,
		s.b.Settings.Bool(entity.SettingUploadDryRun))
	if err != nil {
		return VideoDTO{}, err
	}
	return s.b.videoDTO(ctx, v)
}
