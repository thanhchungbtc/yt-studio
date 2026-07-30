package ninerouter_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

func testRequest(chapters int) provider.BlueprintRequest {
	return provider.BlueprintRequest{
		VideoID:      "vid-1",
		VideoRef:     "DSS-14",
		ChannelSlug:  "deep-sleep-stories",
		Title:        "The Long Winter of the Harbour",
		Topic:        "a northern port town over one winter",
		ChapterCount: chapters,
		Style: entity.StyleConfig{
			Tone:            "calm, measured, nocturnal",
			Voice:           "amber-low",
			ImageStyle:      "muted watercolour",
			Language:        "en-US",
			WordsPerChapter: 420,
		},
	}
}

// chapter is one entry of the rich schema the prompt asks for.
func chapter(n int) map[string]any {
	return map[string]any{
		"order":     n,
		"title":     "Chapter " + strconv.Itoa(n),
		"mechanism": "mechanism " + strconv.Itoa(n),
		"core_concept": "The single precise idea chapter " + strconv.Itoa(n) +
			" teaches, in one clear paragraph.",
		"key_points": []string{
			"Open: mini-scene — a technician watching the harbour freeze",
			"use the 1995 ledger entry that records the last crossing",
			"name the temperature at which the bay closed",
		},
		"do_not_repeats":  []string{"entropy — introduced in 'Entropy' (ch 4) — do not re-derive it"},
		"forward_hook":    "But the ledger for February is written in pencil, and nobody knows why.",
		"tone":            "curious",
		"pacing":          "deep",
		"role":            "exploration",
		"estimated_words": 420,
	}
}

// outline builds a well-formed model response with n chapters.
func outline(n int) string {
	chapters := make([]map[string]any, 0, n)
	for i := 1; i <= n; i++ {
		chapters = append(chapters, chapter(i))
	}
	body, err := json.Marshal(map[string]any{
		"title":    "The Long Winter of the Harbour",
		"summary":  "A winter told through a harbour.",
		"chapters": chapters,
	})
	if err != nil {
		panic(err)
	}
	return string(body)
}

func TestBlueprintParsesAnOutline(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(outline(4)))
	c := newClient(t, g, "")

	bp, err := c.Blueprint(context.Background(), testRequest(4))
	if err != nil {
		t.Fatalf("Blueprint: %v", err)
	}
	if len(bp.Chapters) != 4 {
		t.Fatalf("chapters = %d, want 4", len(bp.Chapters))
	}
	if bp.Title != "The Long Winter of the Harbour" {
		t.Fatalf("title = %q", bp.Title)
	}
	if bp.AssetID == "" {
		t.Fatal("the outline was not stored")
	}
	for i, ch := range bp.Chapters {
		if ch.Ordinal != i+1 {
			t.Fatalf("chapter %d has ordinal %d, want %d", i, ch.Ordinal, i+1)
		}
	}
}

// The brief is what the script writer actually receives, through the port's
// one Summary field. Losing any of it here would mean the prompt did its work
// for nothing.
func TestBlueprintFoldsTheWholeBriefIntoSummary(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(outline(1)))
	c := newClient(t, g, "")

	bp, err := c.Blueprint(context.Background(), testRequest(1))
	if err != nil {
		t.Fatalf("Blueprint: %v", err)
	}
	summary := bp.Chapters[0].Summary
	for _, want := range []string{
		"The single precise idea chapter 1 teaches", // core_concept
		"Open: mini-scene — a technician watching",  // the opening directive
		"- use the 1995 ledger entry",               // a beat
		"Do not re-explain:",                        // the section
		"entropy — introduced in 'Entropy' (ch 4)",  // its entry
		"Hand-off: But the ledger for February",     // forward_hook
		"exploration · curious · deep · ~420 words", // the direction footer
	} {
		if !strings.Contains(summary, want) {
			t.Errorf("brief is missing %q:\n%s", want, summary)
		}
	}
	// The opening directive heads the list; the beats sit tight beneath it.
	if strings.Contains(summary, "\n\n- use the 1995") {
		t.Errorf("beats were blank-line separated:\n%s", summary)
	}
}

