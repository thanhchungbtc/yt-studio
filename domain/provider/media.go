package provider

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
)

// SpeakRequest asks for the narration of exactly one chapter.
type SpeakRequest struct {
	VideoID   entity.VideoID
	ChapterID entity.ChapterID
	Ordinal   int
	Text      string
}

// TTSProvider narrates one chapter per call.
type TTSProvider interface {
	Speak(ctx context.Context, req SpeakRequest) (entity.AssetID, error)
}

// ImageRequest asks for exactly one still.
type ImageRequest struct {
	VideoID   entity.VideoID
	ChapterID entity.ChapterID
	Ordinal   int
	Index     int
	Prompt    string
	Width     int
	Height    int
}

// ImageProvider generates one still per call.
type ImageProvider interface {
	Generate(ctx context.Context, req ImageRequest) (entity.AssetID, error)
}

// ClipRequest asks for one chapter's composed clip.
//
// The titles are carried rather than looked up: a composer that burns text into
// a frame needs the text, and passing it keeps the backend free of any
// repository.
type ClipRequest struct {
	VideoID       entity.VideoID
	ChapterID     entity.ChapterID
	Ordinal       int
	ChapterTitle  string
	VideoTitle    string
	AudioAssetID  entity.AssetID
	ImageAssetIDs []entity.AssetID
}

// ConcatRequest asks for the final render.
type ConcatRequest struct {
	VideoID      entity.VideoID
	ClipAssetIDs []entity.AssetID
}

// VideoComposer builds one chapter clip per call and joins them once.
type VideoComposer interface {
	Clip(ctx context.Context, req ClipRequest) (entity.AssetID, error)
	Concat(ctx context.Context, req ConcatRequest) (entity.AssetID, error)
}

// UploadRequest asks for one video to be published.
type UploadRequest struct {
	VideoID      entity.VideoID
	VideoRef     entity.Ref
	ChannelSlug  entity.Slug
	FinalAssetID entity.AssetID
	Metadata     entity.Metadata
	DryRun       bool
}

// Uploader publishes a finished render.
type Uploader interface {
	Upload(ctx context.Context, req UploadRequest) (entity.UploadRecord, error)
}
