package app

import (
	"context"
	"strings"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// UpdateVideoMetadata replaces the listing the video publishes with.
//
// The whole listing, not the fields somebody typed. The operator edits a title
// and the caller sends back a title, a description, tags and the three fields it
// was handed — which is what keeps this function free of merge logic, and keeps
// the row from being half the model's answer and half the screen's idea of what
// the model said.
//
// It re-runs nothing and flags nothing stale. The only thing downstream of the
// listing is the upload, which reads this row when it runs, so a corrected title
// is simply what publishes.
func UpdateVideoMetadata(
	ctx context.Context,
	videos repository.VideoReader,
	fields repository.VideoFieldWriter,
	videoID entity.VideoID,
	m entity.Metadata,
) (entity.Video, error) {
	m.Title = strings.TrimSpace(m.Title)
	m.Description = strings.TrimSpace(m.Description)
	m.Tags = cleanTags(m.Tags)
	if err := m.Validate(); err != nil {
		return entity.Video{}, err
	}
	v, err := videos.VideoByID(ctx, videoID)
	if err != nil {
		return entity.Video{}, err
	}
	if err := fields.SetVideoMetadata(ctx, videoID, m); err != nil {
		return entity.Video{}, err
	}
	v.Metadata = &m
	return v, nil
}

// cleanTags trims each tag and drops the empty ones, because a tag field is
// filled in as one comma-separated line and a trailing comma is how people
// finish typing a list.
func cleanTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag = strings.TrimSpace(tag); tag != "" {
			out = append(out, tag)
		}
	}
	return out
}
