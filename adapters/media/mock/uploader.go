package mock

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/tbui/yt-studio/adapters/mockcore"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// Uploader is the mock publishing backend. It reads the final render through
// the asset store so the upload path genuinely touches the bytes, and returns a
// receipt. Dry run is the default and stays the default.
type Uploader struct {
	store  provider.AssetStore
	tuning Tuning
	// now is injectable so golden-file tests get a stable receipt.
	now func() time.Time
}

var _ provider.Uploader = (*Uploader)(nil)

// NewUploader constructs the mock.
func NewUploader(store provider.AssetStore, tuning Tuning, now func() time.Time) *Uploader {
	if now == nil {
		now = time.Now
	}
	return &Uploader{store: store, tuning: tuning, now: now}
}

// Upload publishes one finished render.
func (u *Uploader) Upload(ctx context.Context, req provider.UploadRequest) (entity.UploadRecord, error) {
	if err := mockcore.Simulate(ctx, u.tuning, 3); err != nil {
		return entity.UploadRecord{}, err
	}
	info, err := u.store.Stat(ctx, req.FinalAssetID, entity.AssetKindFinal)
	if err != nil {
		return entity.UploadRecord{}, fmt.Errorf("stat final render: %w", err)
	}
	if info.Size == 0 {
		return entity.UploadRecord{}, fmt.Errorf("final render %s is empty", req.FinalAssetID.Short())
	}
	// The thumbnail is read for the same reason the render is: a real backend
	// sets it in a second call, and a mock that never touched the file would hide
	// a thumbnail that was recorded but never stored.
	if req.ThumbnailAssetID != "" {
		if _, err := u.store.Stat(ctx, req.ThumbnailAssetID, entity.AssetKindThumbnail); err != nil {
			return entity.UploadRecord{}, fmt.Errorf("stat thumbnail: %w", err)
		}
	}

	// A stable pseudo-video-id derived from the content address, so re-running an
	// upload of identical bytes yields an identical receipt.
	seed := mockcore.SeedOf(string(req.FinalAssetID), string(req.VideoRef))
	remoteID := "mock-" + strconv.FormatUint(seed, 36)

	return entity.UploadRecord{
		VideoID:    remoteID,
		URL:        "https://www.youtube.com/watch?v=" + remoteID,
		DryRun:     req.DryRun,
		UploadedAt: u.now().UTC(),
	}, nil
}
