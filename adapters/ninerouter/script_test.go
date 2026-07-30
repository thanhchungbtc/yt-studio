package ninerouter_test

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// narration is what a well-behaved model returns: prose, and nothing else.
const narration = "The last grain ship left on a Tuesday. " +
	"The harbour master wrote the time in the ledger, and then he wrote the temperature, " +
	"and then he closed the book for the winter."

func scriptRequest() provider.ScriptRequest {
	return provider.ScriptRequest{
		VideoID:      "vid-1",
		ChapterID:    "vid-1:ch:3",
		Ordinal:      3,
		ChapterTitle: "The Ferry Halts",
		ChapterSummary: "December closes the crossing.\n\n" +
			"Open: mini-scene — the last passenger stepping onto the ice\n" +
			"- name the date the ferry stopped\n\n" +
			"Do not re-explain:\n- sea ice — introduced in 'First Ice Creeps' (ch 2)\n\n" +
			"Hand-off: Nobody had told them how long it would last.\n\n" +
			"exploration · unsettling · deep · ~280 words",
		BlueprintTitle:   "The Long Winter of the Harbour",
		BlueprintSummary: "a northern port town over one winter",
		Style: entity.StyleConfig{
			Tone:            "calm, measured, nocturnal",
			Language:        "en-US",
			WordsPerChapter: 420,
		},
	}
}

// The script is prose, not JSON, so the completion is the narration verbatim.
// Anything else here would be the adapter editing what gets read aloud.
func TestScriptReturnsTheCompletionVerbatim(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(narration))
	c := newClient(t, g, "")

	script, err := c.Script(context.Background(), scriptRequest())
	if err != nil {
		t.Fatalf("Script: %v", err)
	}
	if script.Text != narration {
		t.Fatalf("text = %q, want it unchanged", script.Text)
	}
	if want := len(strings.Fields(narration)); script.WordCount != want {
		t.Fatalf("word count = %d, want %d", script.WordCount, want)
	}
	if script.AssetID == "" {
		t.Fatal("the narration was not stored")
	}
}

func TestScriptStoresThePlainText(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(narration))
	store := newStore(t)
	c := newClientWithStore(t, g, "", store)

	script, err := c.Script(context.Background(), scriptRequest())
	if err != nil {
		t.Fatalf("Script: %v", err)
	}
	file, err := store.Open(context.Background(), script.AssetID, entity.AssetKindScript)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("read stored asset: %v", err)
	}
	if string(raw) != narration {
		t.Fatalf("stored %q, want the narration", string(raw))
	}
}

// The blueprint budgets each chapter unevenly — a deep chapter carries roughly
// twice a short one — and that number reaches us inside the brief, because
// ScriptRequest carries a summary rather than a plan. Falling back to the
// channel average would flatten the pacing the outline was built around.
func TestScriptTakesTheWordBudgetFromTheBrief(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(narration))
	c := newClient(t, g, "")

	if _, err := c.Script(context.Background(), scriptRequest()); err != nil {
		t.Fatalf("Script: %v", err)
	}
	user := g.messages(t)["user"]
	if !strings.Contains(user, "about 280 words") {
		t.Errorf("the brief's budget of 280 did not reach the prompt:\n%s", user)
	}
	if strings.Contains(user, "about 420 words") {
		t.Errorf("the channel average overrode the chapter's own budget:\n%s", user)
	}
}

func TestScriptFallsBackToTheChannelBudget(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		style   entity.StyleConfig
		summary string
		want    int
	}{
		{"brief carries no footer", entity.StyleConfig{WordsPerChapter: 420}, "Just a concept.", 420},
		{"channel has no target", entity.StyleConfig{}, "Just a concept.", entity.DefaultWordsPerChapter},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := newGateway(t, http.StatusOK, completion(narration))
			c := newClient(t, g, "")

			req := scriptRequest()
			req.Style, req.ChapterSummary = tc.style, tc.summary
			if _, err := c.Script(context.Background(), req); err != nil {
				t.Fatalf("Script: %v", err)
			}
			want := "about " + strconv.Itoa(tc.want) + " words"
			if user := g.messages(t)["user"]; !strings.Contains(user, want) {
				t.Errorf("prompt is missing %q:\n%s", want, user)
			}
		})
	}
}

// The brief is the whole instruction set for this chapter. If it stops reaching
// the model, the writer invents its own chapter and nothing fails loudly.
func TestScriptPromptCarriesTheBrief(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(narration))
	c := newClient(t, g, "")

	if _, err := c.Script(context.Background(), scriptRequest()); err != nil {
		t.Fatalf("Script: %v", err)
	}
	messages := g.messages(t)

	for _, want := range []string{
		"The Long Winter of the Harbour",            // the video
		"a northern port town over one winter",      // its topic
		"Chapter 3: The Ferry Halts",                // which chapter
		"Open: mini-scene",                          // the opening directive
		"- name the date the ferry stopped",         // a beat
		"Do not re-explain:",                        // the section
		"sea ice — introduced in 'First Ice Creeps", // its entry
		"Hand-off: Nobody had told them",            // the forward hook
		"calm, measured, nocturnal",                 // the channel tone
	} {
		if !strings.Contains(messages["user"], want) {
			t.Errorf("user prompt is missing %q:\n%s", want, messages["user"])
		}
	}

	// Fragments, not sentences: the templates are hard-wrapped, so a phrase that
	// reads as one line here may straddle two there.
	for _, want := range []string{
		"Sleepy Mind",
		"15-year-old",
		"perform it",                  // the Open directive is acted on, not restated
		"re-derive them from scratch", // do_not_repeats
		"end on it",                   // the hand-off
		"Hit it",                      // the word budget
		"read aloud",                  // the TTS constraint
		"no code fences",              // the output contract
	} {
		if !strings.Contains(messages["system"], want) {
			t.Errorf("system prompt is missing %q", want)
		}
	}
	// The output contract is last, so a long prompt cannot bury it.
	if idx := strings.LastIndex(messages["system"], "## Output"); idx < len(messages["system"])/2 {
		t.Error("the output contract is not in the final half of the system prompt")
	}
}

// Script goes through the same seam as Blueprint, so a gateway failure carries
// the same retry class rather than a second opinion.
func TestScriptPropagatesGatewayFailures(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusUnauthorized, gatewayError("[cc/x] [401]: token expired"))
	if _, err := newClient(t, g, "").Script(context.Background(), scriptRequest()); err == nil {
		t.Fatal("expected an error")
	} else if !strings.Contains(err.Error(), "token expired") {
		t.Fatalf("error lost the gateway's message: %v", err)
	}
}
