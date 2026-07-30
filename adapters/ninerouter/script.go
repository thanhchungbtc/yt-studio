package ninerouter

import (
	"context"

	"github.com/tbui/yt-studio/domain/provider"
)

// Script writes one chapter's narration.
//
// Not implemented yet. This package is wired to nothing, so the panic is
// unreachable rather than a hazard; it becomes a prompt and a parse in the same
// shape as Blueprint.
func (c *Client) Script(_ context.Context, _ provider.ScriptRequest) (provider.Script, error) {
	panic("ninerouter: Script is not implemented yet")
}
