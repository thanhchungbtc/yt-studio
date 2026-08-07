package provider

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
)

// IconCell is one rendered tile: what it says and what it shows.
type IconCell struct {
	Caption     string
	IconAssetID entity.AssetID
}

// ThumbnailRequest asks for the image that fronts a finished video. Everything
// the backend renders is carried here, keeping it free of any repository. Cells
// are in reading order, left to right.
type ThumbnailRequest struct {
	VideoID  entity.VideoID
	VideoRef entity.Ref
	// Title is the video's own title, available to a template that wants it.
	Title string
	// Headline is the all-caps hook, the line the thumbnail is read by.
	Headline string
	Cells    []IconCell
}

// ThumbnailRenderer renders one video's thumbnail per call.
type ThumbnailRenderer interface {
	Render(ctx context.Context, req ThumbnailRequest) (entity.AssetID, error)
}
