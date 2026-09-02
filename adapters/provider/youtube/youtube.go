// Package youtube publishes a finished render to YouTube, and manages the
// per-channel OAuth grant that lets it.
//
// Hand-rolled against the REST API over net/http, the same way every other
// backend in this tree talks to its service. Google ships a Go client for this,
// and it would replace perhaps eighty lines here with a module graph several
// times the size of the whole program's — for a resumable upload, a token
// refresh and two multipart-free POSTs, all of which are a page of the protocol
// each.
//
// One directory per channel, because a grant belongs to a channel and not to
// the installation: two channels are two YouTube accounts, and an operator who
// authorizes one has said nothing about the other.
//
//	<credentials>/<slug>/credentials.json   the OAuth client, placed by hand
//	<credentials>/<slug>/token.json         the grant, written by Exchange
package youtube

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// ErrUnavailable reports a channel that cannot publish until someone does
// something about it — no client on disk, no grant, a grant Google has stopped
// honouring. Wrapping the port's sentinel makes app.classify fail it once
// rather than retrying into the same wall.
var ErrUnavailable = fmt.Errorf("youtube: %w", provider.ErrUnavailable)

// Google's endpoints. Constants rather than settings rows: these are not a
// deployment's choice, and a wrong one is not a thing an operator can debug.
const (
	authBase        = "https://accounts.google.com/o/oauth2/auth"
	defaultTokenURI = "https://oauth2.googleapis.com/token"
	videosEndpoint  = "https://www.googleapis.com/upload/youtube/v3/videos"
	thumbsEndpoint  = "https://www.googleapis.com/upload/youtube/v3/thumbnails/set"
	watchPrefix     = "https://www.youtube.com/watch?v="
)

// scopeUpload is the only scope asked for. videos.insert and thumbnails.set are
// all this program does, and both are covered by it; asking for youtube.force-ssl
// as well would buy the ability to edit and delete somebody's back catalogue.
const scopeUpload = "https://www.googleapis.com/auth/youtube.upload"

// defaultRedirect is used when the client file registers none. Nothing listens
// there: the operator lands on a browser error whose address bar holds the
// code, and pastes it back. See AuthURL.
const defaultRedirect = "http://localhost"

// chunkSize is how much of the render goes up per request. Google requires a
// multiple of 256 KiB for every chunk but the last, and recommends the largest
// the caller can afford to re-send — which is what a chunk costs when one
// fails. Eight megabytes is roughly a second of uplink on a decent line and
// gives a progress bar something to say a few times a minute on a bad one.
const chunkSize = 8 << 20

// Request timings. The session is opened and the render is sent with the same
// client but on very different terms: one is a small JSON round trip, the other
// is eight megabytes uphill, and a timeout sized for the second would let the
// first hang for minutes.
const (
	metaTimeout  = 30 * time.Second
	chunkTimeout = 10 * time.Minute
)

// chunkAttempts is how many times one chunk is re-sent before the task fails.
// A resumable session survives a dropped connection — that is the whole point
// of it — so the retry is cheap and the failure it hides is the common one.
const chunkAttempts = 4

// Client is the backend. One per installation, holding the credentials root
// rather than any one channel's grant: which channel is publishing arrives on
// the request.
type Client struct {
	root  string
	store provider.AssetStore
	http  *http.Client
	log   *slog.Logger

	// mu serialises the read-modify-write of a token file. A refresh rewrites
	// the access token in place, and the four handlers can read one while the
	// scheduler is publishing with it. One mutex for every channel rather than
	// one each: uploads are already serialised by the upload pool, so the
	// contention this could save does not exist.
	mu sync.Mutex
}

var (
	_ provider.Uploader         = (*Client)(nil)
	_ provider.UploadAuthorizer = (*Client)(nil)
)

// New constructs the backend over the credentials root.
func New(root string, store provider.AssetStore, log *slog.Logger) *Client {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Client{
		root:  root,
		store: store,
		log:   log,
		// No client-level timeout: the two calls want wildly different ones and
		// set them per request through a context.
		http: &http.Client{},
	}
}

// Root is where the per-channel directories live, for a startup log line and
// for the error that tells an operator where to put a file.
func (c *Client) Root() string { return c.root }

