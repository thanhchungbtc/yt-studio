package provider

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
)

// UploadRequest asks for one video to be published.
type UploadRequest struct {
	VideoID      entity.VideoID
	VideoRef     entity.Ref
	ChannelSlug  entity.Slug
	FinalAssetID entity.AssetID
	// ThumbnailAssetID is the custom thumbnail. YouTube takes it in a second call
	// after the video exists, which is the backend's business — publishing a
	// listing is one unit of work.
	ThumbnailAssetID entity.AssetID
	Metadata         entity.Metadata
	DryRun           bool
}

// Uploader publishes a finished render.
type Uploader interface {
	Upload(ctx context.Context, req UploadRequest) (entity.UploadRecord, error)
}
