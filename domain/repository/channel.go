// Package repository declares the persistence ports of the domain. Every port
// is split into a reader and a writer so a use case can declare exactly the
// half it needs in its signature (§7).
//
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

// ChannelReader reads channels by either key (§3: the API accepts either).
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
	// NextVideoSeq atomically increments and returns the per-channel counter
	// behind video refs (DSS-1, DSS-2...). It is the only way a ref is minted.
	NextVideoSeq(ctx context.Context, id entity.ChannelID) (int, error)
	// UpsertChannelBySlug is the seed path: INSERT ... ON CONFLICT (slug) DO
	// UPDATE, so running the seed a second time updates in place (§3).
	UpsertChannelBySlug(ctx context.Context, c entity.Channel) (entity.Channel, error)
}
