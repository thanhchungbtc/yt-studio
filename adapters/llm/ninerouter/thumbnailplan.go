package ninerouter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// thumbnailPlanDoc is the shape the model returns, and the type the prompt's
// schema is generated from.
type thumbnailPlanDoc struct {
	Cells []thumbnailCellDoc `json:"cells" doc:"one entry per tile, in reading order"`
}

//nolint:lll // one field, one line: each tag is this field's line in the prompt
type thumbnailCellDoc struct {
	Caption string `json:"caption" doc:"one Title Case word, at most 13 characters, letters digits and spaces only"`
	Prompt  string `json:"icon" doc:"2 to 4 sentences describing a single iconic symbol in literal physical terms, ending with the exact style tag"`
}

// thumbnailPlanPrompt is what the templates render against.
type thumbnailPlanPrompt struct {
	provider.ThumbnailPlanRequest
	ExpectedOutputSchema string
}

// ThumbnailPlan writes the grid of captions and icon subjects that sits under
// the thumbnail's headline.
//
// The cell count is a contract rather than a target: the graph already holds
// one icon task per cell. A plan that comes back short is rejected above, in
// app.normaliseCells, which is also where a long one is cut.
func (c *Client) ThumbnailPlan(ctx context.Context, req provider.ThumbnailPlanRequest) (provider.ThumbnailPlan, error) {
	schema, err := jsonSchemaOf(thumbnailPlanDoc{})
	if err != nil {
		return provider.ThumbnailPlan{}, err
	}
	prompt := thumbnailPlanPrompt{ThumbnailPlanRequest: req, ExpectedOutputSchema: schema}

	system, err := render(thumbnailPlanSystemPrompt, prompt)
	if err != nil {
		return provider.ThumbnailPlan{}, err
	}
	user, err := render(thumbnailPlanUserPrompt, prompt)
	if err != nil {
		return provider.ThumbnailPlan{}, err
	}

	content, err := c.chat(ctx, call{Video: req.VideoID, Label: "thumbnail_plan"}, system, user)
	if err != nil {
		return provider.ThumbnailPlan{}, err
	}
	var doc thumbnailPlanDoc
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return provider.ThumbnailPlan{}, fmt.Errorf("thumbnail plan response is not JSON: %w (%s)",
			err, snippet(content))
	}

	plan := entity.ThumbnailPlan{Cells: make([]entity.ThumbnailCell, 0, len(doc.Cells))}
	for _, cell := range doc.Cells {
		plan.Cells = append(plan.Cells, entity.ThumbnailCell{
			Caption: strings.Join(strings.Fields(cell.Caption), " "),
			Prompt:  strings.TrimSpace(cell.Prompt),
		})
	}
	assetID, err := c.putJSON(ctx, entity.AssetKindThumbnailPlan, plan)
	if err != nil {
		return provider.ThumbnailPlan{}, err
	}
	return provider.ThumbnailPlan{Plan: plan, AssetID: assetID}, nil
}
