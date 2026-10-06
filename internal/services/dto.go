// Package services is what the UI calls: Wails binds each service's
// exported methods, and each one calls into app/ and maps the result. These
// DTOs are the shapes the UI receives; the TypeScript bindings are generated
// from them.
package services

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/tbui/yt-studio/internal/domain/entity"
	"github.com/tbui/yt-studio/internal/domain/repository"
)

// StyleDTO is a channel's creative configuration as the API returns it.
type StyleDTO struct{}

func styleFrom(entity.StyleConfig) StyleDTO { return StyleDTO{} }

// ChannelDTO is a channel as the API presents it.
type ChannelDTO struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Style       StyleDTO  `json:"style"`
	Credentials string    `json:"credentials"`
	VideoCount  int       `json:"videoSeq"`
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
	ThumbnailText string   `json:"thumbnailText"`
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

// ThumbnailCellDTO is one tile of the grid. Prompt is the subject alone: the
// shared style clause lives in settings and is appended when the icon is drawn.
type ThumbnailCellDTO struct {
	Caption string `json:"caption"`
	Prompt  string `json:"prompt"`
}

// VideoDTO is a video as the API presents it.
type VideoDTO struct {
	ID                        string             `json:"id"`
	ChannelID                 string             `json:"channelId"`
	Ref                       string             `json:"ref"`
	Title                     string             `json:"title"`
	Topic                     string             `json:"topic"`
	State                     string             `json:"state"`
	ChapterCount              int                `json:"chapterCount"`
	TargetDurationMinutes     int                `json:"targetDurationMinutes"`
	SlidesPerChapter          int                `json:"slidesPerChapter"`
	ThumbnailCells            int                `json:"thumbnailCells"`
	BlueprintAssetID          string             `json:"blueprintAssetId,omitempty"`
	FinalAssetID              string             `json:"finalAssetId,omitempty"`
	ChapterOffsets            []float64          `json:"chapterOffsets"`
	ThumbnailAssetID          string             `json:"thumbnailAssetId,omitempty"`
	ThumbnailOverrideAssetID  string             `json:"thumbnailOverrideAssetId,omitempty"`
	EffectiveThumbnailAssetID string             `json:"effectiveThumbnailAssetId,omitempty"`
	ThumbnailDesign           any                `json:"thumbnailDesign,omitempty"`
	ThumbnailPlan             []ThumbnailCellDTO `json:"thumbnailPlan"`
	ThumbnailIconIDs          []string           `json:"thumbnailIconIds"`
	Metadata                  *MetadataDTO       `json:"metadata,omitempty"`
	Upload                    *UploadDTO         `json:"upload,omitempty"`
	Error                     string             `json:"error,omitempty"`
	Counts                    TaskCountsDTO      `json:"counts"`
	CreatedAt                 time.Time          `json:"createdAt"`
	UpdatedAt                 time.Time          `json:"updatedAt"`
	StartedAt                 *time.Time         `json:"startedAt,omitempty"`
	CompletedAt               *time.Time         `json:"completedAt,omitempty"`
}

