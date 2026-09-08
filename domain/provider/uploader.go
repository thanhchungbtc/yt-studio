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
	// OnPercent, when set, is called with whole percentages as the bytes go up.
	// Optional in both directions, and called only from inside Upload — the same
	// contract ConcatRequest carries, for the same reason.
	OnPercent func(int)
}

// ListingRequest asks for a published video's listing to be corrected in
// place, with no bytes sent.
type ListingRequest struct {
	VideoRef    entity.Ref
	ChannelSlug entity.Slug
	// PublishedID is the id on the platform, from the video's upload record.
	// This is the whole reason the operation exists as its own call: there is
	// already a video out there, and this names it.
	PublishedID string
	Metadata    entity.Metadata
	DryRun      bool
}

// Uploader publishes a finished render, and corrects what it published.
type Uploader interface {
	Upload(ctx context.Context, req UploadRequest) (entity.UploadRecord, error)
	// UpdateListing pushes a corrected listing to a video already published.
	//
	// Separate from Upload rather than a mode of it because the two have
	// opposite hazards: an Upload run twice leaves two videos, and is guarded
	// against everywhere; this is idempotent, cheap, and safe to run as often as
	// somebody edits a title.
	UpdateListing(ctx context.Context, req ListingRequest) error
}
