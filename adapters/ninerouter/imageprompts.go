package ninerouter

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// ImagePrompts returns every chapter's still prompts for one video.
//
// Not implemented yet, and the one method that will need state on the client:
// the port's contract is that N per-chapter callers collapse onto a single
// generation, which means a singleflight group and a cache. That is why the
// methods here take a pointer receiver.
func (c *Client) ImagePrompts(_ context.Context, _ entity.VideoID) ([]provider.ImagePrompt, error) {
	panic("ninerouter: ImagePrompts is not implemented yet")
}
