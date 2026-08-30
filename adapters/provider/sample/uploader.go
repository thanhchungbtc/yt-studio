package sample

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// Uploader is the local publishing backend: it reads the final render through
// the asset store, so the upload path genuinely touches the bytes, and returns
// a receipt. A publish has no artifact to serve, so this is the one port where
// local means simulated.
//
// What is simulated is the *uplink*, and nothing else. A real publish is bounded
// by bandwidth; this machine has none to be bounded by, so a rate is imposed and
// the bytes are then really read and really counted against the size the store
// reports. The percentage that comes out is a measurement, not a script — which
// is the only kind worth testing a progress bar against.
type Uploader struct {
	store provider.AssetStore
	// now is injectable so a receipt is reproducible.
	now func() time.Time
	// megabytesPerSecond is read per call, so the rate can be changed while the
	// server runs. Nil or zero means as fast as the disk allows.
	megabytesPerSecond func() int
}

var _ provider.Uploader = (*Uploader)(nil)

// NewUploader constructs the backend.
func NewUploader(store provider.AssetStore, now func() time.Time, megabytesPerSecond func() int) *Uploader {
	if now == nil {
		now = time.Now
	}
	return &Uploader{store: store, now: now, megabytesPerSecond: megabytesPerSecond}
}

// uploadChunk is how much is read between two rate-limit pauses. Small enough
// that the pacing is smooth at any plausible rate, large enough that a
// few-hundred-megabyte render is not a million syscalls.
const uploadChunk = 256 << 10

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

	if err := u.send(ctx, req, info.Size); err != nil {
		return entity.UploadRecord{}, err
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

// send reads the whole render at the configured rate, reporting as it goes.
//
// The bytes are discarded. Reading them is the part of a publish a receipt
// cannot prove — a store that has lost the file fails here rather than three
// steps later — and there is nowhere local for them to go afterwards.
//
// The pause is a select rather than a sleep because at a few megabytes a second
// this is the longest-running task in the pipeline, and a cancelled video that
// went on uploading for another minute would be holding a pool slot against an
// operator who has already changed their mind.
func (u *Uploader) send(ctx context.Context, req provider.UploadRequest, size int64) error {
	file, err := u.store.Open(ctx, req.FinalAssetID, entity.AssetKindFinal)
	if err != nil {
		return fmt.Errorf("open final render: %w", err)
	}
	defer func() { _ = file.Close() }()

	var perSecond int64
	if u.megabytesPerSecond != nil {
		if mb := u.megabytesPerSecond(); mb > 0 {
			perSecond = int64(mb) << 20
		}
	}

	buf := make([]byte, uploadChunk)
	var sent int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := file.Read(buf)
		if n > 0 {
			sent += int64(n)
			if req.OnPercent != nil {
				req.OnPercent(int(sent * 100 / size))
			}
			if perSecond > 0 {
				pause := time.Duration(int64(n) * int64(time.Second) / perSecond)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(pause):
				}
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("read final render: %w", readErr)
		}
	}
}
