package sqlite

import (
	"context"
	"fmt"
	"strconv"
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
		// More than one state filter is applied here rather than in SQL: the list is
		// paged and small, and it keeps the query plan a single index range scan.
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
	blobs, err := videoJSON(v)
	if err != nil {
		return sqlcgen.CreateVideoParams{}, err
	}
	metadata, upload, plan, icons := blobs.metadata, blobs.upload, blobs.plan, blobs.icons
	return sqlcgen.CreateVideoParams{
		ID:                    string(v.ID),
		ChannelID:             string(v.ChannelID),
		Ref:                   string(v.Ref),
		Title:                 v.Title,
		Topic:                 v.Topic,
		State:                 string(v.State),
		ChapterCount:          int64(v.ChapterCount),
		SlidesPerChapter:      int64(v.SlidesPerChapter),
		TargetDurationMinutes: int64(v.TargetDurationMinutes),
		ThumbnailCells:        int64(v.ThumbnailCells),
		BlueprintAssetID:      assetIDPtr(v.BlueprintAssetID),
		FinalAssetID:          assetIDPtr(v.FinalAssetID),
		ThumbnailAssetID:      assetIDPtr(v.ThumbnailAssetID),
		ThumbnailPlanJson:     plan,
		ThumbnailIconIdsJson:  icons,
		MetadataJson:          metadata,
		UploadJson:            upload,
		Error:                 v.Error,
		CreatedAt:             toUnix(v.CreatedAt),
		UpdatedAt:             toUnix(v.UpdatedAt),
		StartedAt:             toUnixPtr(v.StartedAt),
		CompletedAt:           toUnixPtr(v.CompletedAt),
	}, nil
}

// videoBlobs are the video's JSON-valued columns, encoded once for whichever
// statement is about to write them.
type videoBlobs struct {
	metadata *string
	upload   *string
	plan     *string
	icons    string
}

func videoJSON(v entity.Video) (videoBlobs, error) {
	var b videoBlobs
	if v.Metadata != nil {
		s, err := encodeJSON(v.Metadata)
		if err != nil {
			return videoBlobs{}, fmt.Errorf("encode video metadata: %w", err)
		}
		b.metadata = strPtr(s)
	}
	if v.Upload != nil {
		s, err := encodeJSON(v.Upload)
		if err != nil {
			return videoBlobs{}, fmt.Errorf("encode upload record: %w", err)
		}
		b.upload = strPtr(s)
	}
	if v.ThumbnailPlan != nil {
		s, err := encodeJSON(v.ThumbnailPlan)
		if err != nil {
			return videoBlobs{}, fmt.Errorf("encode thumbnail plan: %w", err)
		}
		b.plan = strPtr(s)
	}
	// The column is NOT NULL: an unwritten grid is an empty array, never null,
	// so the indexed setter always has a document to write into.
	icons := v.ThumbnailIconAssetIDs
	if icons == nil {
		icons = []entity.AssetID{}
	}
	s, err := encodeJSON(icons)
	if err != nil {
		return videoBlobs{}, fmt.Errorf("encode thumbnail icon ids: %w", err)
	}
	b.icons = s
	return b, nil
}

