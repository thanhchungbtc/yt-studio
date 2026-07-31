// Package http is the HTTP delivery layer.
//
// It is deliberately thin: a handler validates its input, calls exactly one
// function in app/, and maps the result. No business logic lives here, which is
// what makes adding a CLI a matter of calling the same app function.
//
// Handlers are typed huma operations, so the OpenAPI document — and therefore
// the browser client's TypeScript types — are generated from these Go structs
// rather than hand-written.
package http

import (
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
	"github.com/tbui/yt-studio/domain/scheduler"
)

// StyleDTO is a channel's creative configuration as the API returns it. Every
// field is always present; a blank one means the channel leaves it unset.
type StyleDTO struct{}

// StyleInputDTO is the same configuration on the way in, where every field is
// optional: an omitted one leaves the stored value alone. Input and output are
// separate types precisely so the generated client sees that difference.
type StyleInputDTO struct{}

// Into converts the request DTO to the domain type.
func (StyleInputDTO) Into() entity.StyleConfig { return entity.StyleConfig{} }

func styleFrom(entity.StyleConfig) StyleDTO { return StyleDTO{} }

// ChannelDTO is a channel as the API presents it.
type ChannelDTO struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug" doc:"Stable human-readable natural key; immutable"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Style       StyleDTO  `json:"style"`
	Credentials string    `json:"credentials" enum:"missing,valid,expired"`
	VideoCount  int       `json:"videoSeq" doc:"Videos minted so far, which is the next ref's number minus one"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func channelFrom(c entity.Channel) ChannelDTO {
	return ChannelDTO{
		ID:          string(c.ID),
		Slug:        string(c.Slug),
		Name:        c.Name,
		Description: c.Description,
		Style:       styleFrom(c.Style),
		Credentials: string(c.Credentials),
		VideoCount:  c.VideoSeq,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
	}
}

// TaskCountsDTO is a video's task census.
type TaskCountsDTO struct {
	Total            int `json:"total"`
	Succeeded        int `json:"succeeded"`
	Failed           int `json:"failed"`
	Running          int `json:"running"`
	Ready            int `json:"ready"`
	Blocked          int `json:"blocked"`
	AwaitingApproval int `json:"awaitingApproval"`
	Cancelled        int `json:"cancelled"`
	// Stale cuts across the counts above; it does not partition with them.
	Stale int `json:"stale"`
}

func countsFrom(c repository.TaskCounts) TaskCountsDTO {
	return TaskCountsDTO{
		Total:            c.Total,
		Succeeded:        c.Succeeded,
		Failed:           c.Failed,
		Running:          c.Running,
		Ready:            c.Ready,
		Blocked:          c.Blocked,
		AwaitingApproval: c.AwaitingApproval,
		Cancelled:        c.Cancelled,
		Stale:            c.Stale,
	}
}

// MetadataDTO is the YouTube-facing listing.
type MetadataDTO struct {
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Tags          []string `json:"tags"`
	ThumbnailText string   `json:"thumbnailText" doc:"All-caps hook for the thumbnail overlay"`
	CategoryID    string   `json:"categoryId"`
	Privacy       string   `json:"privacy"`
}

// UploadDTO is the upload receipt.
type UploadDTO struct {
	VideoID    string    `json:"remoteVideoId"`
	URL        string    `json:"url"`
	DryRun     bool      `json:"dryRun"`
	UploadedAt time.Time `json:"uploadedAt"`
}

// VideoDTO is a video as the API presents it.
type VideoDTO struct {
	ID           string `json:"id"`
	ChannelID    string `json:"channelId"`
	Ref          string `json:"ref" doc:"Stable human-readable natural key, e.g. DSS-14"`
	Title        string `json:"title"`
	Topic        string `json:"topic"`
	State        string `json:"state" enum:"draft,running,awaiting_approval,blocked,completed,failed,cancelled"`
	ChapterCount int    `json:"chapterCount" doc:"Chapters asked for; the accepted blueprint decides the real number"`
	//nolint:lll // one field, one line
	TargetDurationMinutes int           `json:"targetDurationMinutes" doc:"Planned running time; zero means it falls out of the chapter count"`
	ImagesPerChapter      int           `json:"imagesPerChapter"`
	BlueprintAssetID      string        `json:"blueprintAssetId,omitempty"`
	FinalAssetID          string        `json:"finalAssetId,omitempty"`
	Metadata              *MetadataDTO  `json:"metadata,omitempty"`
	Upload                *UploadDTO    `json:"upload,omitempty"`
	Error                 string        `json:"error,omitempty"`
	Counts                TaskCountsDTO `json:"counts"`
	CreatedAt             time.Time     `json:"createdAt"`
	UpdatedAt             time.Time     `json:"updatedAt"`
	StartedAt             *time.Time    `json:"startedAt,omitempty"`
	CompletedAt           *time.Time    `json:"completedAt,omitempty"`
}

func videoFrom(v entity.Video, counts repository.TaskCounts) VideoDTO {
	dto := VideoDTO{
		ID:                    string(v.ID),
		ChannelID:             string(v.ChannelID),
		Ref:                   string(v.Ref),
		Title:                 v.Title,
		Topic:                 v.Topic,
		State:                 string(v.State),
		ChapterCount:          v.ChapterCount,
		TargetDurationMinutes: v.TargetDurationMinutes,
		ImagesPerChapter:      v.ImagesPerChapter,
		Error:                 v.Error,
		Counts:                countsFrom(counts),
		CreatedAt:             v.CreatedAt,
		UpdatedAt:             v.UpdatedAt,
		StartedAt:             v.StartedAt,
		CompletedAt:           v.CompletedAt,
	}
	if v.BlueprintAssetID != nil {
		dto.BlueprintAssetID = string(*v.BlueprintAssetID)
	}
	if v.FinalAssetID != nil {
		dto.FinalAssetID = string(*v.FinalAssetID)
	}
	if v.Metadata != nil {
		tags := v.Metadata.Tags
		if tags == nil {
			tags = []string{}
		}
		dto.Metadata = &MetadataDTO{
			Title:         v.Metadata.Title,
			Description:   v.Metadata.Description,
			Tags:          tags,
			ThumbnailText: v.Metadata.ThumbnailText,
			CategoryID:    v.Metadata.CategoryID,
			Privacy:       v.Metadata.Privacy,
		}
	}
	if v.Upload != nil {
		dto.Upload = &UploadDTO{
			VideoID:    v.Upload.VideoID,
			URL:        v.Upload.URL,
			DryRun:     v.Upload.DryRun,
			UploadedAt: v.Upload.UploadedAt,
		}
	}
	return dto
}

// ChapterDTO is a chapter as the API presents it.
type ChapterDTO struct {
	ID              string    `json:"id"`
	VideoID         string    `json:"videoId"`
	Ordinal         int       `json:"ordinal"`
	Title           string    `json:"title"`
	Summary         string    `json:"summary"`
	Script          string    `json:"script"`
	ImagePrompts    []string  `json:"imagePrompts"`
	AudioAssetID    string    `json:"audioAssetId,omitempty"`
	ImageAssetIDs   []string  `json:"imageAssetIds"`
	ClipAssetID     string    `json:"clipAssetId,omitempty"`
	DurationSeconds float64   `json:"durationSeconds" doc:"Measured from the script once it exists"`
	EstimatedWords  int       `json:"estimatedWords" doc:"Spoken-word budget the blueprint assigned this chapter"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func chapterFrom(c entity.Chapter) ChapterDTO {
	images := make([]string, 0, len(c.ImageAssetIDs))
	for _, id := range c.ImageAssetIDs {
		images = append(images, string(id))
	}
	prompts := c.ImagePrompts
	if prompts == nil {
		prompts = []string{}
	}
	dto := ChapterDTO{
		ID:              string(c.ID),
		VideoID:         string(c.VideoID),
		Ordinal:         c.Ordinal,
		Title:           c.Title,
		Summary:         c.Summary,
		Script:          c.Script,
		ImagePrompts:    prompts,
		ImageAssetIDs:   images,
		DurationSeconds: c.DurationSeconds,
		EstimatedWords:  c.EstimatedWords,
		UpdatedAt:       c.UpdatedAt,
	}
	if c.AudioAssetID != nil {
		dto.AudioAssetID = string(*c.AudioAssetID)
	}
	if c.ClipAssetID != nil {
		dto.ClipAssetID = string(*c.ClipAssetID)
	}
	return dto
}

