package provider

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
)

// ClipRequest asks for one chapter's composed clip. The titles are carried
// rather than looked up, which keeps the backend free of any repository.
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
	// OnPercent, when set, is called with whole percentages as the render
	// advances. Optional in both directions: a backend that composes nothing has
	// nothing to report, and a caller that is not watching passes nil.
	//
	// Every call happens inside Concat and none after it returns, so an
	// implementation may report straight to whatever is listening without
	// worrying about outliving it.
	OnPercent func(int)
}

// Render is the finished video: where it is, and where each chapter starts in
// it.
//
// The offsets come back with the asset for the same reason a narration's length
// does — this is the only moment they are known to describe it. They are not
// measured off the result either: a backend that crossfades its clips together
// had to decide where each one begins in order to do that, and ChapterOffsets
// is that decision rather than an inspection of the file afterwards.
//
// One entry per clip in ConcatRequest.ClipAssetIDs, in the order they were
// given, seconds from the start. Empty when the backend cannot say — a mock
// that returns a canned recording did not lay those clips end to end, and
// inventing offsets for it would put confident times on a video that has none.
type Render struct {
	AssetID        entity.AssetID
	ChapterOffsets []float64
}

// VideoComposer builds one chapter clip per call and joins them once.
type VideoComposer interface {
	Clip(ctx context.Context, req ClipRequest) (entity.AssetID, error)
	Concat(ctx context.Context, req ConcatRequest) (Render, error)
}
