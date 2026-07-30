package ninerouter

import (
	"context"

	"github.com/tbui/yt-studio/domain/provider"
)

// Metadata writes the YouTube-facing listing for a finished video.
//
// Not implemented yet. See the note on Script.
func (c *Client) Metadata(_ context.Context, _ provider.MetadataRequest) (provider.Metadata, error) {
	panic("ninerouter: Metadata is not implemented yet")
}
