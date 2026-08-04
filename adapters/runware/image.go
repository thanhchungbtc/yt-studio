package runware

import (
	"bytes"
	"context"
	"fmt"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// Image generates one chapter still per call.
type Image struct{ c *Client }

var _ provider.ImageProvider = (*Image)(nil)

// NewImage wires the still backend to a client.
func NewImage(c *Client) *Image { return &Image{c: c} }

// Generate draws one still and returns its content address.
func (i *Image) Generate(ctx context.Context, req provider.ImageRequest) (entity.AssetID, error) {
	width, height := req.Width, req.Height
	// The request's geometry wins when it carries any, and the configured size is
	// the fallback rather than the rule: the port declares the fields, so a use
	// case that starts filling them must be obeyed rather than overridden here.
	if width <= 0 || height <= 0 {
		width, height = i.c.cfg.StillSize()
	}

	image, err := i.c.generate(ctx, req.Prompt, width, height, defaultNegativePrompt)
	if err != nil {
		return "", err
	}
	stored, err := i.c.store.Put(ctx, entity.AssetKindImage, bytes.NewReader(image))
	if err != nil {
		return "", fmt.Errorf("store still: %w", err)
	}
	return stored.ID, nil
}
