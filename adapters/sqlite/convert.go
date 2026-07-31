package sqlite

import (
	"encoding/json"
	"time"

	"github.com/tbui/yt-studio/adapters/sqlite/sqlcgen"
	"github.com/tbui/yt-studio/domain/entity"
)

// Timestamps are stored as unix nanoseconds: integer comparison keeps index
// range scans cheap and avoids parsing a string on every row read.

func toUnix(t time.Time) int64 { return t.UTC().UnixNano() }

func fromUnix(n int64) time.Time { return time.Unix(0, n).UTC() }

func toUnixPtr(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	n := toUnix(*t)
	return &n
}

func fromUnixPtr(n *int64) *time.Time {
	if n == nil {
		return nil
	}
	t := fromUnix(*n)
	return &t
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func assetIDPtr(id *entity.AssetID) *string {
	if id == nil || *id == "" {
		return nil
	}
	s := string(*id)
	return &s
}

func toAssetID(s *string) *entity.AssetID {
	if s == nil || *s == "" {
		return nil
	}
	id := entity.AssetID(*s)
	return &id
}

func chapterIDPtr(id *entity.ChapterID) *string {
	if id == nil || *id == "" {
		return nil
	}
	s := string(*id)
	return &s
}

func toChapterID(s *string) *entity.ChapterID {
	if s == nil || *s == "" {
		return nil
	}
	id := entity.ChapterID(*s)
	return &id
}

func encodeJSON[T any](v T) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func decodeJSON[T any](s string, out *T) error {
	if s == "" {
		return nil
	}
	return json.Unmarshal([]byte(s), out)
}

func channelFromRow(r sqlcgen.Channel) entity.Channel {
	return entity.Channel{
		ID:          entity.ChannelID(r.ID),
		Slug:        entity.Slug(r.Slug),
		Name:        r.Name,
		Description: r.Description,
		Style:       entity.StyleConfig{},
		Credentials: entity.CredentialStatus(r.Credentials),
		VideoSeq:    int(r.VideoSeq),
		CreatedAt:   fromUnix(r.CreatedAt),
		UpdatedAt:   fromUnix(r.UpdatedAt),
	}
}

func videoFromRow(r sqlcgen.Video) (entity.Video, error) {
	v := entity.Video{
		ID:                    entity.VideoID(r.ID),
		ChannelID:             entity.ChannelID(r.ChannelID),
		Ref:                   entity.Ref(r.Ref),
		Title:                 r.Title,
		Topic:                 r.Topic,
		State:                 entity.VideoState(r.State),
		ChapterCount:          int(r.ChapterCount),
		TargetDurationMinutes: int(r.TargetDurationMinutes),
		ImagesPerChapter:      int(r.ImagesPerChapter),
		ThumbnailCells:        int(r.ThumbnailCells),
		BlueprintAssetID:      toAssetID(r.BlueprintAssetID),
		FinalAssetID:          toAssetID(r.FinalAssetID),
		ThumbnailAssetID:      toAssetID(r.ThumbnailAssetID),
		Error:                 r.Error,
		CreatedAt:             fromUnix(r.CreatedAt),
		UpdatedAt:             fromUnix(r.UpdatedAt),
		StartedAt:             fromUnixPtr(r.StartedAt),
		CompletedAt:           fromUnixPtr(r.CompletedAt),
	}
	if r.MetadataJson != nil && *r.MetadataJson != "" {
		var m entity.Metadata
		if err := decodeJSON(*r.MetadataJson, &m); err != nil {
			return entity.Video{}, err
		}
		v.Metadata = &m
	}
	if r.ThumbnailPlanJson != nil && *r.ThumbnailPlanJson != "" {
		var p entity.ThumbnailPlan
		if err := decodeJSON(*r.ThumbnailPlanJson, &p); err != nil {
			return entity.Video{}, err
		}
		v.ThumbnailPlan = &p
	}
	if err := decodeJSON(r.ThumbnailIconIdsJson, &v.ThumbnailIconAssetIDs); err != nil {
		return entity.Video{}, err
	}
	if r.UploadJson != nil && *r.UploadJson != "" {
		var u entity.UploadRecord
		if err := decodeJSON(*r.UploadJson, &u); err != nil {
			return entity.Video{}, err
		}
		v.Upload = &u
	}
	return v, nil
}

func chapterFromRow(r sqlcgen.Chapter) (entity.Chapter, error) {
	c := entity.Chapter{
		ID:              entity.ChapterID(r.ID),
		VideoID:         entity.VideoID(r.VideoID),
		Ordinal:         int(r.Ordinal),
		Title:           r.Title,
		Summary:         r.Summary,
		Script:          r.Script,
		AudioAssetID:    toAssetID(r.AudioAssetID),
		ClipAssetID:     toAssetID(r.ClipAssetID),
		DurationSeconds: r.DurationSeconds,
		EstimatedWords:  int(r.EstimatedWords),
		CreatedAt:       fromUnix(r.CreatedAt),
		UpdatedAt:       fromUnix(r.UpdatedAt),
	}
	if err := decodeJSON(r.ImagePromptsJson, &c.ImagePrompts); err != nil {
		return entity.Chapter{}, err
	}
	if err := decodeJSON(r.ImageAssetIdsJson, &c.ImageAssetIDs); err != nil {
		return entity.Chapter{}, err
	}
	return c, nil
}

func assetFromRow(r sqlcgen.Asset) entity.Asset {
	return entity.Asset{
		ID:         entity.AssetID(r.ID),
		VideoID:    entity.VideoID(r.VideoID),
		ChapterID:  toChapterID(r.ChapterID),
		Kind:       entity.AssetKind(r.Kind),
		Path:       r.Path,
		Size:       r.Size,
		MIME:       r.Mime,
		Provenance: r.Provenance,
		CreatedAt:  fromUnix(r.CreatedAt),
	}
}

func settingFromRow(r sqlcgen.Setting) entity.Setting {
	return entity.Setting{
		Key:         entity.SettingKey(r.Key),
		Value:       r.Value,
		Type:        entity.SettingType(r.Type),
		Group:       r.Grp,
		Description: r.Description,
		Min:         int(r.MinValue),
		Max:         int(r.MaxValue),
		UpdatedAt:   fromUnix(r.UpdatedAt),
	}
}

func taskFromRow(r sqlcgen.Task) entity.Task {
	return entity.Task{
		ID:            entity.TaskID(r.ID),
		VideoID:       entity.VideoID(r.VideoID),
		ChapterID:     toChapterID(r.ChapterID),
		Kind:          entity.TaskKind(r.Kind),
		Ordinal:       int(r.Ordinal),
		Index:         int(r.Idx),
		State:         entity.TaskState(r.State),
		Pool:          entity.Pool(r.Pool),
		Gate:          entity.GateKind(r.Gate),
		Attempt:       int(r.Attempt),
		MaxAttempts:   int(r.MaxAttempts),
		DepsRemaining: int(r.DepsRemaining),
		Stale:         r.Stale != 0,
		Error:         r.Error,
		CreatedAt:     fromUnix(r.CreatedAt),
		UpdatedAt:     fromUnix(r.UpdatedAt),
		StartedAt:     fromUnixPtr(r.StartedAt),
		FinishedAt:    fromUnixPtr(r.FinishedAt),
		NotBefore:     fromUnixPtr(r.NotBefore),
	}
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
