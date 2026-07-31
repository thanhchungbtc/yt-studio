package ninerouter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// The upload settings are not the model's to choose. A YouTube category is an
// API enum rather than a creative decision, and a listing arrives unlisted so
// publishing stays the operator's act.
const (
	metadataCategoryID = "24" // Entertainment
	metadataPrivacy    = "private"
)

// metadataDoc is the shape the model returns, and the type the prompt's schema
// is generated from.
//
//nolint:lll // one field, one line: each tag is this field's line in the prompt
type metadataDoc struct {
	Title       string `json:"title" doc:"follows the format '[Topic] | To Fall Asleep Faster'"`
	Description string `json:"description" doc:"one paragraph of 150 to 300 words"`
	Tags        string `json:"tags" doc:"10 to 15 keywords as a single comma-separated string"`
	Thumbnail   string `json:"thumbnail_text" doc:"a 2 to 10 word ALL-CAPS hook, letters and spaces"`
}

// metadataPrompt is what the templates render against.
type metadataPrompt struct {
	provider.MetadataRequest
	ExpectedOutputSchema string
}

// Metadata writes the YouTube-facing listing for a finished video.
//
// The chapter titles go with it rather than just the working title: they are
// what the video actually covers, and they are where the niche keywords and the
// thumbnail hook have to come from.
func (c *Client) Metadata(ctx context.Context, req provider.MetadataRequest) (provider.Metadata, error) {
	schema, err := jsonSchemaOf(metadataDoc{})
	if err != nil {
		return provider.Metadata{}, err
	}
	prompt := metadataPrompt{MetadataRequest: req, ExpectedOutputSchema: schema}

	system, err := render(metadataSystemPrompt, prompt)
	if err != nil {
		return provider.Metadata{}, err
	}
	user, err := render(metadataUserPrompt, prompt)
	if err != nil {
		return provider.Metadata{}, err
	}

	content, err := c.chat(ctx, call{Video: req.VideoID, Label: "metadata"}, system, user)
	if err != nil {
		return provider.Metadata{}, err
	}
	var doc metadataDoc
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return provider.Metadata{}, fmt.Errorf("metadata response is not JSON: %w (%s)",
			err, snippet(content))
	}

	md := entity.Metadata{
		Title:         firstNonEmpty(strings.TrimSpace(doc.Title), req.Title),
		Description:   strings.TrimSpace(doc.Description),
		Tags:          splitTags(doc.Tags),
		ThumbnailText: strings.TrimSpace(doc.Thumbnail),
		CategoryID:    metadataCategoryID,
		Privacy:       metadataPrivacy,
	}
	assetID, err := c.putJSON(ctx, entity.AssetKindMetadata, md)
	if err != nil {
		return provider.Metadata{}, err
	}
	return provider.Metadata{Metadata: md, AssetID: assetID}, nil
}

// splitTags turns the comma-separated keyword string the prompt asks for into
// the list the domain carries. Asking for one string rather than an array is
// the prompt's choice, and models honour it more reliably than a nested list.
func splitTags(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		key := strings.ToLower(p)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, p)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
