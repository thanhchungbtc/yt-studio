package provider

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
)

// IconRequest asks for exactly one tile's icon. No chapter, no ordinal: an icon
// belongs to the video's grid and is square by definition, so SlideRequest
// would be two thirds empty here.
type IconRequest struct {
	VideoID entity.VideoID
	// Index is which cell of the grid this is, 0-based.
	Index int
	// Prompt is the cell's subject and the shared style clause, already joined,
	// so a style change produces a new content address rather than reusing bytes.
	Prompt string
	// Size is the square edge in pixels.
	Size int
}

// IconGenerator generates one icon per call. Its own port because icons and
// slides are selected independently: the fast model that draws clean line art
// is rarely the one worth pointing at a three-hour video's slides.
type IconGenerator interface {
	Generate(ctx context.Context, req IconRequest) (entity.AssetID, error)
}
