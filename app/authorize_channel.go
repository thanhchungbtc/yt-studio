package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// The four use cases behind the authorization dialog, and the one that runs at
// startup to make the channels table agree with what is on disk.
//
// The credentials on disk decide whether an upload is accepted; the channel row
// is a copy of that answer, kept because the upload gate reads it on a path
// where statting files would be wrong. Every function here reads the truth and
// writes the copy, which is the only way the two stay together.

// ChannelAuth reports what a channel can currently publish with, and brings its
// row into line with what it finds.
//
// The reconciling read is deliberate. This is what the dialog calls when it
// opens, so it is also the moment a token placed by hand — or revoked from
// Google's side since the last upload — becomes something the rest of the
// program knows about.
func ChannelAuth(
	ctx context.Context,
	channels repository.ChannelReader,
	writer repository.ChannelWriter,
	authorizer provider.UploadAuthorizer,
	now time.Time,
	key string,
) (entity.Channel, provider.UploadAuth, error) {
	c, err := GetChannel(ctx, channels, key)
	if err != nil {
		return entity.Channel{}, provider.UploadAuth{}, err
	}
	auth, err := authorizer.Auth(ctx, c.Slug)
	if err != nil {
		return entity.Channel{}, provider.UploadAuth{}, err
	}
	c, err = mirror(ctx, writer, c, auth.Status, now)
	if err != nil {
		return entity.Channel{}, provider.UploadAuth{}, err
	}
	return c, auth, nil
}

// ChannelAuthURL is the consent page for a channel.
func ChannelAuthURL(
	ctx context.Context,
	channels repository.ChannelReader,
	authorizer provider.UploadAuthorizer,
	key string,
) (string, error) {
	c, err := GetChannel(ctx, channels, key)
	if err != nil {
		return "", err
	}
	return authorizer.AuthURL(ctx, c.Slug)
}

// AuthorizeChannel exchanges an authorization code for a stored grant.
func AuthorizeChannel(
	ctx context.Context,
	channels repository.ChannelReader,
	writer repository.ChannelWriter,
	authorizer provider.UploadAuthorizer,
	now time.Time,
	key, code string,
) (entity.Channel, provider.UploadAuth, error) {
	c, err := GetChannel(ctx, channels, key)
	if err != nil {
		return entity.Channel{}, provider.UploadAuth{}, err
	}
	if err := authorizer.Exchange(ctx, c.Slug, code); err != nil {
		return entity.Channel{}, provider.UploadAuth{}, err
	}
	// Read back rather than assumed valid: the exchange having succeeded says a
	// token was stored, and this says what it is worth.
	auth, err := authorizer.Auth(ctx, c.Slug)
	if err != nil {
		return entity.Channel{}, provider.UploadAuth{}, err
	}
	c, err = mirror(ctx, writer, c, auth.Status, now)
	if err != nil {
		return entity.Channel{}, provider.UploadAuth{}, err
	}
	return c, auth, nil
}

// ForgetChannelAuth deletes a channel's stored grant.
func ForgetChannelAuth(
	ctx context.Context,
	channels repository.ChannelReader,
	writer repository.ChannelWriter,
	authorizer provider.UploadAuthorizer,
	now time.Time,
	key string,
) (entity.Channel, error) {
	c, err := GetChannel(ctx, channels, key)
	if err != nil {
		return entity.Channel{}, err
	}
	if err := authorizer.Forget(ctx, c.Slug); err != nil {
		return entity.Channel{}, err
	}
	return mirror(ctx, writer, c, entity.CredentialStatusMissing, now)
}

// ReconcileCredentials makes every channel's row say what its credentials
// directory actually holds.
//
// Runs once at startup, which is what picks up a grant that arrived while the
// server was not running — an operator copying a token in from elsewhere, or
// the very first one, placed before this program had ever been told about it.
// A channel whose credentials cannot be read is logged and skipped: one
// unreadable directory is not a reason to refuse to start.
func ReconcileCredentials(
	ctx context.Context,
	channels repository.ChannelReader,
	writer repository.ChannelWriter,
	authorizer provider.UploadAuthorizer,
	now time.Time,
	log *slog.Logger,
) error {
	rows, err := channels.ListChannels(ctx)
	if err != nil {
		return fmt.Errorf("reconcile credentials: %w", err)
	}
	for _, c := range rows {
		auth, err := authorizer.Auth(ctx, c.Slug)
		if err != nil {
			log.Warn("could not read a channel's upload credentials",
				slog.String("channel", string(c.Slug)),
				slog.String("error", err.Error()))
			continue
		}
		if auth.Status == c.Credentials {
			continue
		}
		was := c.Credentials
		if _, err := mirror(ctx, writer, c, auth.Status, now); err != nil {
			log.Warn("could not record a channel's upload credentials",
				slog.String("channel", string(c.Slug)),
				slog.String("error", err.Error()))
			continue
		}
		log.Info("channel upload credentials reconciled",
			slog.String("channel", string(c.Slug)),
			slog.String("was", string(was)),
			slog.String("now", string(auth.Status)))
	}
	return nil
}

// mirror writes the credential status onto the channel row, and only when it
// has actually changed: this runs on every dialog open, and a write per read
// would move updated_at for nothing and put a channel.updated on the wire
// behind it.
func mirror(
	ctx context.Context,
	writer repository.ChannelWriter,
	c entity.Channel,
	status entity.CredentialStatus,
	now time.Time,
) (entity.Channel, error) {
	if !status.Valid() || status == c.Credentials {
		return c, nil
	}
	c.Credentials = status
	c.UpdatedAt = now
	if err := writer.UpdateChannel(ctx, c); err != nil {
		return entity.Channel{}, err
	}
	return c, nil
}
