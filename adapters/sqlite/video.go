package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/adapters/sqlite/sqlcgen"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

var (
	_ repository.VideoReader = (*Store)(nil)
	_ repository.VideoWriter = (*Store)(nil)
)

// VideoByID reads one video by its opaque id.
func (s *Store) VideoByID(ctx context.Context, id entity.VideoID) (entity.Video, error) {
	row, err := s.rq.GetVideoByID(ctx, string(id))
	if err != nil {
		return entity.Video{}, wrapNotFound(err, "video", string(id))
	}
	return videoFromRow(row)
}

// VideoByRef reads one video by its natural key, e.g. DSS-14.
func (s *Store) VideoByRef(ctx context.Context, ref entity.Ref) (entity.Video, error) {
	row, err := s.rq.GetVideoByRef(ctx, string(ref))
	if err != nil {
		return entity.Video{}, wrapNotFound(err, "video", string(ref))
	}
	return videoFromRow(row)
}

// ListVideos reads a page of videos, newest first.
func (s *Store) ListVideos(ctx context.Context, f repository.VideoFilter) ([]entity.Video, error) {
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	state := ""
	if len(f.States) == 1 {
		state = string(f.States[0])
	}
	rows, err := s.rq.ListVideos(ctx, sqlcgen.ListVideosParams{
		ChannelID: string(f.ChannelID),
		State:     state,
		Lim:       int64(limit),
		Off:       int64(f.Offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list videos: %w", err)
	}
	out := make([]entity.Video, 0, len(rows))
	for _, r := range rows {
		v, err := videoFromRow(r)
		if err != nil {
			return nil, fmt.Errorf("decode video %s: %w", r.ID, err)
		}
		// More than one state filter is applied here rather than in SQL: the
		// list is paged and small, and it keeps the query plan a single index
		// range scan.
		if len(f.States) > 1 && !containsState(f.States, v.State) {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}

func containsState(states []entity.VideoState, s entity.VideoState) bool {
	for _, x := range states {
		if x == s {
			return true
		}
	}
	return false
}

// CountVideos counts videos matching the filter.
func (s *Store) CountVideos(ctx context.Context, f repository.VideoFilter) (int, error) {
	state := ""
	if len(f.States) == 1 {
		state = string(f.States[0])
	}
	n, err := s.rq.CountVideos(ctx, sqlcgen.CountVideosParams{
		ChannelID: string(f.ChannelID),
		State:     state,
	})
	if err != nil {
		return 0, fmt.Errorf("count videos: %w", err)
	}
	return int(n), nil
}

// CreateVideo inserts a new video.
func (s *Store) CreateVideo(ctx context.Context, v entity.Video) error {
	params, err := createVideoParams(v)
	if err != nil {
		return err
	}
	if err := s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.CreateVideo(ctx, params)
	}); err != nil {
		return wrapConflict(err, "video", string(v.Ref))
	}
	return nil
}

func createVideoParams(v entity.Video) (sqlcgen.CreateVideoParams, error) {
	metadata, upload, err := videoJSON(v)
	if err != nil {
		return sqlcgen.CreateVideoParams{}, err
	}
	return sqlcgen.CreateVideoParams{
		ID:               string(v.ID),
		ChannelID:        string(v.ChannelID),
		Ref:              string(v.Ref),
		Title:            v.Title,
		Topic:            v.Topic,
		State:            string(v.State),
		ChapterCount:     int64(v.ChapterCount),
		ImagesPerChapter: int64(v.ImagesPerChapter),
		BlueprintAssetID: assetIDPtr(v.BlueprintAssetID),
		FinalAssetID:     assetIDPtr(v.FinalAssetID),
		MetadataJson:     metadata,
		UploadJson:       upload,
		Error:            v.Error,
		CreatedAt:        toUnix(v.CreatedAt),
		UpdatedAt:        toUnix(v.UpdatedAt),
		StartedAt:        toUnixPtr(v.StartedAt),
		CompletedAt:      toUnixPtr(v.CompletedAt),
	}, nil
}

func videoJSON(v entity.Video) (metadata, upload *string, err error) {
	if v.Metadata != nil {
		s, err := encodeJSON(v.Metadata)
		if err != nil {
			return nil, nil, fmt.Errorf("encode video metadata: %w", err)
		}
		metadata = strPtr(s)
	}
	if v.Upload != nil {
		s, err := encodeJSON(v.Upload)
		if err != nil {
			return nil, nil, fmt.Errorf("encode upload record: %w", err)
		}
		upload = strPtr(s)
	}
	return metadata, upload, nil
}

// UpdateVideo writes back every mutable field of a video.
func (s *Store) UpdateVideo(ctx context.Context, v entity.Video) error {
	metadata, upload, err := videoJSON(v)
	if err != nil {
		return err
	}
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.UpdateVideo(ctx, sqlcgen.UpdateVideoParams{
			Title:            v.Title,
			Topic:            v.Topic,
			State:            string(v.State),
			ChapterCount:     int64(v.ChapterCount),
			ImagesPerChapter: int64(v.ImagesPerChapter),
			BlueprintAssetID: assetIDPtr(v.BlueprintAssetID),
			FinalAssetID:     assetIDPtr(v.FinalAssetID),
			MetadataJson:     metadata,
			UploadJson:       upload,
			Error:            v.Error,
			UpdatedAt:        toUnix(v.UpdatedAt),
			StartedAt:        toUnixPtr(v.StartedAt),
			CompletedAt:      toUnixPtr(v.CompletedAt),
			ID:               string(v.ID),
		})
	})
}

