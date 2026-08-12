// Package http is the HTTP delivery layer. It is thin: a handler validates its
// input, calls one function in app/, and maps the result. Handlers are typed
// huma operations, so the OpenAPI document — and the browser's TypeScript
// types — are generated from these structs rather than hand-written.
package http

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
	"github.com/tbui/yt-studio/domain/scheduler"
)

// StyleDTO is a channel's creative configuration as the API returns it.
type StyleDTO struct{}

// StyleInputDTO is the same configuration on the way in, where an omitted field
// leaves the stored value alone. Separate types so the client sees that.
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

// ThumbnailCellDTO is one tile of the grid. Prompt is the subject alone: the
// shared style clause lives in settings and is appended when the icon is drawn.
type ThumbnailCellDTO struct {
	Caption string `json:"caption"`
	Prompt  string `json:"prompt"`
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
	TargetDurationMinutes int    `json:"targetDurationMinutes" doc:"Planned running time; zero means it falls out of the chapter count"`
	SlidesPerChapter      int    `json:"slidesPerChapter"`
	ThumbnailCells        int    `json:"thumbnailCells" doc:"Tiles in the thumbnail grid; one icon is generated per tile"`
	BlueprintAssetID      string `json:"blueprintAssetId,omitempty"`
	FinalAssetID          string `json:"finalAssetId,omitempty"`
	ThumbnailAssetID      string `json:"thumbnailAssetId,omitempty"`
	//nolint:lll // one field, one line
	ThumbnailOverrideAssetID string `json:"thumbnailOverrideAssetId,omitempty" doc:"A thumbnail the operator built in the editor; when present this is what publishes"`
	//nolint:lll // one field, one line
	EffectiveThumbnailAssetID string `json:"effectiveThumbnailAssetId,omitempty" doc:"The thumbnail that will actually publish: the override if there is one, otherwise the rendered one"`
	//nolint:lll // one field, one line
	ThumbnailDesign any `json:"thumbnailDesign,omitempty" doc:"The browser editor's document. Arbitrary JSON: the editor owns its shape and the server never interprets it"`
	//nolint:lll // one field, one line
	ThumbnailPlan []ThumbnailCellDTO `json:"thumbnailPlan" doc:"One entry per grid cell, in reading order; empty until the plan has run"`
	//nolint:lll // one field, one line
	ThumbnailIconIDs []string      `json:"thumbnailIconIds" doc:"The icon drawn for each cell, by index; an empty entry is a cell not yet drawn"`
	Metadata         *MetadataDTO  `json:"metadata,omitempty"`
	Upload           *UploadDTO    `json:"upload,omitempty"`
	Error            string        `json:"error,omitempty"`
	Counts           TaskCountsDTO `json:"counts"`
	CreatedAt        time.Time     `json:"createdAt"`
	UpdatedAt        time.Time     `json:"updatedAt"`
	StartedAt        *time.Time    `json:"startedAt,omitempty"`
	CompletedAt      *time.Time    `json:"completedAt,omitempty"`
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
	ID              string    `json:"id"`
	VideoID         string    `json:"videoId"`
	Ordinal         int       `json:"ordinal"`
	Title           string    `json:"title"`
	Summary         string    `json:"summary"`
	Script          string    `json:"script"`
	SlidePrompts    []string  `json:"slidePrompts"`
	AudioAssetID    string    `json:"audioAssetId,omitempty"`
	SlideAssetIDs   []string  `json:"slideAssetIds"`
	ClipAssetID     string    `json:"clipAssetId,omitempty"`
	DurationSeconds float64   `json:"durationSeconds" doc:"Measured from the script once it exists"`
	EstimatedWords  int       `json:"estimatedWords" doc:"Spoken-word budget the blueprint assigned this chapter"`
	UpdatedAt       time.Time `json:"updatedAt"`
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
		ID:              string(c.ID),
		VideoID:         string(c.VideoID),
		Ordinal:         c.Ordinal,
		Title:           c.Title,
		Summary:         c.Summary,
		Script:          c.Script,
		SlidePrompts:    prompts,
		SlideAssetIDs:   slides,
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
	Kind          string     `json:"kind" enum:"blueprint,prime_slide_prompts,slide_prompts,script,tts,slide,clip,concat,metadata,thumbnail,upload"`
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
	Key         string  `json:"key"`
	Value       string  `json:"value"`
	Type        string  `json:"type" enum:"int,bool,string,float"`
	Group       string  `json:"group"`
	Description string  `json:"description"`
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
	//nolint:lll // one field, one line
	Options []string `json:"options" doc:"The only accepted values, when the setting is constrained to a fixed set; empty means free-form"`
	//nolint:lll // one field, one line
	Backend string `json:"backend" doc:"The backend that reads this row, when only one does; empty means the row applies whatever is selected"`
	//nolint:lll // one field, one line
	Suggestions []SettingSuggestionDTO `json:"suggestions" doc:"Known-good values worth offering, with the name a human uses for each; advisory, the field still takes anything"`
	//nolint:lll // one field, one line
	Secret bool `json:"secret" doc:"The value is write-only: it is never sent back, so value is always empty and configured says whether one is stored"`
	//nolint:lll // one field, one line
	Configured bool      `json:"configured" doc:"A secret row has a value stored. Always false for a row that is not secret, which carries its value outright"`
	UpdatedAt  time.Time `json:"updatedAt"`
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

// PresetValueDTO is one row a preset writes.
type PresetValueDTO struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// PresetDTO is a named patch over the settings table. It carries no "active"
// flag: the client holds the settings table, so that is a comparison it makes
// rather than a field that could arrive stale.
type PresetDTO struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	//nolint:lll // one field, one line
	Values []PresetValueDTO `json:"values" doc:"The rows this preset writes, in the order it writes them; rows it does not name are left alone"`
}

func presetFrom(p entity.Preset) PresetDTO {
	values := make([]PresetValueDTO, 0, len(p.Values))
	for _, v := range p.Values {
		values = append(values, PresetValueDTO{Key: string(v.Key), Value: v.Value})
	}
	return PresetDTO{
		Name:        p.Name,
		Title:       p.Title,
		Description: p.Description,
		Values:      values,
	}
}

// AssetDTO is a stored artifact's metadata.
type AssetDTO struct {
	ID        string    `json:"id" doc:"Content address; also the immutable cache key of its URL"`
	VideoID   string    `json:"videoId"`
	ChapterID string    `json:"chapterId,omitempty"`
	Kind      string    `json:"kind" enum:"blueprint,script,prompt,audio,image,clip,final,metadata,thumbnail,thumbnail_plan,thumbnail_icon"`
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
