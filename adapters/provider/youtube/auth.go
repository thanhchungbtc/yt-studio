package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// tokenFile is the grant as it is kept on disk.
//
// Deliberately the shape the previous generation of this tool wrote, so a token
// carried over from it is read rather than re-earned: the fields Google returns,
// plus the client that earned them. Embedding the client id and secret is what
// makes a refresh possible without reopening credentials.json, and means a
// token that has been moved between channels fails honestly instead of being
// refreshed by whichever client happens to sit beside it.
type tokenFile struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	TokenURI     string `json:"token_uri,omitempty"`
	// Expiry is absolute, unlike the expires_in Google answers with. A duration
	// is only meaningful next to the moment it was issued, which a file does not
	// carry; an absent or unreadable one simply means refresh before using it,
	// which is what a token inherited from another tool gets.
	Expiry string `json:"expiry,omitempty"`
	// RefreshRejected records that Google has refused to renew this grant —
	// revoked from the account's side, or six months unused. It is the
	// difference between "you have not authorized" and "you need to authorize
	// again", and it cannot be discovered without a network call, so the one
	// call that discovers it writes it down.
	RefreshRejected bool `json:"refresh_rejected,omitempty"`
}

// expiry parses the absolute expiry, reporting whether there was a usable one.
func (t tokenFile) expiry() (time.Time, bool) {
	if t.Expiry == "" {
		return time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339, t.Expiry)
	if err != nil {
		return time.Time{}, false
	}
	return at, true
}

// grants reports whether the token's scopes cover what this program does. A
// grant for something else is not a lesser grant, it is the wrong one, and it
// will be refused at the first insert rather than here.
func (t tokenFile) grants() bool {
	for _, s := range strings.Fields(t.Scope) {
		if s == scopeUpload {
			return true
		}
	}
	// An older token may not have recorded its scope at all. Absent is not
	// wrong, and refusing to publish over a missing field would strand a grant
	// that works.
	return strings.TrimSpace(t.Scope) == ""
}

// refreshSkew is how long before expiry the access token is renewed anyway. An
// upload runs for minutes; one that started forty seconds before its token
// lapsed would fail halfway up for no reason worth explaining.
const refreshSkew = 5 * time.Minute

// Auth reports what a channel's stored credentials permit.
//
// Reads the two files and nothing else — no network. It answers a page load,
// and a status that cost a round trip to Google would either be slow on every
// render or cached into the same lie the channels table already tells.
func (c *Client) Auth(_ context.Context, slug entity.Slug) (provider.UploadAuth, error) {
	out := provider.UploadAuth{
		Status:     entity.CredentialStatusMissing,
		ClientPath: c.clientPath(slug),
	}

	if _, err := os.Stat(c.clientPath(slug)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return out, fmt.Errorf("stat %s: %w", c.clientPath(slug), err)
	}
	out.ClientPresent = true

	c.mu.Lock()
	defer c.mu.Unlock()

	token, err := c.readToken(slug)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			// Unreadable is reported as unauthorized rather than as an error: the
			// remedy is the same as for a token that was never there, and failing
			// the request would close the dialog that offers it.
			c.log.Warn("unreadable youtube token",
				slog.String("channel", string(slug)),
				slog.String("path", c.tokenPath(slug)),
				slog.String("error", err.Error()))
		}
		return out, nil
	}

	out.Scope = token.Scope
	if at, ok := token.expiry(); ok {
		out.Expiry = at
	}
	switch {
	case token.RefreshToken == "":
		// A token with no refresh half is spent within the hour and cannot be
		// renewed, so it is worth no more than none at all.
		out.Status = entity.CredentialStatusMissing
	case token.RefreshRejected:
		out.Status = entity.CredentialStatusExpired
	case !token.grants():
		out.Status = entity.CredentialStatusMissing
	default:
		out.Status = entity.CredentialStatusValid
	}
	return out, nil
}