// A chapter with nothing but a concept must not produce a brief full of empty
// headings.
func TestBlueprintBriefOmitsEmptySections(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(
		`{"chapters":[{"title":"One","core_concept":"Just the idea."}]}`))
	c := newClient(t, g, "")

	bp, err := c.Blueprint(context.Background(), testRequest(1))
	if err != nil {
		t.Fatalf("Blueprint: %v", err)
	}
	if got := bp.Chapters[0].Summary; got != "Just the idea." {
		t.Fatalf("brief = %q, want just the concept", got)
	}
}

// The whole point of the two-phase DAG: the model returns what it found, and
// the adapter does not argue with the number. How far it may land from the
// brief is the use case's policy, enforced once, elsewhere.
func TestBlueprintDoesNotEnforceTheChapterCount(t *testing.T) {
	t.Parallel()
	for _, returned := range []int{1, 3, 45} {
		g := newGateway(t, http.StatusOK, completion(outline(returned)))
		c := newClient(t, g, "")

		bp, err := c.Blueprint(context.Background(), testRequest(50))
		if err != nil {
			t.Fatalf("asked for 50, model returned %d: %v", returned, err)
		}
		if len(bp.Chapters) != returned {
			t.Fatalf("chapters = %d, want the %d the model returned", len(bp.Chapters), returned)
		}
	}
}

// Order is reassigned from position, because it is the key the DAG and the
// chapter table are addressed by. Everything else the model said stands.
func TestBlueprintRenumbersChapters(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(
		`{"chapters":[{"order":7,"title":"A"},{"order":0,"title":"B"},{"order":7,"title":"C"}]}`))
	c := newClient(t, g, "")

	bp, err := c.Blueprint(context.Background(), testRequest(3))
	if err != nil {
		t.Fatalf("Blueprint: %v", err)
	}
	for i, ch := range bp.Chapters {
		if ch.Ordinal != i+1 {
			t.Fatalf("chapter %q has ordinal %d, want %d", ch.Title, ch.Ordinal, i+1)
		}
	}
}

