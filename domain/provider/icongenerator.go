package provider

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
)

// IconRequest asks for exactly one tile's icon.
//
// It has no chapter and no ordinal: an icon belongs to the video's grid, not to
// a chapter, and it is square by definition. Reusing SlideRequest here would
// leave two thirds of that struct permanently empty.
type IconRequest struct {
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

// IconGenerator generates one icon per call. It is its own port rather
// than a second use of SlideGenerator because icons and chapter slides are
// selected independently: the cheap fast model that draws clean line art is
// rarely the one worth pointing at a three-hour video's slides.
type IconGenerator interface {
	Generate(ctx context.Context, req IconRequest) (entity.AssetID, error)
}
