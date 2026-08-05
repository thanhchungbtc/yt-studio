package ninerouter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
)

// Transcripts are the record of what was actually sent to a model and what came
// back. They exist for one job: reading a generation afterwards to work out why
// it came out the way it did.
//
// They are files rather than assets on purpose. An asset needs an ownership row
// only the app layer can write, and the sweep reclaims what has none — so the
// failed call, the one most worth reading, is exactly the one that would be
// collected. A file on disk has no such opinion.
//
// Writing one never fails a generation. The directory is created when the
// client is constructed, so a bad path is a wiring error rather than something
// discovered halfway through a fifty-chapter video.

// call names the generation a transcript belongs to.
type call struct {
	Video entity.VideoID
	// Label is the filename's readable half, e.g. "blueprint" or "script-ch12".
	Label string
}

// transcript is one recorded exchange.
type transcript struct {
	call
	Model     string
	System    string
	User      string
	Response  string
	Err       error
	Usage     *chatUsage
	StartedAt time.Time
	Duration  time.Duration
}

// transcriptSeq disambiguates two calls landing in the same millisecond, which
// the slide-prompt fan-out makes likely.
var transcriptSeq atomic.Uint64

// transcriptWriter appends exchanges under a directory, one file each.
type transcriptWriter struct{ dir string }

// newTranscriptWriter returns nil when no directory is configured, which is how
// the feature is switched off: every call site is a nil check away from doing
// nothing.
func newTranscriptWriter(dir string) (*transcriptWriter, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, nil //nolint:nilnil // a nil writer is the disabled state
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create transcript directory %s: %w", dir, err)
	}
	return &transcriptWriter{dir: filepath.Clean(dir)}, nil
}

// write records one exchange, best effort. A transcript that cannot be written
// is not a reason to fail work that already succeeded.
func (w *transcriptWriter) write(t transcript) {
	if w == nil {
		return
	}
	dir := filepath.Join(w.dir, safeSegment(string(t.Video)))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	name := fmt.Sprintf("%s-%03d-%s.md",
		t.StartedAt.UTC().Format("20060102T150405.000"),
		transcriptSeq.Add(1)%1000,
		safeSegment(t.Label))
	_ = os.WriteFile(filepath.Join(dir, name), []byte(t.render()), 0o600)
}

// render lays the exchange out for reading, not for parsing.
func (t transcript) render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", t.Label)
	fmt.Fprintf(&b, "- video: %s\n", t.Video)
	fmt.Fprintf(&b, "- model: %s\n", t.Model)
	fmt.Fprintf(&b, "- started: %s\n", t.StartedAt.UTC().Format(time.RFC3339Nano))
	fmt.Fprintf(&b, "- duration: %s\n", t.Duration.Round(time.Millisecond))
	if u := t.Usage; u != nil {
		fmt.Fprintf(&b, "- tokens: %d in, %d out, %d total\n",
			u.PromptTokens, u.CompletionTokens, u.TotalTokens)
	}

	b.WriteString("\n## system\n\n")
	b.WriteString(t.System)
	b.WriteString("\n\n## user\n\n")
	b.WriteString(t.User)

	if t.Err != nil {
		b.WriteString("\n\n## error\n\n")
		b.WriteString(t.Err.Error())
	}
	if t.Response != "" {
		b.WriteString("\n\n## response\n\n")
		b.WriteString(t.Response)
	}
	b.WriteString("\n")
	return b.String()
}

// safeSegment keeps a path component to characters a filename can hold, so a
// label or an id never escapes the directory it belongs in.
func safeSegment(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		return "unknown"
	}
	return out
}
