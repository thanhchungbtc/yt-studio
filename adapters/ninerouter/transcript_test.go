package ninerouter_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tbui/yt-studio/adapters/ninerouter"
	"github.com/tbui/yt-studio/domain/provider"
)

// newRecordingClient wires a client that writes transcripts into a temporary
// directory, and returns the directory.
func newRecordingClient(t *testing.T, g *gateway) (*ninerouter.Client, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "transcripts")
	c, err := ninerouter.New(ninerouter.Config{
		BaseURL:       g.server.URL,
		Model:         testModel,
		Timeout:       5 * time.Second,
		TranscriptDir: dir,
	}, newStore(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, dir
}

// transcriptFiles returns every transcript written, by path.
func transcriptFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			out = append(out, path)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return out
}

func readOnly(t *testing.T, dir string) string {
	t.Helper()
	files := transcriptFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("wrote %d transcripts, want 1: %v", len(files), files)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read %s: %v", files[0], err)
	}
	return string(raw)
}

// The whole exchange has to be there, or reading one afterwards answers
// nothing.
func TestTranscriptRecordsTheExchange(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(outline(2)))
	c, dir := newRecordingClient(t, g)

	if _, err := c.Blueprint(context.Background(), testRequest(2)); err != nil {
		t.Fatalf("Blueprint: %v", err)
	}
	body := readOnly(t, dir)

	for _, want := range []string{
		"# blueprint",
		"- model: " + testModel,
		"- video: vid-1",
		"## system",
		"Sleepy Mind Lab", // the system prompt, as sent
		"## user",         // the assignment
		"Target chapters: 2",
		"## response",
		"The Long Winter of the Harbour", // what came back
	} {
		if !strings.Contains(body, want) {
			t.Errorf("transcript is missing %q:\n%s", want, body)
		}
	}
}

// The failing call is the one most worth reading, which is the whole reason
// these are files rather than assets.
func TestTranscriptRecordsFailures(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusUnauthorized, gatewayError("[cc/x] [401]: token expired"))
	c, dir := newRecordingClient(t, g)

	if _, err := c.Blueprint(context.Background(), testRequest(2)); err == nil {
		t.Fatal("expected an error")
	}
	body := readOnly(t, dir)
	if !strings.Contains(body, "## error") || !strings.Contains(body, "token expired") {
		t.Errorf("the failure was not recorded:\n%s", body)
	}
	// The prompt that produced it is the point.
	if !strings.Contains(body, "## system") || !strings.Contains(body, "## user") {
		t.Errorf("the failing transcript dropped its prompts:\n%s", body)
	}
}

// A model that answered but said nothing usable still produced a transcript
// worth keeping, and the token count is how the cost of a call is known at all.
func TestTranscriptRecordsUsage(t *testing.T) {
	t.Parallel()
	reply := `{"choices":[{"index":0,"message":{"role":"assistant","content":` +
		`"{\"chapters\":[{\"title\":\"One\"}]}"},"finish_reason":"stop"}],` +
		`"usage":{"prompt_tokens":2586,"completion_tokens":1204,"total_tokens":3790}}`
	g := newGateway(t, http.StatusOK, reply)
	c, dir := newRecordingClient(t, g)

	if _, err := c.Blueprint(context.Background(), testRequest(1)); err != nil {
		t.Fatalf("Blueprint: %v", err)
	}
	if body := readOnly(t, dir); !strings.Contains(body, "2586 in, 1204 out, 3790 total") {
		t.Errorf("token usage was not recorded:\n%s", body)
	}
}

// One file per call, grouped by video, named so they sort in the order they
// happened.
func TestTranscriptsAreOnePerCallGroupedByVideo(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(narration))
	c, dir := newRecordingClient(t, g)

	for _, ordinal := range []int{1, 2} {
		req := scriptRequest()
		req.Ordinal = ordinal
		if _, err := c.Script(context.Background(), req); err != nil {
			t.Fatalf("Script ch%d: %v", ordinal, err)
		}
	}

	files := transcriptFiles(t, dir)
	if len(files) != 2 {
		t.Fatalf("wrote %d transcripts, want 2: %v", len(files), files)
	}
	for _, f := range files {
		if filepath.Base(filepath.Dir(f)) != "vid-1" {
			t.Errorf("%s is not filed under its video", f)
		}
	}
	joined := strings.Join(files, "\n")
	for _, want := range []string{"script-ch1", "script-ch2"} {
		if !strings.Contains(joined, want) {
			t.Errorf("no transcript named for %q:\n%s", want, joined)
		}
	}
}

// Off is the default, so a daemon that never asked for transcripts never grows
// a directory of them.
func TestTranscriptsAreOffWithoutADirectory(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(outline(1)))
	c := newClient(t, g, "")

	if _, err := c.Blueprint(context.Background(), testRequest(1)); err != nil {
		t.Fatalf("Blueprint: %v", err)
	}
	// Nothing to assert beyond not panicking on a nil writer, which is the
	// disabled state.
}

// An unusable path is a wiring error. Finding out at the first generation of a
// fifty-chapter video would be finding out too late.
func TestUnusableTranscriptDirectoryFailsAtConstruction(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ninerouter.New(ninerouter.Config{
		BaseURL:       "http://localhost:20128",
		Model:         testModel,
		TranscriptDir: filepath.Join(file, "under-a-file"),
	}, newStore(t))
	if !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("New = %v, want ErrUnavailable", err)
	}
}