// UpdateVideo writes back every mutable field of a video.
func (s *Store) UpdateVideo(ctx context.Context, v entity.Video) error {
	blobs, err := videoJSON(v)
	if err != nil {
		return err
	}
	metadata, upload, plan, icons := blobs.metadata, blobs.upload, blobs.plan, blobs.icons
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.UpdateVideo(ctx, sqlcgen.UpdateVideoParams{
			Title:                 v.Title,
			Topic:                 v.Topic,
			State:                 string(v.State),
			ChapterCount:          int64(v.ChapterCount),
			SlidesPerChapter:      int64(v.SlidesPerChapter),
			TargetDurationMinutes: int64(v.TargetDurationMinutes),
			ThumbnailCells:        int64(v.ThumbnailCells),
			BlueprintAssetID:      assetIDPtr(v.BlueprintAssetID),
			FinalAssetID:          assetIDPtr(v.FinalAssetID),
			ThumbnailAssetID:      assetIDPtr(v.ThumbnailAssetID),
			ThumbnailPlanJson:     plan,
			ThumbnailIconIdsJson:  icons,
			MetadataJson:          metadata,
			UploadJson:            upload,
			Error:                 v.Error,
			UpdatedAt:             toUnix(v.UpdatedAt),
			StartedAt:             toUnixPtr(v.StartedAt),
			CompletedAt:           toUnixPtr(v.CompletedAt),
			ID:                    string(v.ID),
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
	// started_at is COALESCE'd in SQL, so the first non-draft transition stamps it
	// and every later one leaves it alone.
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

// DeleteVideo removes a video and everything below it in one transaction, and
// reports which files that left unreferenced.
//
// Chapters and asset rows go by foreign key; tasks and their edges are deleted
// explicitly, because a task is keyed by video without a constraint to hang a
// cascade on. Doing it as one transaction is what keeps a failure from leaving a
// video with no graph, or a graph with no video.
//
// The owner count is asked *after* the rows are gone, so an address with no
// owners left is one nothing surviving can reach. An address another video still
// owns is simply not returned, and its file stays where it is.
func (s *Store) DeleteVideo(ctx context.Context, id entity.VideoID) ([]entity.Asset, error) {
	var unreferenced []entity.Asset
	err := s.doTx(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		// Reset per attempt: the caller sees the result of the commit that stuck.
		unreferenced = nil

		owned, err := q.ListAssetsByVideo(ctx, string(id))
		if err != nil {
			return fmt.Errorf("list assets of %s: %w", id, err)
		}
		if err := q.DeleteTaskDepsByVideo(ctx, string(id)); err != nil {
			return fmt.Errorf("delete deps of %s: %w", id, err)
		}
		if err := q.DeleteTasksByVideo(ctx, string(id)); err != nil {
			return fmt.Errorf("delete tasks of %s: %w", id, err)
		}
		if err := q.DeleteVideo(ctx, string(id)); err != nil {
			return fmt.Errorf("delete video %s: %w", id, err)
		}
		for _, row := range owned {
			owners, err := q.CountAssetOwners(ctx, row.ID)
			if err != nil {
				return fmt.Errorf("count owners of asset %s: %w", row.ID, err)
			}
			if owners == 0 {
				unreferenced = append(unreferenced, assetFromRow(row))
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return unreferenced, nil
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

// SetVideoThumbnailPlan records the grid and sizes the slots its icons will
// land in. Both in one statement: a plan whose slot array still belongs to the
// plan before it would put an icon in the wrong cell.
func (s *Store) SetVideoThumbnailPlan(ctx context.Context, id entity.VideoID, p entity.ThumbnailPlan) error {
	encoded, err := encodeJSON(p)
	if err != nil {
		return fmt.Errorf("encode thumbnail plan: %w", err)
	}
	slots, err := encodeJSON(make([]entity.AssetID, len(p.Cells)))
	if err != nil {
		return fmt.Errorf("encode thumbnail icon ids: %w", err)
	}
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.SetVideoThumbnailPlan(ctx, sqlcgen.SetVideoThumbnailPlanParams{
			ThumbnailPlanJson:    &encoded,
			ThumbnailIconIdsJson: slots,
			UpdatedAt:            toUnix(time.Now()),
			ID:                   string(id),
		})
	})
}

// SetVideoThumbnailIcon records one icon at its index. json_set makes this a
// single atomic statement, so two concurrent icon tasks cannot lose each
// other's write.
func (s *Store) SetVideoThumbnailIcon(ctx context.Context, id entity.VideoID, index int, assetID entity.AssetID) error {
	if index < 0 {
		return fmt.Errorf("%w: icon index must not be negative", entity.ErrInvalidVideo)
	}
	path := "$[" + strconv.Itoa(index) + "]"
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.SetVideoThumbnailIcon(ctx, sqlcgen.SetVideoThumbnailIconParams{
			Path:      path,
			AssetID:   string(assetID),
			UpdatedAt: toUnix(time.Now()),
			ID:        string(id),
		})
	})
}

// SetVideoThumbnailCellPrompt replaces what one cell pictures.
//
// The path reaches into the encoded plan rather than rewriting it, so the
// caption stays as the plan wrote it and an icon generating right now cannot
// have its cell replaced underneath it. Go field names, because the plan is
// marshalled without json tags.
func (s *Store) SetVideoThumbnailCellPrompt(ctx context.Context, id entity.VideoID, index int, prompt string) error {
	if index < 0 {
		return fmt.Errorf("%w: cell index must not be negative", entity.ErrInvalidVideo)
	}
	path := "$.Cells[" + strconv.Itoa(index) + "].Prompt"
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.SetVideoThumbnailCellPrompt(ctx, sqlcgen.SetVideoThumbnailCellPromptParams{
			Path:      path,
			Prompt:    prompt,
			UpdatedAt: toUnix(time.Now()),
			ID:        string(id),
		})
	})
}

// SetVideoThumbnailAsset records the image that fronts the video.
func (s *Store) SetVideoThumbnailAsset(ctx context.Context, id entity.VideoID, assetID entity.AssetID) error {
	value := string(assetID)
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.SetVideoThumbnailAsset(ctx, sqlcgen.SetVideoThumbnailAssetParams{
			ThumbnailAssetID: &value,
			UpdatedAt:        toUnix(time.Now()),
			ID:               string(id),
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
