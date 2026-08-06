package provider

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
)

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

// SlideGenerator generates one slide per call.
type SlideGenerator interface {
	Generate(ctx context.Context, req SlideRequest) (entity.AssetID, error)
}
