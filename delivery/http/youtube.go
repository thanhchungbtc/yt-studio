package http

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
)

// The authorization half of publishing: four operations an operator drives from
// the dialog, none of which the scheduler ever calls.
//
// Scoped under a channel because a grant is a channel's, not the
// installation's. Two channels are two YouTube accounts, and there is no
// meaningful global answer to "is this authorized".

// ChannelAuthDTO is what a channel can currently publish with.
type ChannelAuthDTO struct {
	Channel ChannelDTO `json:"channel"`
	// ClientPresent reports whether an OAuth client has been placed. False is a
	// different problem from an unauthorized channel and has a different remedy:
	// put a file at clientPath, rather than visit a URL.
	ClientPresent bool   `json:"clientPresent"`
	Status        string `json:"status" enum:"missing,valid,expired"`
	// Authorized is the one question callers actually ask, answered here rather
	// than left to be recomputed from the two fields above.
	Authorized bool `json:"authorized"`
	// Scope is what the stored grant covers, so a token minted for the wrong
	// scopes can be shown as such instead of failing at the first upload.
	Scope string `json:"scope,omitempty"`
	// Expiry is when the access token lapses, absent when unknown. Not a
	// deadline for the operator: the grant behind it renews itself.
	Expiry *time.Time `json:"expiry,omitempty"`
	// ClientPath is where to put credentials.json, for the screen that has to
	// say so. Display text; nothing reads it back.
	ClientPath string `json:"clientPath,omitempty"`
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

// ChannelAuthOutput is the authorization status response.
type ChannelAuthOutput struct {
	Body ChannelAuthDTO
}

// AuthURLOutput carries the consent page's address.
type AuthURLOutput struct {
	Body struct {
		URL string `json:"url"`
	}
}

// AuthorizeChannelInput is the code the operator pasted back.
type AuthorizeChannelInput struct {
	Key  string `path:"key" doc:"Channel slug or id"`
	Body struct {
		// Accepts the whole redirect URL as well as a bare code: the client
		// extracts the parameter, and this is the backstop for anything it did
		// not recognise.
		Code string `json:"code" required:"true" minLength:"1" maxLength:"2048" doc:"The authorization code, or the redirect URL containing it"`
	}
}

// getChannelAuth reports what a channel can publish with, reconciling its row
// against what is actually stored on the way.
func getChannelAuth(
	channels repository.ChannelReader,
	writer repository.ChannelWriter,
	authorizer provider.UploadAuthorizer,
	now func() time.Time,
) func(context.Context, *ChannelKeyInput) (*ChannelAuthOutput, error) {
	return func(ctx context.Context, in *ChannelKeyInput) (*ChannelAuthOutput, error) {
		c, auth, err := app.ChannelAuth(ctx, channels, writer, authorizer, now(), in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		return &ChannelAuthOutput{Body: channelAuthFrom(channelFrom(c), auth)}, nil
	}
}

// getChannelAuthURL generates the consent page's address.
//
// Generated per request rather than once, because it is only useful while the
// operator is looking at it: a URL held in a client cache from an hour ago
// still works, but nothing about that is worth relying on.
func getChannelAuthURL(
	channels repository.ChannelReader,
	authorizer provider.UploadAuthorizer,
) func(context.Context, *ChannelKeyInput) (*AuthURLOutput, error) {
	return func(ctx context.Context, in *ChannelKeyInput) (*AuthURLOutput, error) {
		url, err := app.ChannelAuthURL(ctx, channels, authorizer, in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		out := &AuthURLOutput{}
		out.Body.URL = url
		return out, nil
	}
}

// postChannelAuthorize exchanges the pasted code for a stored grant.
func postChannelAuthorize(
	channels repository.ChannelReader,
	writer repository.ChannelWriter,
	authorizer provider.UploadAuthorizer,
	now func() time.Time,
) func(context.Context, *AuthorizeChannelInput) (*ChannelAuthOutput, error) {
	return func(ctx context.Context, in *AuthorizeChannelInput) (*ChannelAuthOutput, error) {
		c, auth, err := app.AuthorizeChannel(ctx, channels, writer, authorizer, now(),
			in.Key, in.Body.Code)
		if err != nil {
			return nil, mapError(err)
		}
		return &ChannelAuthOutput{Body: channelAuthFrom(channelFrom(c), auth)}, nil
	}
}

// deleteChannelAuth forgets a channel's grant, leaving its client in place.
func deleteChannelAuth(
	channels repository.ChannelReader,
	writer repository.ChannelWriter,
	authorizer provider.UploadAuthorizer,
	now func() time.Time,
) func(context.Context, *ChannelKeyInput) (*ChannelAuthOutput, error) {
	return func(ctx context.Context, in *ChannelKeyInput) (*ChannelAuthOutput, error) {
		c, err := app.ForgetChannelAuth(ctx, channels, writer, authorizer, now(), in.Key)
		if err != nil {
			return nil, mapError(err)
		}
		auth, err := authorizer.Auth(ctx, c.Slug)
		if err != nil {
			return nil, mapError(err)
		}
		return &ChannelAuthOutput{Body: channelAuthFrom(channelFrom(c), auth)}, nil
	}
}

func registerYouTubeRoutes(
	api huma.API,
	channels repository.ChannelReader,
	writer repository.ChannelWriter,
	authorizer provider.UploadAuthorizer,
	now func() time.Time,
) {
	huma.Register(api, huma.Operation{
		OperationID: "getChannelAuth", Method: "GET", Path: "/api/channels/{key}/youtube",
		Summary: "Report a channel's YouTube authorization", Tags: []string{"youtube"},
		Description: "Reads the channel's stored credentials and brings its credentials " +
			"field into line with them. No network call.",
	}, getChannelAuth(channels, writer, authorizer, now))

	huma.Register(api, huma.Operation{
		OperationID: "getChannelAuthURL", Method: "GET", Path: "/api/channels/{key}/youtube/auth-url",
		Summary: "Generate a YouTube consent URL", Tags: []string{"youtube"},
		Description: "Fails with 409 when the channel has no OAuth client on disk.",
	}, getChannelAuthURL(channels, authorizer))

	huma.Register(api, huma.Operation{
		OperationID: "authorizeChannel", Method: "POST", Path: "/api/channels/{key}/youtube/authorize",
		Summary: "Exchange an authorization code for a grant", Tags: []string{"youtube"},
		Description: "Accepts the whole redirect URL or a bare code. Fails when Google " +
			"returns no refresh token, rather than storing a grant that lapses in an hour.",
	}, postChannelAuthorize(channels, writer, authorizer, now))

	huma.Register(api, huma.Operation{
		OperationID: "forgetChannelAuth", Method: "DELETE", Path: "/api/channels/{key}/youtube",
		Summary: "Forget a channel's YouTube grant", Tags: []string{"youtube"},
		Description: "Deletes the stored token. The OAuth client stays, so authorizing " +
			"again needs no new file.",
	}, deleteChannelAuth(channels, writer, authorizer, now))
}
