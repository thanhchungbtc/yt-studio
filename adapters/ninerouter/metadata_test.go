package ninerouter_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/tbui/yt-studio/domain/provider"
)

func metadataRequest(chapters int) provider.MetadataRequest {
	req := provider.MetadataRequest{
		VideoID:  "vid-1",
		VideoRef: "DSS-14",
		Title:    "The Long Winter of the Harbour",
		Topic:    "a northern port town over one winter",
	}
	for i := 1; i <= chapters; i++ {
		req.Chapters = append(req.Chapters, provider.BlueprintChapter{
			Ordinal:        i,
			Title:          "Chapter " + strconv.Itoa(i),
			Summary:        "What chapter " + strconv.Itoa(i) + " is about.",
			EstimatedWords: 450,
		})
	}
	return req
}

func metadataReply(title, description, tags, thumbnail string) string {
	body, err := json.Marshal(map[string]string{
		"title": title, "description": description,
		"tags": tags, "thumbnail_text": thumbnail,
	})
	if err != nil {
		panic(err)
	}
	return string(body)
}

func TestMetadataParsesTheListing(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(metadataReply(
		"50 Ideas That Break Your Sense of Self To Fall Asleep To",
		"In this session, we walk through the harbour in winter…",
		"philosophy, psychology, sleep philosophy, to fall asleep faster",
		"50 BROKEN BELIEFS")))
	c := newClient(t, g, "")

	md, err := c.Metadata(context.Background(), metadataRequest(3))
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if !strings.HasSuffix(md.Metadata.Title, "To Fall Asleep To") {
		t.Errorf("title = %q", md.Metadata.Title)
	}
	if md.Metadata.ThumbnailText != "50 BROKEN BELIEFS" {
		t.Errorf("thumbnail = %q", md.Metadata.ThumbnailText)
	}
	if md.AssetID == "" {
		t.Error("the listing was not stored")
	}
}

// The prompt asks for one comma-separated string because models honour that
// more reliably than a nested list; the domain carries a list.
func TestMetadataSplitsTheKeywordString(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(metadataReply("T", "D",
		"  philosophy , psychology,, Philosophy ,sleep philosophy,  ", "HOOK")))
	c := newClient(t, g, "")

	md, err := c.Metadata(context.Background(), metadataRequest(2))
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	want := []string{"philosophy", "psychology", "sleep philosophy"}
	if len(md.Metadata.Tags) != len(want) {
		t.Fatalf("tags = %#v, want %#v", md.Metadata.Tags, want)
	}
	for i, tag := range want {
		if md.Metadata.Tags[i] != tag {
			t.Errorf("tag %d = %q, want %q", i, md.Metadata.Tags[i], tag)
		}
	}
}

// A YouTube category is an API enum and an unpublished listing stays private —
// neither is the model's to choose.
func TestMetadataSetsTheUploadFieldsItself(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(metadataReply("T", "D", "a,b", "HOOK")))
	c := newClient(t, g, "")

	md, err := c.Metadata(context.Background(), metadataRequest(1))
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if md.Metadata.CategoryID != "24" || md.Metadata.Privacy != "private" {
		t.Fatalf("category/privacy = %q/%q, want 24 (Entertainment)/private",
			md.Metadata.CategoryID, md.Metadata.Privacy)
	}
}

func TestMetadataFallsBackToTheWorkingTitle(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(metadataReply("   ", "D", "a", "HOOK")))
	c := newClient(t, g, "")

	md, err := c.Metadata(context.Background(), metadataRequest(1))
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if md.Metadata.Title != "The Long Winter of the Harbour" {
		t.Fatalf("title = %q, want the working title", md.Metadata.Title)
	}
}

// The chapter titles are what the video actually covers, and where the niche
// keywords and the hook have to come from.
func TestMetadataPromptCarriesTheChapters(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(metadataReply("T", "D", "a", "HOOK")))
	c := newClient(t, g, "")

	if _, err := c.Metadata(context.Background(), metadataRequest(4)); err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	messages := g.messages(t)

	for _, want := range []string{
		"The Long Winter of the Harbour",
		"a northern port town over one winter",
		"Chapters: 4",
		"1. Chapter 1",
		"4. Chapter 4",
	} {
		if !strings.Contains(messages["user"], want) {
			t.Errorf("user prompt is missing %q:\n%s", want, messages["user"])
		}
	}
	for _, want := range []string{
		"YouTube SEO",
		"To Fall Asleep To",
		"DURATION-LED",
		"COUNT-LED",
		"CURIOSITY-LED",
		"3+ Hours of Game Theory Explained",
		"150–300 words",
		"10–15 keywords",
		"ALL-CAPS text hook",
		"NUMBERED LIST",
		"Never hype or sensationalize",
	} {
		if !strings.Contains(messages["system"], want) {
			t.Errorf("system prompt is missing %q", want)
		}
	}
}

func TestMetadataRejectsAnUnreadableResponse(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion("Sure! Here is your metadata:"))
	_, err := newClient(t, g, "").Metadata(context.Background(), metadataRequest(2))
	if err == nil {
		t.Fatal("expected an error")
	}
	// A bad roll is worth another attempt, not a permanent failure.
	if errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("a bad roll was reported as unavailable: %v", err)
	}
	if !strings.Contains(err.Error(), "Sure! Here is your metadata:") {
		t.Fatalf("error does not show the response: %v", err)
	}
}

// A duration-led title promises a running time, and the model cannot know one.
// It is computed from what the blueprint budgeted so the promise is true.
func TestMetadataPromptCarriesTheRunningTime(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		chapters int
		words    int
		want     string
	}{
		// 60 chapters x 450 words = 27000 words, 207 minutes at 130 wpm.
		{"three and a half hours", 60, 450, "3+ Hours"},
		// 52 x 450 = 23400 words, exactly 180 minutes.
		{"a round three hours", 52, 450, "3 Hours"},
		{"one hour", 18, 450, "1 Hour"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := newGateway(t, http.StatusOK, completion(metadataReply("T", "D", "a", "HOOK")))
			req := metadataRequest(tc.chapters)
			for i := range req.Chapters {
				req.Chapters[i].EstimatedWords = tc.words
			}
			if _, err := newClient(t, g, "").Metadata(context.Background(), req); err != nil {
				t.Fatalf("Metadata: %v", err)
			}
			user := g.messages(t)["user"]
			if !strings.Contains(user, "Running time: "+tc.want) {
				t.Errorf("running time is not %q:\n%s", tc.want, user)
			}
			if !strings.Contains(user, `uses "`+tc.want+`" exactly`) {
				t.Errorf("the prompt did not pin the duration to %q:\n%s", tc.want, user)
			}
		})
	}
}

// A title promising "1 Hour" of a seven-minute test video is worse than one
// that never mentions length, so a short video is told not to reach for the
// duration-led shape at all.
func TestShortVideosGetNoRunningTime(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(metadataReply("T", "D", "a", "HOOK")))
	c := newClient(t, g, "")

	if _, err := c.Metadata(context.Background(), metadataRequest(2)); err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	user := g.messages(t)["user"]
	if strings.Contains(user, "Running time:") {
		t.Errorf("a two-chapter video was given a running time:\n%s", user)
	}
	if !strings.Contains(user, "do not use a duration-led") {
		t.Errorf("the prompt did not rule out the duration-led shape:\n%s", user)
	}
	// The count-led shape still applies, and it has to name the real count.
	if !strings.Contains(user, "count-led title\nleads with 2") &&
		!strings.Contains(user, "leads with 2") {
		t.Errorf("the prompt did not offer the real chapter count:\n%s", user)
	}
}
