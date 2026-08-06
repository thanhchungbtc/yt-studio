package ninerouter_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tbui/yt-studio/adapters/ninerouter"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// videoContext is the plan an slide-prompt generation illustrates. The port
// hands the method an id and nothing else, so the caller resolves it.
func videoContext(chapters, perChapter int) ninerouter.VideoContext {
	out := ninerouter.VideoContext{SlidesPerChapter: perChapter}
	out.Title = "The Long Winter of the Harbour"
	out.Summary = "a northern port town over one winter"
	for i := 1; i <= chapters; i++ {
		out.Chapters = append(out.Chapters, provider.BlueprintChapter{
			Ordinal: i,
			Title:   "Chapter " + strconv.Itoa(i),
			Summary: "What chapter " + strconv.Itoa(i) + " is about.",
		})
	}
	return out
}

// promptBatch is a well-formed reply covering every requested asset.
func promptBatch(chapters, perChapter int) string {
	type entry struct {
		Chapter int    `json:"chapter"`
		Index   int    `json:"index"`
		Prompt  string `json:"prompt"`
	}
	var out []entry
	for c := 1; c <= chapters; c++ {
		for i := range perChapter {
			out = append(out, entry{c, i, fmt.Sprintf("a symbol for chapter %d image %d", c, i)})
		}
	}
	body, err := json.Marshal(map[string]any{"prompts": out})
	if err != nil {
		panic(err)
	}
	return string(body)
}

func newSlideClient(t *testing.T, g *gateway, vc ninerouter.VideoContext) *ninerouter.Client {
	t.Helper()
	c, err := ninerouter.New(ninerouter.Config{
		BaseURL: g.server.URL, Model: staticModel(testModel), Timeout: 5 * time.Second,
	}, newStore(t), func(context.Context, entity.VideoID) (ninerouter.VideoContext, error) {
		return vc, nil
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestSlidePromptsCoversEveryAsset(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(promptBatch(3, 2)))
	c := newSlideClient(t, g, videoContext(3, 2))

	prompts, err := c.SlidePrompts(context.Background(), "vid-1")
	if err != nil {
		t.Fatalf("SlidePrompts: %v", err)
	}
	if len(prompts) != 6 {
		t.Fatalf("prompts = %d, want 6", len(prompts))
	}
	seen := map[[2]int]bool{}
	for _, p := range prompts {
		seen[[2]int{p.Ordinal, p.Index}] = true
		if p.Prompt == "" {
			t.Errorf("chapter %d image %d has no prompt", p.Ordinal, p.Index)
		}
	}
	for chapter := 1; chapter <= 3; chapter++ {
		for index := range 2 {
			if !seen[[2]int{chapter, index}] {
				t.Errorf("nothing addresses chapter %d image %d", chapter, index)
			}
		}
	}
}

// The port's contract: N per-chapter callers, one real generation. Both halves
// matter — singleflight only collapses callers that overlap in time, and the
// image pool is capped well below the number of them.
func TestSlidePromptsCoalescesConcurrentCallers(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	release := make(chan struct{})
	g := newGateway(t, http.StatusOK, completion(promptBatch(4, 2)))
	// Hold the first caller inside the gateway until every other has queued
	// behind it, which is what makes this a race the cache alone cannot win.
	g.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(completion(promptBatch(4, 2))))
	})
	c := newSlideClient(t, g, videoContext(4, 2))

	const callers = 8
	var wg sync.WaitGroup
	results := make([][]provider.SlidePrompt, callers)
	errs := make([]error, callers)
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = c.SlidePrompts(context.Background(), "vid-1")
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d: %v", i, err)
		}
		if len(results[i]) != 8 {
			t.Fatalf("caller %d got %d prompts, want 8", i, len(results[i]))
		}
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("%d generations for one video, want exactly 1", n)
	}
}