// AuthURL is the consent page an operator must visit to grant access.
//
// The redirect is whatever the client file registers, and nothing of ours
// listens on it. The operator lands on a browser error whose address bar holds
// the code and pastes it back — which sounds worse than it is, and is the only
// flow that works against an unmodified client: a loopback listener needs a
// port, a port makes a redirect the console has never seen, and a web-type
// client matches its redirects exactly.
func (c *Client) AuthURL(_ context.Context, slug entity.Slug) (string, error) {
	client, err := c.loadClient(slug)
	if err != nil {
		return "", err
	}
	query := url.Values{
		"client_id":     {client.ID},
		"redirect_uri":  {client.Redirect},
		"response_type": {"code"},
		"scope":         {scopeUpload},
		// Both are load-bearing. Offline is what asks for a refresh token at
		// all, and consent is what makes Google issue a new one to an account
		// that has already granted this client — without it the second
		// authorization of the same channel returns an access token alone, and
		// the grant silently expires an hour later.
		"access_type": {"offline"},
		"prompt":      {"consent"},
	}
	return authBase + "?" + query.Encode(), nil
}

// Exchange trades an authorization code for a token and stores it.
func (c *Client) Exchange(ctx context.Context, slug entity.Slug, code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return errors.New("youtube: an authorization code is required")
	}
	client, err := c.loadClient(slug)
	if err != nil {
		return err
	}

	form := url.Values{
		"code":          {code},
		"client_id":     {client.ID},
		"client_secret": {client.Secret},
		"redirect_uri":  {client.Redirect},
		"grant_type":    {"authorization_code"},
	}
	granted, err := c.postForm(ctx, client.TokenURI, form)
	if err != nil {
		if isRejectedGrant(err) {
			// Nearly always a code that has already been spent — they are
			// single-use and short-lived, and the second attempt at a paste that
			// worked the first time looks exactly like this.
			return fmt.Errorf(
				"%w: Google refused that authorization code — it may have been used already or "+
					"expired. Open a freshly generated URL and paste the new one",
				provider.ErrRejected)
		}
		return err
	}
	if granted.RefreshToken == "" {
		// Almost always a code that was minted without access_type=offline and
		// prompt=consent, and always worth failing on: the alternative is
		// storing a grant that works for an hour and then looks like a bug.
		return fmt.Errorf(
			"%w: Google returned no refresh token — authorize again from a freshly generated URL",
			ErrUnavailable)
	}

	token := tokenFile{
		AccessToken:  granted.AccessToken,
		RefreshToken: granted.RefreshToken,
		Scope:        granted.Scope,
		TokenType:    granted.TokenType,
		ClientID:     client.ID,
		ClientSecret: client.Secret,
		TokenURI:     client.TokenURI,
		Expiry:       expiryFrom(granted.ExpiresIn).Format(time.RFC3339),
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.writeToken(slug, token); err != nil {
		return err
	}
	c.log.Info("youtube channel authorized",
		slog.String("channel", string(slug)),
		slog.String("scope", token.Scope))
	return nil
}

// Forget deletes the stored token, leaving the client where it is.
func (c *Client) Forget(_ context.Context, slug entity.Slug) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := os.Remove(c.tokenPath(slug)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", c.tokenPath(slug), err)
	}
	c.log.Info("youtube grant forgotten", slog.String("channel", string(slug)))
	return nil
}

