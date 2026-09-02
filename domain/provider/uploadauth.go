package provider

import (
	"context"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
)

// UploadAuth is what a channel's stored credentials currently permit.
//
// Read from wherever the credentials actually live rather than from the
// channels table. The row is a mirror kept for the gate to read cheaply; this
// is the thing that decides whether a request will be accepted, and the two can
// disagree — an operator who drops a token in by hand, or a grant revoked from
// Google's side, moves one without moving the other.
type UploadAuth struct {
	// ClientPresent reports whether an OAuth client has been placed for this
	// channel. Without one there is nothing to authorize against, which is a
	// different problem from not having authorized yet and is answered
	// differently: put a file here, versus visit this URL.
	ClientPresent bool
	// Status is what the stored token permits. Missing means there is none,
	// expired means there is one the provider will no longer refresh.
	Status entity.CredentialStatus
	// Scope is what the stored token was granted, empty when there is none. A
	// token minted for the wrong scopes authorizes nothing useful and is worth
	// showing rather than discovering at the first upload.
	Scope string
	// Expiry is when the access token lapses, zero when unknown. Not a reason to
	// re-authorize: the refresh token outlives it and is renewed in passing.
	Expiry time.Time
	// ClientPath is where a client for this channel goes, for the one screen
	// that has to tell an operator where to put a file. Display text and
	// nothing else — nothing reads it back — and empty when the backend has no
	// such place.
	ClientPath string
}

// Authorized reports whether an upload would be accepted right now.
func (a UploadAuth) Authorized() bool {
	return a.ClientPresent && a.Status == entity.CredentialStatusValid
}

// UploadAuthorizer manages the credentials one channel publishes with.
//
// A port of its own rather than four more methods on Uploader: publishing is
// what the pipeline does, and this is what an operator does beforehand. The
// scheduler never calls anything here, and the four handlers that do never
// publish.
type UploadAuthorizer interface {
	// Auth reports what the channel's stored credentials permit.
	Auth(ctx context.Context, slug entity.Slug) (UploadAuth, error)
	// AuthURL is the consent page the operator must visit to grant access.
	AuthURL(ctx context.Context, slug entity.Slug) (string, error)
	// Exchange trades an authorization code for a token and stores it. It fails
	// rather than storing a token that cannot be refreshed.
	Exchange(ctx context.Context, slug entity.Slug, code string) error
	// Forget deletes the stored token, leaving the client in place: the next
	// upload needs a new grant, and getting one needs no new file.
	Forget(ctx context.Context, slug entity.Slug) error
}
