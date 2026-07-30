package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// UpdateChannelInput carries the mutable fields of a channel. The slug is
// absent on purpose: it is immutable.
type UpdateChannelInput struct {
	Name        string
	Description string
	Style       entity.StyleConfig
	Credentials entity.CredentialStatus
}

// UpdateChannel writes back a channel's editable fields.
func UpdateChannel(
	ctx context.Context,
	channels repository.ChannelReader,
	writer repository.ChannelWriter,
	now time.Time,
	key string,
	in UpdateChannelInput,
) (entity.Channel, error) {
	c, err := GetChannel(ctx, channels, key)
	if err != nil {
		return entity.Channel{}, err
	}
	if name := strings.TrimSpace(in.Name); name != "" {
		c.Name = name
	}
	c.Description = in.Description
	if in.Style.Tone != "" {
		c.Style.Tone = in.Style.Tone
	}
	if in.Style.Voice != "" {
		c.Style.Voice = in.Style.Voice
	}
	if in.Style.ImageStyle != "" {
		c.Style.ImageStyle = in.Style.ImageStyle
	}
	if in.Style.Language != "" {
		c.Style.Language = in.Style.Language
	}
	if in.Style.WordsPerChapter > 0 {
		c.Style.WordsPerChapter = in.Style.WordsPerChapter
	}
	if in.Credentials != "" {
		if !in.Credentials.Valid() {
			return entity.Channel{}, Invalid("credentials", fmt.Sprintf("must be one of %v", entity.AllCredentialStatuses))
		}
		c.Credentials = in.Credentials
	}
	c.UpdatedAt = now

	if err := writer.UpdateChannel(ctx, c); err != nil {
		return entity.Channel{}, err
	}
	return c, nil
}