// The store keeps the whole document, not the collapse. The port carries a
// title and a sentence today; the brief the model wrote must survive that so
// widening the port later is a change to the collapse, not a regeneration.
func TestBlueprintStoresTheFullDocument(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(outline(2)))
	store := newStore(t)
	c := newClientWithStore(t, g, "", store)

	bp, err := c.Blueprint(context.Background(), testRequest(2))
	if err != nil {
		t.Fatalf("Blueprint: %v", err)
	}

	file, err := store.Open(context.Background(), bp.AssetID, entity.AssetKindBlueprint)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("read stored asset: %v", err)
	}

	var stored struct {
		Video    string `json:"video"`
		Ref      string `json:"ref"`
		Chapters []struct {
			Order          int      `json:"order"`
			Mechanism      string   `json:"mechanism"`
			CoreConcept    string   `json:"core_concept"`
			KeyPoints      []string `json:"key_points"`
			DoNotRepeats   []string `json:"do_not_repeats"`
			ForwardHook    string   `json:"forward_hook"`
			Tone           string   `json:"tone"`
			Pacing         string   `json:"pacing"`
			Role           string   `json:"role"`
			EstimatedWords int      `json:"estimated_words"`
		} `json:"chapters"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("stored asset is not JSON: %v", err)
	}
	// Identity is stamped from the request rather than trusted from the model.
	if stored.Video != "vid-1" || stored.Ref != "DSS-14" {
		t.Fatalf("stored identity = %q/%q, want vid-1/DSS-14", stored.Video, stored.Ref)
	}
	if len(stored.Chapters) != 2 {
		t.Fatalf("stored chapters = %d, want 2", len(stored.Chapters))
	}
	first := stored.Chapters[0]
	if first.Mechanism == "" || first.CoreConcept == "" || len(first.KeyPoints) != 3 ||
		len(first.DoNotRepeats) != 1 || first.ForwardHook == "" ||
		first.Tone == "" || first.Pacing == "" || first.Role == "" || first.EstimatedWords != 420 {
		t.Fatalf("the stored brief lost fields: %+v", first)
	}
}

func TestBlueprintFallsBackToTheRequestedTitle(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(
		`{"title":"  ","chapters":[{"title":"One","core_concept":"A."}]}`))
	c := newClient(t, g, "")

	bp, err := c.Blueprint(context.Background(), testRequest(1))
	if err != nil {
		t.Fatalf("Blueprint: %v", err)
	}
	if bp.Title != "The Long Winter of the Harbour" {
		t.Fatalf("title = %q, want the requested one", bp.Title)
	}
}

// Only a response that cannot be read at all is rejected. What the model said
// is trusted: the tolerance band owns the count, and entity.NewChapter owns
// what makes a chapter well formed.
func TestBlueprintRejectsOnlyUnreadableResponses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		content string
	}{
		{"prose instead of JSON", "Chapter 1: Harbour Dusk\nChapter 2: Lantern Rain"},
		// Deliberate: the output contract is the last thing the system prompt
		// says, and a fenced response is a bad roll the scheduler asks again for
		// rather than a shape the adapter unwraps.
		{"JSON in a code fence", "```json\n{\"chapters\":[{\"title\":\"One\"}]}\n```"},
		{"a JSON array rather than an object", `[{"title":"One"}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := newGateway(t, http.StatusOK, completion(tc.content))
			_, err := newClient(t, g, "").Blueprint(context.Background(), testRequest(2))
			if err == nil {
				t.Fatal("expected an error")
			}
			// A bad roll is worth another attempt; it is not a misconfiguration.
			if errors.Is(err, provider.ErrUnavailable) {
				t.Fatalf("a bad roll was reported as unavailable: %v", err)
			}
		})
	}
}

// The error has to say what came back, or an operator cannot tell a chatty
// model from a broken gateway.
func TestBlueprintErrorCarriesTheResponse(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion("Sure! Here is your outline:"))
	_, err := newClient(t, g, "").Blueprint(context.Background(), testRequest(2))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "Sure! Here is your outline:") {
		t.Fatalf("error does not show the response: %v", err)
	}
}

// The prompt is the output contract and the brief. If a rule stops reaching the
// model, quality drops silently, so the rendering is asserted rather than
// assumed.
func TestBlueprintPromptCarriesTheRules(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(outline(2)))
	c := newClient(t, g, "")

	if _, err := c.Blueprint(context.Background(), testRequest(45)); err != nil {
		t.Fatalf("Blueprint: %v", err)
	}
	messages := g.messages(t)

	for _, want := range []string{
		"Sleepy Mind Lab",
		"NO DUPLICATES",
		"Orthogonal means",
		"mechanism",
		"ONLY letters and spaces",
		"Open: <style>",
		"estimated_words",
		"as if explaining to a 15-year-old",
		"concrete and true beats",
		"single JSON object",
		"no markdown code fences",
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

// The word budget is computed here because text/template cannot do arithmetic,
// so it is worth checking the arithmetic.
func TestBlueprintPromptCarriesTheBudget(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(outline(2)))
	c := newClient(t, g, "")

	// 45 chapters x 420 words = 18900 words, which is 145 minutes at 130 wpm.
	if _, err := c.Blueprint(context.Background(), testRequest(45)); err != nil {
		t.Fatalf("Blueprint: %v", err)
	}
	user := g.messages(t)["user"]
	for _, want := range []string{
		"The Long Winter of the Harbour",
		"a northern port town over one winter",
		"Target chapters: 45",
		"Target duration: 145 minutes",
		"18900 words",
		"numbered from 1",
	} {
		if !strings.Contains(user, want) {
			t.Errorf("user prompt is missing %q:\n%s", want, user)
		}
	}
}

// A channel with no per-chapter target must not brief the model with a budget
// of zero words.
func TestBlueprintPromptSubstitutesAWordBudget(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(outline(1)))
	c := newClient(t, g, "")

	req := testRequest(10)
	req.Style.WordsPerChapter = 0
	if _, err := c.Blueprint(context.Background(), req); err != nil {
		t.Fatalf("Blueprint: %v", err)
	}
	want := strconv.Itoa(10 * entity.DefaultWordsPerChapter)
	if user := g.messages(t)["user"]; !strings.Contains(user, want+" words") {
		t.Errorf("user prompt did not substitute the domain default:\n%s", user)
	}
}
