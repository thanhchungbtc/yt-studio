package services

import (
	"context"
	"time"

	"github.com/tbui/yt-studio/internal/app"
	"github.com/tbui/yt-studio/internal/domain/provider"
)

// ChannelService lists channels and manages each one's YouTube grant.
type ChannelService struct{ b *Backend }

// NewChannelService creates a ChannelService.
func NewChannelService(b *Backend) *ChannelService { return &ChannelService{b: b} }

// ChannelAuthDTO is what a channel can currently publish with.
type ChannelAuthDTO struct {
	Channel ChannelDTO `json:"channel"`
	// ClientPresent: an OAuth client is at ClientPath.
	ClientPresent bool       `json:"clientPresent"`
	Status        string     `json:"status"`
	Authorized    bool       `json:"authorized"`
	Scope         string     `json:"scope,omitempty"`
	Expiry        *time.Time `json:"expiry,omitempty"`
	ClientPath    string     `json:"clientPath,omitempty"`
}

func channelAuthFrom(c ChannelDTO, auth provider.UploadAuth) ChannelAuthDTO {
	out := ChannelAuthDTO{
		Channel:       c,
		ClientPresent: auth.ClientPresent,
		Status:        string(auth.Status),
		Authorized:    auth.Authorized(),
		Scope:         auth.Scope,
		ClientPath:    auth.ClientPath,
	}
	if !auth.Expiry.IsZero() {
		at := auth.Expiry
		out.Expiry = &at
	}
	return out
}

// List returns every channel.
func (s *ChannelService) List(ctx context.Context) ([]ChannelDTO, error) {
	rows, err := app.ListChannels(ctx, s.b.Store)
	if err != nil {
		return nil, err
	}
	out := make([]ChannelDTO, 0, len(rows))
	for _, c := range rows {
		out = append(out, channelFrom(c))
	}
	return out, nil
}

// Auth reports what a channel can publish with, reconciling its row.
func (s *ChannelService) Auth(ctx context.Context, channel string) (ChannelAuthDTO, error) {
	c, auth, err := app.ChannelAuth(ctx, s.b.Store, s.b.Store, s.b.UploadAuth, s.b.Now(), channel)
	if err != nil {
		return ChannelAuthDTO{}, err
	}
	return channelAuthFrom(channelFrom(c), auth), nil
}

// AuthURL is the consent page's address.
func (s *ChannelService) AuthURL(ctx context.Context, channel string) (string, error) {
	return app.ChannelAuthURL(ctx, s.b.Store, s.b.UploadAuth, channel)
}

// Authorize exchanges a pasted code (or redirect URL) for a stored grant.
func (s *ChannelService) Authorize(ctx context.Context, channel, code string) (ChannelAuthDTO, error) {
	if code == "" || len(code) > 2048 {
		return ChannelAuthDTO{}, app.Invalid("code", "must be 1 to 2048 characters")
	}
	c, auth, err := app.AuthorizeChannel(ctx, s.b.Store, s.b.Store, s.b.UploadAuth, s.b.Now(), channel, code)
	if err != nil {
		return ChannelAuthDTO{}, err
	}
	return channelAuthFrom(channelFrom(c), auth), nil
}

// ForgetAuth drops a channel's grant; its OAuth client stays.
func (s *ChannelService) ForgetAuth(ctx context.Context, channel string) (ChannelAuthDTO, error) {
	c, err := app.ForgetChannelAuth(ctx, s.b.Store, s.b.Store, s.b.UploadAuth, s.b.Now(), channel)
	if err != nil {
		return ChannelAuthDTO{}, err
	}
	auth, err := s.b.UploadAuth.Auth(ctx, c.Slug)
	if err != nil {
		return ChannelAuthDTO{}, err
	}
	return channelAuthFrom(channelFrom(c), auth), nil
}