// TaskDTO is a task as the API presents it.
type TaskDTO struct {
	ID            string     `json:"id"`
	VideoID       string     `json:"videoId"`
	ChapterID     string     `json:"chapterId,omitempty"`
	Kind          string     `json:"kind" enum:"blueprint,prime_image_prompts,image_prompts,script,tts,image,clip,concat,metadata,thumbnail,upload"`
	Ordinal       int        `json:"ordinal"`
	Index         int        `json:"index"`
	State         string     `json:"state" enum:"blocked,ready,running,awaiting_approval,succeeded,failed,cancelled"`
	Pool          string     `json:"pool" enum:"llm,tts,image,compose,cache,upload"`
	Gate          string     `json:"gate,omitempty" enum:"blueprint,upload"`
	Attempt       int        `json:"attempt"`
	MaxAttempts   int        `json:"maxAttempts"`
	DepsRemaining int        `json:"depsRemaining"`
	Stale         bool       `json:"stale" doc:"An input changed after this task ran; its artifact is intact but unverified"`
	Error         string     `json:"error,omitempty"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	FinishedAt    *time.Time `json:"finishedAt,omitempty"`
	NotBefore     *time.Time `json:"notBefore,omitempty"`
}

func taskFrom(t entity.Task) TaskDTO {
	dto := TaskDTO{
		ID:            string(t.ID),
		VideoID:       string(t.VideoID),
		Kind:          string(t.Kind),
		Ordinal:       t.Ordinal,
		Index:         t.Index,
		State:         string(t.State),
		Pool:          string(t.Pool),
		Gate:          string(t.Gate),
		Attempt:       t.Attempt,
		MaxAttempts:   t.MaxAttempts,
		DepsRemaining: t.DepsRemaining,
		Stale:         t.Stale,
		Error:         t.Error,
		UpdatedAt:     t.UpdatedAt,
		StartedAt:     t.StartedAt,
		FinishedAt:    t.FinishedAt,
		NotBefore:     t.NotBefore,
	}
	if t.ChapterID != nil {
		dto.ChapterID = string(*t.ChapterID)
	}
	return dto
}

// PoolStatDTO is one pool's live utilisation.
type PoolStatDTO struct {
	Pool     string `json:"pool" enum:"llm,tts,image,compose,cache,upload"`
	Limit    int    `json:"limit"`
	InFlight int    `json:"inFlight"`
	Queued   int    `json:"queued"`
}

// SchedulerStatusDTO is the operator console's view.
type SchedulerStatusDTO struct {
	Pools            []PoolStatDTO `json:"pools"`
	Ready            int           `json:"ready"`
	Running          int           `json:"running"`
	Blocked          int           `json:"blocked"`
	AwaitingApproval int           `json:"awaitingApproval"`
	Succeeded        int           `json:"succeeded"`
	Failed           int           `json:"failed"`
	RetryPending     int           `json:"retryPending"`
	Videos           int           `json:"videos"`
	UptimeSeconds    float64       `json:"uptimeSeconds"`
	StartedAt        time.Time     `json:"startedAt"`
}

func statusFrom(s scheduler.Status) SchedulerStatusDTO {
	pools := make([]PoolStatDTO, 0, len(s.Pools))
	for _, p := range s.Pools {
		pools = append(pools, PoolStatDTO{
			Pool:     string(p.Pool),
			Limit:    p.Limit,
			InFlight: p.InFlight,
			Queued:   p.Queued,
		})
	}
	return SchedulerStatusDTO{
		Pools:            pools,
		Ready:            s.Ready,
		Running:          s.Running,
		Blocked:          s.Blocked,
		AwaitingApproval: s.AwaitingApproval,
		Succeeded:        s.Succeeded,
		Failed:           s.Failed,
		RetryPending:     s.RetryPending,
		Videos:           s.Videos,
		UptimeSeconds:    s.UptimeSeconds,
		StartedAt:        s.StartedAt,
	}
}

// SettingDTO is one runtime configuration row.
type SettingDTO struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Type        string `json:"type" enum:"int,bool,string"`
	Group       string `json:"group"`
	Description string `json:"description"`
	Min         int    `json:"min"`
	Max         int    `json:"max"`
	//nolint:lll // one field, one line
	Options   []string  `json:"options" doc:"The only accepted values, when the setting is constrained to a fixed set; empty means free-form"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func settingFrom(s entity.Setting) SettingDTO {
	options := s.Options
	if options == nil {
		options = []string{}
	}
	return SettingDTO{
		Key:         string(s.Key),
		Value:       s.Value,
		Type:        string(s.Type),
		Group:       s.Group,
		Description: s.Description,
		Min:         s.Min,
		Max:         s.Max,
		Options:     options,
		UpdatedAt:   s.UpdatedAt,
	}
}

// AssetDTO is a stored artifact's metadata.
type AssetDTO struct {
	ID        string    `json:"id" doc:"Content address; also the immutable cache key of its URL"`
	VideoID   string    `json:"videoId"`
	ChapterID string    `json:"chapterId,omitempty"`
	Kind      string    `json:"kind" enum:"blueprint,script,prompt,audio,image,clip,final,metadata,thumbnail"`
	Size      int64     `json:"size"`
	MIME      string    `json:"mime"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
}

func assetFrom(a entity.Asset) AssetDTO {
	dto := AssetDTO{
		ID:        string(a.ID),
		VideoID:   string(a.VideoID),
		Kind:      string(a.Kind),
		Size:      a.Size,
		MIME:      a.MIME,
		URL:       "/assets/" + string(a.ID),
		CreatedAt: a.CreatedAt,
	}
	if a.ChapterID != nil {
		dto.ChapterID = string(*a.ChapterID)
	}
	return dto
}
