package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// CreateChannelInput is the boundary shape for a new channel. A blank slug is
// derived from the name; an explicit one is validated as given.
type CreateChannelInput struct {
	Slug        string
	Name        string
	Description string
	Style       entity.StyleConfig
}

// CreateChannel validates and stores a new channel. The slug is chosen here and
// never changes afterwards.
func CreateChannel(
	ctx context.Context,
	channels repository.ChannelWriter,
	newID func() string,
	now time.Time,
	in CreateChannelInput,
) (entity.Channel, error) {
	var (
		slug entity.Slug
		err  error
	)
	if in.Slug == "" {
		slug, err = entity.SlugifyName(in.Name)
	} else {
		slug, err = entity.NewSlug(in.Slug)
	}
	if err != nil {
		return entity.Channel{}, fmt.Errorf("%w: %w", ErrValidation, err)
	}

	c, err := entity.NewChannel(entity.ChannelID(newID()), slug, in.Name, in.Style, now)
	if err != nil {
		return entity.Channel{}, fmt.Errorf("%w: %w", ErrValidation, err)
	}
	c.Description = in.Description

	if err := channels.CreateChannel(ctx, c); err != nil {
		return entity.Channel{}, err
	}
	return c, nil
}
