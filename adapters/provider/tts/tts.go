// Package tts is what the narration backends share: RIFF/WAVE surgery, the
// text a narrator is actually handed, and the HTTP plumbing for talking to a
// speech server.
//
// The backends themselves live one directory down — xtts for an AllTalk server,
// kokoro for an OpenAI-compatible one — because a package named for a port
// holding two vendors is a package nobody can name a type in. What stayed here
// is what both of them need: every backend cleans the tail of what it is given
// and prefixes the chapter's title, and both read a WAV.
package tts

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/tbui/yt-studio/domain/provider"
)

// ErrUnavailable reports audio or a server this package cannot work with.
// It wraps the port's sentinel so app.classify fails the task once rather than
// retrying it; a backend that wants its own name in the message wraps
// provider.ErrUnavailable itself and hands that to StatusError.
var ErrUnavailable = fmt.Errorf("tts: %w", provider.ErrUnavailable)

// IntroOrdinal is the first chapter, the one whose title is not announced
// because it has no topic to read out. Chapters are 1-based here.
const IntroOrdinal = 1

// The response-body ceilings: an error body describes a failure rather than
// being kept, and a readiness probe answers with one word or one list.
const (
	ReplyBodyLimit = 64 << 10
	ReadyBodyLimit = 64 << 10
)

// snippetLimit is enough of a response to recognise what came back, without
// turning a log line into a transcript.
const snippetLimit = 240

// Snippet flattens and truncates text for an error message.
func Snippet(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= snippetLimit {
		return s
	}
	return s[:snippetLimit] + "…"
}

// StatusError turns a non-200 into an error of the right retry class: a
// rejected voice cannot land differently, a rate limit or a loading model can.
//
// The two names are the caller's, not this package's: unavailable is its own
// sentinel and backend is what it is called, so an error reads as coming from
// the server that answered rather than from the layer that parsed the reply.
func StatusError(unavailable error, backend string, code int, status string, body []byte) error {
	detail := Snippet(string(body))
	switch {
	case code == http.StatusTooManyRequests, code >= 500:
		return fmt.Errorf("%s: %s: %s", backend, status, detail)
	case code >= 400:
		return fmt.Errorf("%w: %s: %s", unavailable, status, detail)
	default:
		return fmt.Errorf("%s: unexpected %s: %s", backend, status, detail)
	}
}

// Normalize is the only tidying a script gets: surrounding whitespace, nothing
// else. What the model was told to write is what the narrator reads.
func Normalize(text string) string {
	return strings.TrimSpace(text)
}

// PrependChapterTitle prefixes the narration so the narrator announces the
// title. The intro is spoken as it stands, having no topic to read. The period
// is what makes the narrator pause instead of running on into the first
// sentence.
func PrependChapterTitle(body, title string, isIntro bool) string {
	title = strings.TrimSpace(title)
	if isIntro || title == "" {
		return body
	}
	return title + ".\n\n" + body
}
