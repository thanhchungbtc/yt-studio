package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// IconOptions are the settings-sourced inputs of one icon: the clause every
// cell shares, and the size they are all drawn at.
type IconOptions struct {
	Style string
	Size  int
}

// GenerateThumbnailIcon draws one cell's icon. The task's index is the cell: it
// reads that prompt from the plan and writes back into the slot the plan sized,
// so icons finishing in any order still land in their own cells.
//
//nolint:revive // the parameter list is the dependency list
func GenerateThumbnailIcon(
	ctx context.Context,
	t entity.Task,
	videos repository.VideoReader,
	icons provider.IconGenerator,
	videoFields repository.VideoFieldWriter,
	assets repository.AssetWriter,
	store provider.AssetStore,
	opts IconOptions,
	now time.Time,
) entity.TaskOutcome {
	video, err := videos.VideoByID(ctx, t.VideoID)
	if err != nil {
		return classify(err)
	}
	if video.ThumbnailPlan == nil {
		return entity.Failed{Err: fmt.Errorf("%w: video has no thumbnail plan", ErrValidation), Retryable: true}
	}
	cells := video.ThumbnailPlan.Cells
	if t.Index < 0 || t.Index >= len(cells) {
		// The plan is narrower than the graph, so it was replaced by a shorter
		// one. Re-running the plan task fixes it; another attempt here does not.
		return entity.Failed{
			Err: fmt.Errorf("%w: cell %d of a %d-cell plan", ErrValidation, t.Index, len(cells)),
		}
	}

	assetID, err := icons.Generate(ctx, provider.IconRequest{
		VideoID: video.ID,
		Index:   t.Index,
		Prompt:  joinStyle(cells[t.Index].Prompt, opts.Style),
		Size:    opts.Size,
	})
	if err != nil {
		return classify(fmt.Errorf("generate icon %d for %s: %w", t.Index, video.Ref, err))
	}

	if _, err := RecordAsset(ctx, assets, store, assetID, entity.AssetKindThumbnailIcon,
		video.ID, nil, "thumbnail.icon", now); err != nil {
		return classify(err)
	}
	if err := videoFields.SetVideoThumbnailIcon(ctx, video.ID, t.Index, assetID); err != nil {
		return classify(err)
	}
	return entity.Success{Assets: []entity.AssetID{assetID}}
}

// joinStyle appends the shared style clause to a cell's subject, so the content
// address covers the style and an edit produces new icons rather than old ones.
func joinStyle(subject, style string) string {
	style = strings.TrimSpace(style)
	if style == "" {
		return subject
	}
	return subject + " — " + style
}
