// Package repository declares the persistence ports of the domain, each split
// into a reader and a writer so a use case can name exactly the half it needs.
// Nothing here imports anything outside domain/entity.
package repository

import (
	"context"
	"errors"

	"github.com/tbui/yt-studio/domain/entity"
)

// ErrNotFound is returned by every reader when the row does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned by a writer when a natural key is already taken.
var ErrConflict = errors.New("conflict")

// ChannelReader reads channels by either key.
type ChannelReader interface {
	ChannelByID(ctx context.Context, id entity.ChannelID) (entity.Channel, error)
	ChannelBySlug(ctx context.Context, slug entity.Slug) (entity.Channel, error)
	ListChannels(ctx context.Context) ([]entity.Channel, error)
}

// ChannelWriter creates and updates channels.
type ChannelWriter interface {
	CreateChannel(ctx context.Context, c entity.Channel) error
	UpdateChannel(ctx context.Context, c entity.Channel) error
	DeleteChannel(ctx context.Context, id entity.ChannelID) error
	// NextVideoSeq atomically increments the per-channel counter behind refs. It
	// is the only way a ref is minted.
	NextVideoSeq(ctx context.Context, id entity.ChannelID) (int, error)
	// UpsertChannelBySlug is the seed path, so a second seed updates in place.
	UpsertChannelBySlug(ctx context.Context, c entity.Channel) (entity.Channel, error)
}
