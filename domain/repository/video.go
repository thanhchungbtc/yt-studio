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
	DeleteVideo(ctx context.Context, id entity.VideoID) error
}

// VideoFieldWriter narrows writes to a single field, so a task that produced
// one artifact does not have to read and rewrite the whole row.
type VideoFieldWriter interface {
	SetVideoBlueprintAsset(ctx context.Context, id entity.VideoID, assetID entity.AssetID) error
	SetVideoFinalAsset(ctx context.Context, id entity.VideoID, assetID entity.AssetID) error
	SetVideoMetadata(ctx context.Context, id entity.VideoID, m entity.Metadata) error
	SetVideoUpload(ctx context.Context, id entity.VideoID, r entity.UploadRecord) error
}

// VideoStateWriter is the scheduler's narrow lifecycle port: a derived state
// change is one row update, nothing more.
type VideoStateWriter interface {
	SetVideoState(ctx context.Context, id entity.VideoID, state entity.VideoState, errMsg string) error
}
