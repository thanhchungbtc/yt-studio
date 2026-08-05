package runware

import (
	"bytes"
	"context"
	"fmt"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// Icon generates one thumbnail grid tile per call.
type Icon struct{ c *Client }

var _ provider.ThumbnailIconGenerator = (*Icon)(nil)

// NewIcon wires the icon backend to a client.
func NewIcon(c *Client) *Icon { return &Icon{c: c} }

// Icon draws one tile and returns its content address.
//
// The size on the request is used for both edges: an icon is square by the
// port's definition, and asking the model for the square directly is what keeps
// the renderer from scaling a rectangle into one.
func (i *Icon) Icon(ctx context.Context, req provider.ThumbnailIconRequest) (entity.AssetID, error) {
	size := req.Size
	if size < 1 {
		size = defaultIconSize
	}

	image, err := i.c.generate(ctx, req.Prompt, size, size, defaultNegativePrompt)
	if err != nil {
		return "", err
	}
	stored, err := i.c.store.Put(ctx, entity.AssetKindThumbnailIcon, bytes.NewReader(image))
	if err != nil {
		return "", fmt.Errorf("store icon: %w", err)
	}
	return stored.ID, nil
}
