package repository

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
)

// VideoFilter narrows a video listing. A zero filter lists everything.
type VideoFilter struct {
	ChannelID entity.ChannelID
	States    []entity.VideoState
	Limit     int
	Offset    int
}

// VideoReader reads videos by either key.
type VideoReader interface {
	VideoByID(ctx context.Context, id entity.VideoID) (entity.Video, error)
	VideoByRef(ctx context.Context, ref entity.Ref) (entity.Video, error)
	ListVideos(ctx context.Context, f VideoFilter) ([]entity.Video, error)
	CountVideos(ctx context.Context, f VideoFilter) (int, error)
}

// VideoWriter creates and updates videos.
type VideoWriter interface {
	CreateVideo(ctx context.Context, v entity.Video) error
	UpdateVideo(ctx context.Context, v entity.Video) error
	// DeleteVideo removes a video and everything that belongs to it — its
	// chapters, its asset rows and its task graph — as one transaction, and
	// returns the assets whose last owner it just removed.
	//
	// Those returned assets are the files the caller may unlink: their rows are
	// gone and no other video's row names the same content address. Anything
	// still shared with a surviving video is deliberately absent. The unlink
	// happens after this returns, never inside it — a crash in between leaves an
	// unreferenced file, which the sweep reclaims, where the other order would
	// leave a live row pointing at nothing.
	DeleteVideo(ctx context.Context, id entity.VideoID) ([]entity.Asset, error)
}

// VideoFieldWriter narrows writes to a single field, so a task that produced
// one artifact does not have to read and rewrite the whole row.
type VideoFieldWriter interface {
	SetVideoBlueprintAsset(ctx context.Context, id entity.VideoID, assetID entity.AssetID) error
	SetVideoFinalAsset(ctx context.Context, id entity.VideoID, assetID entity.AssetID) error
	SetVideoThumbnailPlan(ctx context.Context, id entity.VideoID, p entity.ThumbnailPlan) error
	// SetVideoThumbnailIcon writes one icon into the slot the plan sized for it,
	// so icons that finish out of order still land in their own cell.
	SetVideoThumbnailIcon(ctx context.Context, id entity.VideoID, index int, assetID entity.AssetID) error
	// SetVideoThumbnailCellPrompt replaces what one cell pictures, leaving the
	// caption the plan gave it alone.
	SetVideoThumbnailCellPrompt(ctx context.Context, id entity.VideoID, index int, prompt string) error
	SetVideoThumbnailAsset(ctx context.Context, id entity.VideoID, assetID entity.AssetID) error
	SetVideoMetadata(ctx context.Context, id entity.VideoID, m entity.Metadata) error
	SetVideoUpload(ctx context.Context, id entity.VideoID, r entity.UploadRecord) error
}

// VideoStateWriter is the scheduler's narrow lifecycle port: a derived state
// change is one row update, nothing more.
type VideoStateWriter interface {
	SetVideoState(ctx context.Context, id entity.VideoID, state entity.VideoState, errMsg string) error
}
