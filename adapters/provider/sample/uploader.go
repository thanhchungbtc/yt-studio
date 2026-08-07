package sample

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// Uploader is the local publishing backend: it reads the final render through
// the asset store, so the upload path genuinely touches the bytes, and returns
// a receipt. A publish has no artifact to serve, so this is the one port where
// local means simulated.
type Uploader struct {
	store provider.AssetStore
	// now is injectable so a receipt is reproducible.
	now func() time.Time
}

var _ provider.Uploader = (*Uploader)(nil)

// NewUploader constructs the backend.
func NewUploader(store provider.AssetStore, now func() time.Time) *Uploader {
	if now == nil {
		now = time.Now
	}
	return &Uploader{store: store, now: now}
}

// Upload publishes one finished render.
func (u *Uploader) Upload(ctx context.Context, req provider.UploadRequest) (entity.UploadRecord, error) {
	info, err := u.store.Stat(ctx, req.FinalAssetID, entity.AssetKindFinal)
	if err != nil {
		return entity.UploadRecord{}, fmt.Errorf("stat final render: %w", err)
	}
	if info.Size == 0 {
		return entity.UploadRecord{}, fmt.Errorf("final render %s is empty", req.FinalAssetID.Short())
	}
	// The thumbnail is read for the same reason the render is: a real backend
	// sets it in a second call, and a backend that never touched the file would
	// hide a thumbnail that was recorded but never stored.
	if req.ThumbnailAssetID != "" {
		if _, err := u.store.Stat(ctx, req.ThumbnailAssetID, entity.AssetKindThumbnail); err != nil {
			return entity.UploadRecord{}, fmt.Errorf("stat thumbnail: %w", err)
		}
	}

	// A stable pseudo-video-id derived from the content address, so re-running an
	// upload of identical bytes yields an identical receipt.
	seed := seedOf(string(req.FinalAssetID), string(req.VideoRef))
	remoteID := "sample-" + strconv.FormatUint(seed, 36)

	return entity.UploadRecord{
		VideoID: remoteID,
		URL:     "https://www.youtube.com/watch?v=" + remoteID,
		// Always dry, whatever the request said. upload.dry_run asks a real
		// backend to stop short of publishing; this one never publishes at all,
		// and a receipt claiming otherwise would be the one record in the database
		// that lies about whether something reached YouTube.
		DryRun:     true,
		UploadedAt: u.now().UTC(),
	}, nil
}
