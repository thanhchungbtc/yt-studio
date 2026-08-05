package xtts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// ErrUnavailable reports a server that cannot serve this request until someone
// changes something: an unreachable host, a voice that does not exist, a
// rejected language. It wraps the port's sentinel, so app.classify fails the
// task once and says why rather than spending its retries on an answer that
// will not change.
var ErrUnavailable = fmt.Errorf("xtts: %w", provider.ErrUnavailable)

// errNotImplemented is what every unwritten body returns. It is deliberately an
// error rather than a zero value: a stub that quietly returned empty audio
// would be stored, content-addressed and composed into a video before anyone
// noticed the silence.
var errNotImplemented = errors.New("xtts: not implemented")

// The two endpoints this package uses, relative to Config.BaseURL.
const (
	endpointGenerate = "/api/tts-generate"
	endpointReady    = "/api/ready"
)

// introOrdinal is the first chapter. Chapters are 1-based here (the Python this
// is ported from was 0-based), and the intro is the one chapter whose title is
// not announced — it has no topic to read out.
const introOrdinal = 1

// The tail cleanup constants, carried over unchanged: trim trailing samples
// below the threshold, then ramp the last fade to zero so whatever artefact
// survives the trim is attenuated below audibility.
const (
	defaultFadeMillis       = 40
	defaultSilenceThreshold = 0.005
)

// defaultTimeout bounds one HTTP request, not one chapter.
//
// It is generous because a chunk on a CPU-only server genuinely takes minutes,
// and cutting it off early costs the whole chapter rather than the chunk. The
// cost of the generosity is that a hung call holds its TTS pool slot for the
// duration; cancelling the video is the faster way out, since the per-video
// context aborts an in-flight call promptly.
const defaultTimeout = 20 * time.Minute

// Options are the knobs that belong in the settings table, read per call so an
// edit on the settings screen applies to the next chapter rather than the next
// restart — the same reason the registry resolves its backend per call.
type Options struct {
	// Voice is the server-side voice file, e.g. `female_01.wav`.
	Voice string
	// Language is the two-letter code the model is asked to speak.
	Language string
	// Speed is the playback rate the server is asked for; 1.0 is unmodified.
	Speed float64
	// ChunkMinChars is the floor on a chunk's length in characters. The chunk
	// count follows from it (len(text) / ChunkMinChars), so it sets the size of
	// the pieces rather than their number.
	ChunkMinChars int
	// ChunkSilenceMillis is the pause inserted between chunks when they are
	// rejoined, so a sentence boundary does not become a splice.
	ChunkSilenceMillis int
}

// Config is everything needed to reach one AllTalk instance.
type Config struct {
	// BaseURL is the server root, e.g. http://127.0.0.1:7851. The endpoints are
	// appended here rather than configured, and the audio URL a generation
	// returns is resolved against it.
	BaseURL string
	// Timeout bounds one request; zero means defaultTimeout.
	Timeout time.Duration
	// Options resolves the settings-sourced knobs. It is a function for the
	// reason given on Options itself.
	Options func() Options
}

// Client is the narration backend.
type Client struct {
	cfg   Config
	http  *http.Client
	store provider.AssetStore
}

var _ provider.TTSProvider = (*Client)(nil)

// New validates the configuration and wires the client. It touches no network:
// wiring cannot fail because a server is down, and Check is what reports that.
//
// TODO: reject an empty or non-absolute BaseURL and a nil Options as
// ErrUnavailable, the way ninerouter.New does — a bad flag should fail at
// startup, not at the first chapter of fifty.
func New(cfg Config, store provider.AssetStore) (*Client, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	return &Client{
		cfg:   cfg,
		http:  &http.Client{Timeout: cfg.Timeout},
		store: store,
	}, nil
}

// Check probes the server so an operator learns it is unreachable at startup
// rather than from the first chapter of a fifty-chapter video.
//
// TODO: GET BaseURL+endpointReady and require the ready response. Do not cache
// the result: a server that was down at boot may be up now, and a remembered
// failure would keep saying otherwise.
func (c *Client) Check(_ context.Context) error {
	return errNotImplemented
}

// Speak narrates exactly one chapter and returns the audio's content address.
//
// The body below is the order of operations, ported from the Python; every step
// it calls is still a stub.
func (c *Client) Speak(ctx context.Context, req provider.SpeakRequest) (entity.AssetID, error) {
	opts := c.options()

	text := normalize(req.Text)

	text = prependChapterTitle(text, "", req.Ordinal == introOrdinal)

	chunks := chunkTextBySentence(text, opts.ChunkMinChars)
	if len(chunks) == 0 {
		chunks = []string{text}
	}

	parts := make([][]byte, 0, len(chunks))
	for _, chunk := range chunks {
		part, err := c.synthesize(ctx, chunk, opts)
		if err != nil {
			return "", err
		}
		parts = append(parts, part)
	}

	joined, err := concatWavs(parts, opts.ChunkSilenceMillis)
	if err != nil {
		return "", err
	}
	audio := cleanTail(joined, defaultFadeMillis, defaultSilenceThreshold)

	stored, err := c.store.Put(ctx, entity.AssetKindAudio, bytes.NewReader(audio))
	if err != nil {
		return "", fmt.Errorf("store narration: %w", err)
	}
	return stored.ID, nil
}

// synthesize speaks one chunk and returns the WAV bytes.
//
// TODO: two requests.
//
//  1. POST BaseURL+endpointGenerate, form-encoded, with text_input,
//     character_voice_gen, language and speed. The response is JSON carrying
//     output_file_url.
//  2. GET that URL and read the body — that is the audio.
//
// Classify the failures on the way out, the way ninerouter.statusError does:
// 4xx wraps ErrUnavailable, because a bad voice name is not fixed by a second
// attempt; 429 and 5xx stay plain errors, which is what backoff is for.
func (c *Client) synthesize(_ context.Context, _ string, _ Options) ([]byte, error) {
	return nil, errNotImplemented
}

// audioURL resolves what a generation reported into something fetchable.
//
// TODO: output_file_url is a server-root-relative path on the builds this was
// written against, but absolute URLs come back from others. Parse it: absolute
// is used as it stands, relative is joined onto BaseURL. String concatenation
// works until the day it silently does not.
func (c *Client) audioURL(_ string) (string, error) {
	return "", errNotImplemented
}

// options reads the current settings, with the zero value meaning "unset"
// rather than "zero" for anything a server would reject.
//
// TODO: fall back to sane defaults for a nil Options func and for a zero Speed
// or ChunkMinChars, so a half-configured client still speaks.
func (c *Client) options() Options {
	if c.cfg.Options == nil {
		return Options{}
	}
	return c.cfg.Options()
}

// normalize is the only tidying done to a script before it is spoken: leading
// and trailing whitespace, nothing else. What the model was told to write is
// what the narrator reads.
//
// TODO: strip.
func normalize(text string) string {
	return text
}