// bearer returns an access token good for the next few minutes, refreshing the
// stored grant when it is not.
//
// The whole read-refresh-write runs under the mutex, so two callers cannot both
// decide to refresh and have the loser write a token the winner has already
// replaced.
func (c *Client) bearer(ctx context.Context, slug entity.Slug) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	token, err := c.readToken(slug)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: channel %s is not authorized for YouTube", ErrUnavailable, slug)
		}
		return "", err
	}
	if token.RefreshToken == "" {
		return "", fmt.Errorf("%w: channel %s has no refreshable YouTube grant", ErrUnavailable, slug)
	}
	if token.RefreshRejected {
		return "", fmt.Errorf("%w: channel %s needs to be authorized again", ErrUnavailable, slug)
	}
	if at, ok := token.expiry(); ok && time.Until(at) > refreshSkew && token.AccessToken != "" {
		return token.AccessToken, nil
	}

	// An inherited token records no absolute expiry, so the first upload after
	// an import always refreshes. That is one extra round trip, once, and it
	// leaves the file in the shape the rest of this file expects.
	tokenURI := token.TokenURI
	if tokenURI == "" {
		tokenURI = defaultTokenURI
	}
	form := url.Values{
		"client_id":     {token.ClientID},
		"client_secret": {token.ClientSecret},
		"refresh_token": {token.RefreshToken},
		"grant_type":    {"refresh_token"},
	}
	refreshed, err := c.postForm(ctx, tokenURI, form)
	if err != nil {
		if isRejectedGrant(err) {
			token.RefreshRejected = true
			if writeErr := c.writeToken(slug, token); writeErr != nil {
				c.log.Warn("could not record the rejected youtube grant",
					slog.String("channel", string(slug)),
					slog.String("error", writeErr.Error()))
			}
			return "", fmt.Errorf(
				"%w: channel %s needs to be authorized again — Google refused to renew the grant",
				ErrUnavailable, slug)
		}
		return "", err
	}

	token.AccessToken = refreshed.AccessToken
	token.Expiry = expiryFrom(refreshed.ExpiresIn).Format(time.RFC3339)
	// A refresh answers with the scope it renewed; an empty one means unchanged.
	if refreshed.Scope != "" {
		token.Scope = refreshed.Scope
	}
	if err := c.writeToken(slug, token); err != nil {
		return "", err
	}
	return token.AccessToken, nil
}

// grantResponse is what both token calls answer with.
type grantResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

// expiryFrom turns Google's relative lifetime into a moment. A response with no
// lifetime is treated as already lapsed, so it is refreshed rather than trusted.
func expiryFrom(seconds int64) time.Time {
	if seconds <= 0 {
		return time.Now().UTC()
	}
	return time.Now().UTC().Add(time.Duration(seconds) * time.Second)
}

// errRejectedGrant marks the one token failure a retry cannot fix.
var errRejectedGrant = errors.New("the grant was rejected")

func isRejectedGrant(err error) bool { return errors.Is(err, errRejectedGrant) }

// postForm runs one form-encoded call against Google's token endpoint.
func (c *Client) postForm(ctx context.Context, endpoint string, form url.Values) (grantResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, metaTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return grantResponse{}, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return grantResponse{}, fmt.Errorf("reach %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return grantResponse{}, fmt.Errorf("read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// The OAuth endpoint answers with its own error shape rather than the
		// API's, and invalid_grant is the one that means re-authorize.
		var oauthErr struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		_ = json.Unmarshal(body, &oauthErr)
		if oauthErr.Error == "invalid_grant" {
			return grantResponse{}, fmt.Errorf("%w: %s", errRejectedGrant, oauthErr.Description)
		}
		if oauthErr.Error != "" {
			return grantResponse{}, fmt.Errorf("youtube token exchange: %s: %s",
				oauthErr.Error, oauthErr.Description)
		}
		return grantResponse{}, apiError("youtube token exchange", resp.StatusCode, body)
	}

	var out grantResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return grantResponse{}, fmt.Errorf("decode token response: %w", err)
	}
	if out.AccessToken == "" {
		return grantResponse{}, errors.New("youtube: the token response carried no access token")
	}
	return out, nil
}

// readToken reads one channel's grant. Callers hold the mutex.
func (c *Client) readToken(slug entity.Slug) (tokenFile, error) {
	path := c.tokenPath(slug)
	raw, err := os.ReadFile(path) //nolint:gosec // a path this program composed from a validated slug
	if err != nil {
		return tokenFile{}, err
	}
	var out tokenFile
	if err := json.Unmarshal(raw, &out); err != nil {
		return tokenFile{}, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	return out, nil
}

// writeToken replaces one channel's grant. Callers hold the mutex.
//
// Written to a neighbouring file and renamed over the original, at 0600. A
// truncated token file is not a corrupt cache to be rebuilt — it is a grant
// that costs a trip through Google's consent screen to replace — and a refresh
// that was interrupted mid-write is exactly the moment it would happen.
func (c *Client) writeToken(slug entity.Slug, token tokenFile) error {
	if err := os.MkdirAll(c.dir(slug), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", c.dir(slug), err)
	}
	body, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("encode token: %w", err)
	}
	path := c.tokenPath(slug)
	temp := path + ".tmp"
	if err := os.WriteFile(temp, body, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", temp, err)
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
