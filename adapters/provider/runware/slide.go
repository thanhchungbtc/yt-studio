package runware

import (
	"bytes"
	"context"
	"fmt"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// Slide generates one chapter slide per call.
type Slide struct{ c *Client }

var _ provider.SlideGenerator = (*Slide)(nil)

// NewSlide wires the slide backend to a client.
func NewSlide(c *Client) *Slide { return &Slide{c: c} }

// Generate draws one slide and returns its content address.
func (i *Slide) Generate(ctx context.Context, req provider.SlideRequest) (entity.AssetID, error) {
	width, height := req.Width, req.Height
	// The request's geometry wins when it carries any: the port declares those
	// fields, so a use case that fills them must be obeyed, not overridden.
	if width <= 0 || height <= 0 {
		width, height = i.c.cfg.SlideSize()
	}

	image, err := i.c.generate(ctx, req.Prompt, width, height, defaultNegativePrompt)
	if err != nil {
		return "", err
	}
	stored, err := i.c.store.Put(ctx, entity.AssetKindImage, bytes.NewReader(image))
	if err != nil {
		return "", fmt.Errorf("store slide: %w", err)
	}
	return stored.ID, nil
}
