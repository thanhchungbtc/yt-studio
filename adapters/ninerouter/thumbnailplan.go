package ninerouter

import (
	"context"
	"fmt"

	"github.com/tbui/yt-studio/domain/provider"
)

// ThumbnailPlan is not written yet.
//
// Its prompt is a design decision rather than a translation: which ideas of a
// fifty-chapter video earn a tile, and how an icon subject is described so ten
// of them come back looking like one set. Guessing at that and shipping it
// would mean tuning against a prompt nobody chose.
//
// ErrUnavailable is the honest answer until then — the operator learns the
// backend cannot do this from a parked task naming the reason, rather than from
// a thumbnail that quietly came out wrong. The mock backend serves the grid in
// the meantime.
func (c *Client) ThumbnailPlan(_ context.Context, _ provider.ThumbnailPlanRequest) (provider.ThumbnailPlan, error) {
	return provider.ThumbnailPlan{}, fmt.Errorf(
		"%w: the 9router backend has no thumbnail plan prompt yet", ErrUnavailable)
}
