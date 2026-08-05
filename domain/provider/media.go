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

// SlideRequest asks for exactly one slide.
type SlideRequest struct {
	VideoID   entity.VideoID
	ChapterID entity.ChapterID
	Ordinal   int
	Index     int
	Prompt    string
	Width     int
	Height    int
}

// SlideProvider generates one slide per call.
type SlideProvider interface {
	Generate(ctx context.Context, req SlideRequest) (entity.AssetID, error)
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
	SlideAssetIDs []entity.AssetID
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

// ThumbnailIconRequest asks for exactly one tile's icon.
//
// It has no chapter and no ordinal: an icon belongs to the video's grid, not to
// a chapter, and it is square by definition. Reusing SlideRequest here would
// leave two thirds of that struct permanently empty.
type ThumbnailIconRequest struct {
	VideoID entity.VideoID
	// Index is which cell of the grid this is, 0-based.
	Index int
	// Prompt is the cell's subject and the grid's shared style clause, already
	// joined. The backend is handed exactly what was asked for, so a style change
	// produces a new content address rather than silently reusing old bytes.
	Prompt string
	// Size is the square edge in pixels.
	Size int
}

// ThumbnailIconGenerator generates one icon per call. It is its own port rather
// than a second use of SlideProvider because icons and chapter slides are
// selected independently: the cheap fast model that draws clean line art is
// rarely the one worth pointing at a three-hour video's slides.
type ThumbnailIconGenerator interface {
	Icon(ctx context.Context, req ThumbnailIconRequest) (entity.AssetID, error)
}

// ThumbnailIconCell is one rendered tile: what it says and what it shows.
type ThumbnailIconCell struct {
	Caption     string
	IconAssetID entity.AssetID
}

// ThumbnailRequest asks for the one image that fronts a finished video.
//
// Everything the backend renders is carried here, for the same reason
// ClipRequest carries its titles: it keeps the backend free of any repository.
// Cells are in grid order — reading order, left to right.
type ThumbnailRequest struct {
	VideoID  entity.VideoID
	VideoRef entity.Ref
	// Title is the video's own title, available to a template that wants it.
	Title string
	// Headline is the all-caps hook, the line the thumbnail is read by.
	Headline string
	Cells    []ThumbnailIconCell
}

// ThumbnailBuilder renders one video's thumbnail per call.
type ThumbnailBuilder interface {
	Build(ctx context.Context, req ThumbnailRequest) (entity.AssetID, error)
}

// UploadRequest asks for one video to be published.
type UploadRequest struct {
	VideoID      entity.VideoID
	VideoRef     entity.Ref
	ChannelSlug  entity.Slug
	FinalAssetID entity.AssetID
	// ThumbnailAssetID is the custom thumbnail to set on the published video.
	// YouTube takes it in a second call after the video resource exists, which is
	// the backend's business: publishing a listing is one unit of work.
	ThumbnailAssetID entity.AssetID
	Metadata         entity.Metadata
	DryRun           bool
}

// Uploader publishes a finished render.
type Uploader interface {
	Upload(ctx context.Context, req UploadRequest) (entity.UploadRecord, error)
}