// A caller arriving after the production finished is served from the cache, not
// from a second generation.
func TestSlidePromptsServesLaterCallersFromCache(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(promptBatch(2, 1)))
	c := newSlideClient(t, g, videoContext(2, 1))

	for i := range 5 {
		if _, err := c.SlidePrompts(context.Background(), "vid-1"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if g.called != 1 {
		t.Fatalf("%d gateway calls, want 1", g.called)
	}
}

// Retrying a chapter must not replay the batch it was retried away from.
func TestForgetDropsTheBatch(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(promptBatch(2, 1)))
	c := newSlideClient(t, g, videoContext(2, 1))

	if _, err := c.SlidePrompts(context.Background(), "vid-1"); err != nil {
		t.Fatalf("SlidePrompts: %v", err)
	}
	c.Forget("vid-1")
	if _, err := c.SlidePrompts(context.Background(), "vid-1"); err != nil {
		t.Fatalf("SlidePrompts after Forget: %v", err)
	}
	if g.called != 2 {
		t.Fatalf("%d gateway calls, want 2: Forget did not drop the batch", g.called)
	}
}

// A batch that misses assets leaves per-chapter tasks with nothing to draw
// from, so it is a bad roll worth asking again rather than a hard failure.
func TestSlidePromptsRejectsAShortBatch(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(promptBatch(2, 1)))
	c := newSlideClient(t, g, videoContext(4, 2))

	_, err := c.SlidePrompts(context.Background(), "vid-1")
	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("a short batch was reported as unavailable: %v", err)
	}
	if !strings.Contains(err.Error(), "want 8") {
		t.Fatalf("error does not say what was missing: %v", err)
	}
	// And nothing was cached, so the retry actually re-asks.
	if _, err := c.SlidePrompts(context.Background(), "vid-1"); err == nil {
		t.Fatal("a failed batch was cached")
	}
}

// Without a lookup the method cannot know what to illustrate, and no number of
// attempts changes that.
func TestSlidePromptsNeedsALookup(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(promptBatch(1, 1)))
	c := newClient(t, g, "")

	if _, err := c.SlidePrompts(context.Background(), "vid-1"); !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("SlidePrompts = %v, want ErrUnavailable", err)
	}
}

// The brand identity is the whole point of this prompt: if a rule stops
// reaching the model the slides drift off-channel and nothing fails.
func TestSlidePromptsPromptCarriesTheVisualIdentity(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(promptBatch(6, 3)))
	c := newSlideClient(t, g, videoContext(6, 3))

	if _, err := c.SlidePrompts(context.Background(), "vid-1"); err != nil {
		t.Fatalf("SlidePrompts: %v", err)
	}
	messages := g.messages(t)

	for _, want := range []string{
		"Channel Visual Identity",
		"RENDER STYLE",
		"BOLD AND FILLED",
		"#000000",
		"Three Image Roles",
		"ESTABLISHING",
		"DARK_MIDDLE",
		"minimalist 2D hand-drawn chalk illustration",
	} {
		if !strings.Contains(messages["system"], want) {
			t.Errorf("system prompt is missing %q", want)
		}
	}

	user := messages["user"]
	for _, want := range []string{
		"The Long Winter of the Harbour",
		"a northern port town over one winter",
		"18 image assets", // 6 chapters x 3
		"chapter 1, image 0 — ESTABLISHING, OPENING",
		"chapter 6, image 2 — RESONANCE, LANDING",
		"exactly 18 items",
	} {
		if !strings.Contains(user, want) {
			t.Errorf("user prompt is missing %q:\n%s", want, user)
		}
	}
}

// A video too short for an arc gets no macro position rather than a spurious
// one.
func TestShortVideosGetNoMacroPosition(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(promptBatch(3, 1)))
	c := newSlideClient(t, g, videoContext(3, 1))

	if _, err := c.SlidePrompts(context.Background(), "vid-1"); err != nil {
		t.Fatalf("SlidePrompts: %v", err)
	}
	user := g.messages(t)["user"]
	for _, absent := range []string{"OPENING", "DARK_MIDDLE", "LANDING"} {
		if strings.Contains(user, absent) {
			t.Errorf("a three-chapter video was given the macro position %q:\n%s", absent, user)
		}
	}
	// The per-chapter role still applies; it does not depend on length.
	if !strings.Contains(user, "ESTABLISHING") {
		t.Errorf("the image role was dropped along with the position:\n%s", user)
	}
}
