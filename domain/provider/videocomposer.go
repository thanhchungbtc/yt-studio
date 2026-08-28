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

// VideoComposer builds one chapter clip per call and joins them once.
type VideoComposer interface {
	Clip(ctx context.Context, req ClipRequest) (entity.AssetID, error)
	Concat(ctx context.Context, req ConcatRequest) (entity.AssetID, error)
}
