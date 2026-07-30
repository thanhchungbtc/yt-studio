package sqlite

import (
	"context"
	"fmt"

	"github.com/tbui/yt-studio/adapters/sqlite/sqlcgen"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

var (
	_ repository.AssetReader     = (*Store)(nil)
	_ repository.AssetWriter     = (*Store)(nil)
	_ repository.AssetMaintainer = (*Store)(nil)
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

// PutAsset records one video's ownership of a content address. It is an upsert
// on (address, video), so re-running a task that produced identical bytes is a
// no-op, and a second video that produces the same bytes gets its own row rather
// than silently borrowing the first one's.
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

// MissingAssetOwners lists references to a content address that no asset row
// records, which is what the ownership repair puts right.
func (s *Store) MissingAssetOwners(ctx context.Context) ([]repository.AssetOwnerRef, error) {
	rows, err := s.rq.ListMissingAssetOwners(ctx)
	if err != nil {
		return nil, fmt.Errorf("list missing asset owners: %w", err)
	}
	out := make([]repository.AssetOwnerRef, 0, len(rows))
	for _, r := range rows {
		// The column is nullable to SQL's eyes because it comes out of a union of
		// nullable columns; the query already filters the nulls away.
		if r.AssetID == nil || *r.AssetID == "" {
			continue
		}
		out = append(out, repository.AssetOwnerRef{
			VideoID:   entity.VideoID(r.VideoID),
			ChapterID: toChapterID(r.ChapterID),
			AssetID:   entity.AssetID(*r.AssetID),
			Kind:      entity.AssetKind(r.Kind),
		})
	}
	return out, nil
}

// AssetAddresses lists every content address with at least one owner.
func (s *Store) AssetAddresses(ctx context.Context) ([]repository.AssetAddress, error) {
	rows, err := s.rq.ListAssetAddresses(ctx)
	if err != nil {
		return nil, fmt.Errorf("list asset addresses: %w", err)
	}
	out := make([]repository.AssetAddress, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.AssetAddress{
			ID:   entity.AssetID(r.ID),
			Kind: entity.AssetKind(r.Kind),
		})
	}
	return out, nil
}
