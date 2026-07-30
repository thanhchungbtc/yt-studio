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

// outlineFixture is a four-chapter plan; chapter 3 is the one under test.
func outlineFixture() provider.BlueprintOutline {
	return provider.BlueprintOutline{
		Title:   "The Long Winter of the Harbour",
		Summary: "a northern port town over one winter",
		Chapters: []provider.BlueprintChapter{
			{Ordinal: 1, Title: "Intro", Summary: "Welcome to the harbour.", EstimatedWords: 160},
			{Ordinal: 2, Title: "First Ice Creeps", Summary: "Sea ice closes the bay.", EstimatedWords: 300},
			{Ordinal: 3, Title: "The Ferry Halts", Summary: "December closes the crossing.\n\n" +
				"Open: mini-scene — the last passenger stepping onto the ice\n" +
				"- name the date the ferry stopped\n\n" +
				"Do not re-explain:\n- sea ice — introduced in 'First Ice Creeps' (ch 2)\n\n" +
				"Hand-off: Nobody had told them how long it would last.\n\n" +
				"exploration · unsettling · deep · ~280 words", EstimatedWords: 280},
			{Ordinal: 4, Title: "Frozen Ink Lines", Summary: "The inkwell freezes.", EstimatedWords: 340},
		},
	}
}

func scriptRequest() provider.ScriptRequest {
	return provider.ScriptRequest{
		VideoID:     "vid-1",
		ChapterID:   "vid-1:ch:3",
		Ordinal:     3,
		Blueprint:   outlineFixture(),
		TargetWords: 280,
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
// twice a short one — so the chapter's own budget must win over the channel
// average, or the pacing the outline was built around is flattened.
func TestScriptTakesTheWordBudgetFromTheBlueprint(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(narration))
	c := newClient(t, g, "")

	if _, err := c.Script(context.Background(), scriptRequest()); err != nil {
		t.Fatalf("Script: %v", err)
	}
	user := g.messages(t)["user"]
	if !strings.Contains(user, "about 280 words") {
		t.Errorf("the chapter's budget of 280 did not reach the prompt:\n%s", user)
	}
	if strings.Contains(user, "about 420 words") {
		t.Errorf("the channel average overrode the chapter's own budget:\n%s", user)
	}
}

func TestScriptFallsBackToTheChannelBudget(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		style entity.StyleConfig
		want  int
	}{
		{"blueprint assigned none", entity.StyleConfig{WordsPerChapter: 420}, 420},
		{"channel has no target either", entity.StyleConfig{}, entity.DefaultWordsPerChapter},
	}
	// Clearing the outline entry too: the resolved budget and the entry's own
	// figure are both zero, so only the channel is left to answer.
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := newGateway(t, http.StatusOK, completion(narration))
			c := newClient(t, g, "")

			req := scriptRequest()
			req.Style, req.TargetWords = tc.style, 0
			for i := range req.Blueprint.Chapters {
				req.Blueprint.Chapters[i].EstimatedWords = 0
			}
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

	// Fragments, not sentences: the template is hard-wrapped, so a phrase that
	// reads as one line here may straddle two there.
	for _, want := range []string{
		"Sleepy Mind Lab",
		"15-year-old",
		"Target Audience",
		"Warm & Conversational",
		"Late Night Pacing",
		"TTS Rules",
		"Spell out all numbers",
		"Forbidden Patterns",
		"not just X but Y",
	} {
		if !strings.Contains(messages["system"], want) {
			t.Errorf("system prompt is missing %q", want)
		}
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

// The writer sees the whole plan, in order, with its own chapter marked. That
// is what lets it build on chapter 2 and leave room for chapter 4 instead of
// re-deriving the same ground under a different title.
func TestScriptPromptCarriesTheWholeOutline(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(narration))
	c := newClient(t, g, "")

	if _, err := c.Script(context.Background(), scriptRequest()); err != nil {
		t.Fatalf("Script: %v", err)
	}
	user := g.messages(t)["user"]

	for _, want := range []string{
		"Title: The Long Winter of the Harbour",
		"Summary: a northern port town over one winter",
		"Chapter structure (in order):",
		"Chapter 1: Intro",
		"Chapter 2: First Ice Creeps",
		"Chapter 3: The Ferry Halts",
		"Chapter 4: Frozen Ink Lines",
		"Sea ice closes the bay.", // an earlier chapter's brief
		"The inkwell freezes.",    // a later one's
		"about 340 words",         // each entry carries its own budget
		"Your Assignment",
	} {
		if !strings.Contains(user, want) {
			t.Errorf("user prompt is missing %q:\n%s", want, user)
		}
	}

	// Exactly one chapter is marked, and it is the one being written.
	marker := ">>> YOU ARE WRITING THIS ONE <<<"
	if n := strings.Count(user, marker); n != 1 {
		t.Fatalf("the outline marks %d chapters, want exactly 1", n)
	}
	before := user[:strings.Index(user, marker)]
	if !strings.Contains(before, "Chapter 2: First Ice Creeps") ||
		strings.Contains(before, "Chapter 4: Frozen Ink Lines") {
		t.Errorf("the marker is not on chapter 3:\n%s", user)
	}
}

// Ordinals are how the assignment and the outline entry are tied together, so a
// request naming a chapter the outline does not contain is a bug worth saying
// out loud rather than writing a script for the wrong thing.
func TestScriptRejectsAnOrdinalOutsideTheOutline(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(narration))
	c := newClient(t, g, "")

	req := scriptRequest()
	req.Ordinal = 99
	if _, err := c.Script(context.Background(), req); err == nil {
		t.Fatal("expected an error")
	} else if !strings.Contains(err.Error(), "not in the outline") {
		t.Fatalf("error = %v, want it to name the missing chapter", err)
	}
	if g.called != 0 {
		t.Error("the gateway was called for a chapter that is not in the plan")
	}
}

// The assignment repeats the chapter's own brief in full, since that is what
// the writer works from; the outline entries are context around it.
func TestScriptAssignmentCarriesTheChapterBrief(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(narration))
	c := newClient(t, g, "")

	if _, err := c.Script(context.Background(), scriptRequest()); err != nil {
		t.Fatalf("Script: %v", err)
	}
	user := g.messages(t)["user"]
	assignment := user[strings.Index(user, "Your Assignment"):]
	for _, want := range []string{
		"Chapter 3: The Ferry Halts",
		"about 280 words",
		"Open: mini-scene",
		"Do not re-explain:",
		"Hand-off: Nobody had told them",
		"calm, measured, nocturnal",
	} {
		if !strings.Contains(assignment, want) {
			t.Errorf("assignment is missing %q:\n%s", want, assignment)
		}
	}
}