// SetVideoState implements the scheduler's narrow lifecycle port: a derived
// state change is one row update, nothing more.
func (s *Store) SetVideoState(ctx context.Context, id entity.VideoID, state entity.VideoState, errMsg string) error {
	now := time.Now()
	var completedAt *int64
	if state.Terminal() {
		n := toUnix(now)
		completedAt = &n
	}
	// started_at is COALESCE'd in SQL, so the first non-draft transition stamps
	// it and every later one leaves it alone.
	var startedAt *int64
	if state != entity.VideoStateDraft {
		n := toUnix(now)
		startedAt = &n
	}
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.SetVideoState(ctx, sqlcgen.SetVideoStateParams{
			State:       string(state),
			Error:       errMsg,
			UpdatedAt:   toUnix(now),
			StartedAt:   startedAt,
			CompletedAt: completedAt,
			ID:          string(id),
		})
	})
}

// DeleteVideo removes a video and, by cascade, its chapters.
func (s *Store) DeleteVideo(ctx context.Context, id entity.VideoID) error {
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.DeleteVideo(ctx, string(id))
	})
}

var (
	_ repository.VideoFieldWriter = (*Store)(nil)
	_ repository.VideoStateWriter = (*Store)(nil)
)

// SetVideoBlueprintAsset records the approved outline's artifact.
func (s *Store) SetVideoBlueprintAsset(ctx context.Context, id entity.VideoID, assetID entity.AssetID) error {
	value := string(assetID)
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.SetVideoBlueprintAsset(ctx, sqlcgen.SetVideoBlueprintAssetParams{
			BlueprintAssetID: &value,
			UpdatedAt:        toUnix(time.Now()),
			ID:               string(id),
		})
	})
}

// SetVideoFinalAsset records the final render.
func (s *Store) SetVideoFinalAsset(ctx context.Context, id entity.VideoID, assetID entity.AssetID) error {
	value := string(assetID)
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.SetVideoFinalAsset(ctx, sqlcgen.SetVideoFinalAssetParams{
			FinalAssetID: &value,
			UpdatedAt:    toUnix(time.Now()),
			ID:           string(id),
		})
	})
}

// SetVideoMetadata records the generated listing.
func (s *Store) SetVideoMetadata(ctx context.Context, id entity.VideoID, m entity.Metadata) error {
	encoded, err := encodeJSON(m)
	if err != nil {
		return fmt.Errorf("encode video metadata: %w", err)
	}
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.SetVideoMetadata(ctx, sqlcgen.SetVideoMetadataParams{
			MetadataJson: &encoded,
			UpdatedAt:    toUnix(time.Now()),
			ID:           string(id),
		})
	})
}

// SetVideoUpload records the upload receipt.
func (s *Store) SetVideoUpload(ctx context.Context, id entity.VideoID, r entity.UploadRecord) error {
	encoded, err := encodeJSON(r)
	if err != nil {
		return fmt.Errorf("encode upload record: %w", err)
	}
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.SetVideoUpload(ctx, sqlcgen.SetVideoUploadParams{
			UploadJson: &encoded,
			UpdatedAt:  toUnix(time.Now()),
			ID:         string(id),
		})
	})
}