// dir is one channel's credentials directory.
func (c *Client) dir(slug entity.Slug) string { return filepath.Join(c.root, string(slug)) }

// clientPath is where the operator puts what they downloaded from Google.
func (c *Client) clientPath(slug entity.Slug) string {
	return filepath.Join(c.dir(slug), "credentials.json")
}

// tokenPath is where a granted token is kept.
func (c *Client) tokenPath(slug entity.Slug) string {
	return filepath.Join(c.dir(slug), "token.json")
}

// oauthClient is the half of the downloaded file that matters.
type oauthClient struct {
	ID       string
	Secret   string
	TokenURI string
	Redirect string
}

// clientFile is the shape Google hands out. The interesting fields sit under
// one of two keys depending on which application type was chosen in the
// console, and neither is a thing this program should make an operator care
// about, so both are read.
type clientFile struct {
	Web       *clientBlock `json:"web"`
	Installed *clientBlock `json:"installed"`
}

type clientBlock struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	TokenURI     string   `json:"token_uri"`
	RedirectURIs []string `json:"redirect_uris"`
}

// loadClient reads and validates one channel's OAuth client.
func (c *Client) loadClient(slug entity.Slug) (oauthClient, error) {
	path := c.clientPath(slug)
	raw, err := os.ReadFile(path) //nolint:gosec // a path this program composed from a validated slug
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return oauthClient{}, fmt.Errorf(
				"%w: channel %s has no OAuth client — put the JSON from the Google Cloud console at %s",
				ErrUnavailable, slug, path)
		}
		return oauthClient{}, fmt.Errorf("read %s: %w", path, err)
	}
	var file clientFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return oauthClient{}, fmt.Errorf("%w: %s is not valid JSON: %w", ErrUnavailable, path, err)
	}
	block := file.Web
	if block == nil {
		block = file.Installed
	}
	if block == nil || block.ClientID == "" || block.ClientSecret == "" {
		return oauthClient{}, fmt.Errorf(
			"%w: %s has no client_id and client_secret under \"web\" or \"installed\"",
			ErrUnavailable, path)
	}

	out := oauthClient{
		ID:       block.ClientID,
		Secret:   block.ClientSecret,
		TokenURI: strings.TrimSpace(block.TokenURI),
		Redirect: defaultRedirect,
	}
	if out.TokenURI == "" {
		out.TokenURI = defaultTokenURI
	}
	// Whatever the console has registered, rather than a constant: a web-type
	// client matches its redirect exactly, so guessing one Google has never
	// heard of turns the consent page into an error page.
	if len(block.RedirectURIs) > 0 && strings.TrimSpace(block.RedirectURIs[0]) != "" {
		out.Redirect = strings.TrimSpace(block.RedirectURIs[0])
	}
	return out, nil
}

// apiError turns a failed Google response into something worth reading. Their
// errors carry a message that names the actual problem — a quota, a rejected
// thumbnail, a video too long for an unverified account — and a bare status
// code would throw all of it away.
func apiError(what string, code int, body []byte) error {
	var parsed struct {
		Error struct {
			Message string `json:"message"`
			Errors  []struct {
				Reason  string `json:"reason"`
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"error"`
	}
	message := ""
	if json.Unmarshal(body, &parsed) == nil {
		message = parsed.Error.Message
		if len(parsed.Error.Errors) > 0 && parsed.Error.Errors[0].Reason != "" {
			message = fmt.Sprintf("%s (%s)", message, parsed.Error.Errors[0].Reason)
		}
	}
	if message == "" {
		message = snippet(body)
	}
	// Quota is the one failure worth naming, because it is the one an operator
	// cannot fix and must instead wait out: an insert costs 1600 of a default
	// 10,000 units a day, so the sixth upload of a day is the last.
	if code == http.StatusForbidden && strings.Contains(strings.ToLower(message), "quota") {
		return fmt.Errorf("%s: %w: YouTube quota exhausted — %s", what, provider.ErrUnavailable, message)
	}
	return fmt.Errorf("%s: youtube returned %d: %s", what, code, message)
}

// snippet bounds an unparseable body so a log line stays a line.
func snippet(body []byte) string {
	s := strings.TrimSpace(string(body))
	if s == "" {
		return "no body"
	}
	const limit = 240
	if len(s) > limit {
		return s[:limit] + "…"
	}
	return s
}
