package sqlite

import (
	"context"
	"fmt"

	"github.com/tbui/yt-studio/adapters/sqlite/sqlcgen"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

var (
	_ repository.AssetReader = (*Store)(nil)
	_ repository.AssetWriter = (*Store)(nil)
)

// AssetByID reads asset metadata by content address.
func (s *Store) AssetByID(ctx context.Context, id entity.AssetID) (entity.Asset, error) {
	row, err := s.rq.GetAssetByID(ctx, string(id))
	if err != nil {
		return entity.Asset{}, wrapNotFound(err, "asset", string(id))
	}
	return assetFromRow(row), nil
}

// ListAssetsByVideo reads every asset produced for a video.
func (s *Store) ListAssetsByVideo(ctx context.Context, videoID entity.VideoID) ([]entity.Asset, error) {
	rows, err := s.rq.ListAssetsByVideo(ctx, string(videoID))
	if err != nil {
		return nil, fmt.Errorf("list assets of %s: %w", videoID, err)
	}
	out := make([]entity.Asset, 0, len(rows))
	for _, r := range rows {
		out = append(out, assetFromRow(r))
	}
	return out, nil
}

// PutAsset records asset metadata. It is an upsert by content address, so
// re-running a task that produced identical bytes is a no-op.
func (s *Store) PutAsset(ctx context.Context, a entity.Asset) error {
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.PutAsset(ctx, sqlcgen.PutAssetParams{
			ID:         string(a.ID),
			VideoID:    string(a.VideoID),
			ChapterID:  chapterIDPtr(a.ChapterID),
			Kind:       string(a.Kind),
			Path:       a.Path,
			Size:       a.Size,
			Mime:       a.MIME,
			Provenance: a.Provenance,
			CreatedAt:  toUnix(a.CreatedAt),
		})
	})
}
