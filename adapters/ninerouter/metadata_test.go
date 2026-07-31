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
			Ordinal: i,
			Title:   "Chapter " + strconv.Itoa(i),
			Summary: "What chapter " + strconv.Itoa(i) + " is about.",
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
		"Fifty Ideas That Break Your Sense of Self | To Fall Asleep Faster",
		"In this session, we walk through the harbour in winter…",
		"philosophy, psychology, sleep philosophy, to fall asleep faster",
		"50 BROKEN BELIEFS")))
	c := newClient(t, g, "")

	md, err := c.Metadata(context.Background(), metadataRequest(3))
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if !strings.HasSuffix(md.Metadata.Title, "| To Fall Asleep Faster") {
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
		"| To Fall Asleep Faster",
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