func videoFrom(v entity.Video, counts repository.TaskCounts) VideoDTO {
	// json:"chapterOffsets" has no omitempty, so a nil slice would serialise as
	// null where the client's type says array.
	offsets := v.ChapterOffsets
	if offsets == nil {
		offsets = []float64{}
	}
	dto := VideoDTO{
		ChapterOffsets:        offsets,
		ID:                    string(v.ID),
		ChannelID:             string(v.ChannelID),
		Ref:                   string(v.Ref),
		Title:                 v.Title,
		Topic:                 v.Topic,
		State:                 string(v.State),
		ChapterCount:          v.ChapterCount,
		TargetDurationMinutes: v.TargetDurationMinutes,
		SlidesPerChapter:      v.SlidesPerChapter,
		ThumbnailCells:        v.ThumbnailCells,
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
	if v.ThumbnailAssetID != nil {
		dto.ThumbnailAssetID = string(*v.ThumbnailAssetID)
	}
	if v.ThumbnailOverrideAssetID != nil {
		dto.ThumbnailOverrideAssetID = string(*v.ThumbnailOverrideAssetID)
	}
	// Sent alongside both rather than left to the client to work out, so the
	// screen and the upload cannot come to different answers about which of the
	// two images is live.
	dto.EffectiveThumbnailAssetID = string(v.EffectiveThumbnailAssetID())
	// Handed back as a JSON value rather than a string, so the editor reads its
	// own document instead of parsing one out of a field. A document that will
	// not decode is dropped rather than failing the whole video: it was written
	// by a browser and nothing here depends on it.
	if len(v.ThumbnailDesign) > 0 {
		var doc any
		if err := json.Unmarshal(v.ThumbnailDesign, &doc); err == nil {
			dto.ThumbnailDesign = doc
		}
	}
	// Never null: an unplanned grid is an empty list, so the client's cell loop
	// is the same either way.
	dto.ThumbnailPlan = make([]ThumbnailCellDTO, 0, v.ThumbnailCells)
	if v.ThumbnailPlan != nil {
		for _, cell := range v.ThumbnailPlan.Cells {
			dto.ThumbnailPlan = append(dto.ThumbnailPlan, ThumbnailCellDTO{
				Caption: cell.Caption,
				Prompt:  cell.Prompt,
			})
		}
	}
	dto.ThumbnailIconIDs = make([]string, 0, len(v.ThumbnailIconAssetIDs))
	for _, id := range v.ThumbnailIconAssetIDs {
		dto.ThumbnailIconIDs = append(dto.ThumbnailIconIDs, string(id))
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
	ID                   string    `json:"id"`
	VideoID              string    `json:"videoId"`
	Ordinal              int       `json:"ordinal"`
	Title                string    `json:"title"`
	Summary              string    `json:"summary"`
	Script               string    `json:"script"`
	SlidePrompts         []string  `json:"slidePrompts"`
	AudioAssetID         string    `json:"audioAssetId,omitempty"`
	SlideAssetIDs        []string  `json:"slideAssetIds"`
	ClipAssetID          string    `json:"clipAssetId,omitempty"`
	AudioDurationSeconds float64   `json:"audioDurationSeconds"`
	EstimatedWords       int       `json:"estimatedWords"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

func chapterFrom(c entity.Chapter) ChapterDTO {
	slides := make([]string, 0, len(c.SlideAssetIDs))
	for _, id := range c.SlideAssetIDs {
		slides = append(slides, string(id))
	}
	prompts := c.SlidePrompts
	if prompts == nil {
		prompts = []string{}
	}
	dto := ChapterDTO{
		ID:                   string(c.ID),
		VideoID:              string(c.VideoID),
		Ordinal:              c.Ordinal,
		Title:                c.Title,
		Summary:              c.Summary,
		Script:               c.Script,
		SlidePrompts:         prompts,
		SlideAssetIDs:        slides,
		AudioDurationSeconds: c.AudioDurationSeconds,
		EstimatedWords:       c.EstimatedWords,
		UpdatedAt:            c.UpdatedAt,
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
	Kind          string     `json:"kind"`
	Ordinal       int        `json:"ordinal"`
	Index         int        `json:"index"`
	State         string     `json:"state"`
	Pool          string     `json:"pool"`
	Gate          string     `json:"gate,omitempty"`
	Attempt       int        `json:"attempt"`
	MaxAttempts   int        `json:"maxAttempts"`
	DepsRemaining int        `json:"depsRemaining"`
	Stale         bool       `json:"stale"`
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

// SettingDTO is one runtime configuration row.
type SettingDTO struct {
	Key         string                 `json:"key"`
	Value       string                 `json:"value"`
	Type        string                 `json:"type"`
	Group       string                 `json:"group"`
	Description string                 `json:"description"`
	Min         float64                `json:"min"`
	Max         float64                `json:"max"`
	Options     []string               `json:"options"`
	Backend     string                 `json:"backend"`
	Suggestions []SettingSuggestionDTO `json:"suggestions"`
	Secret      bool                   `json:"secret"`
	Configured  bool                   `json:"configured"`
	UpdatedAt   time.Time              `json:"updatedAt"`
}

// SettingSuggestionDTO is one known-good value and the name it goes by.
type SettingSuggestionDTO struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

func settingFrom(s entity.Setting) SettingDTO {
	options := s.Options
	if options == nil {
		options = []string{}
	}
	suggestions := make([]SettingSuggestionDTO, 0, len(s.Suggestions))
	for _, sg := range s.Suggestions {
		suggestions = append(suggestions, SettingSuggestionDTO{Value: sg.Value, Label: sg.Label})
	}
	// A secret's value never leaves the server, not even to the client that just
	// wrote it: the screen needs to know whether one is set, which is what
	// `configured` answers, and nothing more.
	value, configured := s.Value, false
	if s.Secret {
		configured = strings.TrimSpace(value) != ""
		value = ""
	}
	return SettingDTO{
		Key:         string(s.Key),
		Value:       value,
		Type:        string(s.Type),
		Group:       s.Group,
		Description: s.Description,
		Min:         s.Min,
		Max:         s.Max,
		Options:     options,
		Backend:     s.Backend,
		Suggestions: suggestions,
		Secret:      s.Secret,
		Configured:  configured,
		UpdatedAt:   s.UpdatedAt,
	}
}
