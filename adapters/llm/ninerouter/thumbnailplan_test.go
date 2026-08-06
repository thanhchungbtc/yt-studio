package ninerouter_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

func thumbnailPlanRequest(chapters, cells int) provider.ThumbnailPlanRequest {
	req := provider.ThumbnailPlanRequest{
		VideoID:  "vid-1",
		VideoRef: "DSS-14",
		Blueprint: provider.BlueprintOutline{
			Title:   "The Long Winter of the Harbour",
			Summary: "a northern port town over one winter",
		},
		Headline: "50 FORGOTTEN HARBOURS",
		Cells:    cells,
	}
	for i := 1; i <= chapters; i++ {
		req.Blueprint.Chapters = append(req.Blueprint.Chapters, provider.BlueprintChapter{
			Ordinal: i,
			Title:   "Chapter " + strconv.Itoa(i),
			Summary: "What chapter " + strconv.Itoa(i) + " is about.",
		})
	}
	return req
}

func thumbnailPlanReply(cells ...[2]string) string {
	out := make([]map[string]string, 0, len(cells))
	for _, c := range cells {
		out = append(out, map[string]string{"caption": c[0], "icon": c[1]})
	}
	body, err := json.Marshal(map[string]any{"cells": out})
	if err != nil {
		panic(err)
	}
	return string(body)
}

func TestThumbnailPlanParsesTheGrid(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(thumbnailPlanReply(
		[2]string{"1707", "a shipwrecked hull on the seabed"},
		[2]string{"Salt", "a merchant ship under sail"},
		[2]string{"Ice", "a frozen mooring rope"},
	)))
	c := newClient(t, g, "")

	plan, err := c.ThumbnailPlan(context.Background(), thumbnailPlanRequest(6, 3))
	if err != nil {
		t.Fatalf("ThumbnailPlan: %v", err)
	}
	want := []entity.ThumbnailCell{
		{Caption: "1707", Prompt: "a shipwrecked hull on the seabed"},
		{Caption: "Salt", Prompt: "a merchant ship under sail"},
		{Caption: "Ice", Prompt: "a frozen mooring rope"},
	}
	if len(plan.Plan.Cells) != len(want) {
		t.Fatalf("got %d cells, want %d", len(plan.Plan.Cells), len(want))
	}
	for i, cell := range plan.Plan.Cells {
		if cell != want[i] {
			t.Errorf("cell %d = %+v, want %+v", i, cell, want[i])
		}
	}
	if plan.AssetID == "" {
		t.Error("the plan was not stored")
	}
}

func TestThumbnailPlanPromptCarriesTheVideo(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(thumbnailPlanReply(
		[2]string{"Salt", "a merchant ship under sail"})))
	c := newClient(t, g, "")

	if _, err := c.ThumbnailPlan(context.Background(), thumbnailPlanRequest(4, 12)); err != nil {
		t.Fatalf("ThumbnailPlan: %v", err)
	}
	messages := g.messages(t)
	for _, want := range []string{"12", `"cells"`, "caption"} {
		if !strings.Contains(messages["system"], want) {
			t.Errorf("system prompt does not carry %q", want)
		}
	}
	for _, want := range []string{
		"The Long Winter of the Harbour",
		"50 FORGOTTEN HARBOURS",
		"Chapter 4",
		"12",
	} {
		if !strings.Contains(messages["user"], want) {
			t.Errorf("user prompt does not carry %q", want)
		}
	}
}

// The whitespace a model puts in a caption is the adapter's to flatten; the
// count and any empty cell belong to app.normaliseCells.
func TestThumbnailPlanFlattensWhitespace(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion(thumbnailPlanReply(
		[2]string{"  Salt\n Roads ", "  a merchant ship under sail\n"})))
	c := newClient(t, g, "")

	plan, err := c.ThumbnailPlan(context.Background(), thumbnailPlanRequest(4, 1))
	if err != nil {
		t.Fatalf("ThumbnailPlan: %v", err)
	}
	cell := plan.Plan.Cells[0]
	if cell.Caption != "Salt Roads" {
		t.Errorf("caption = %q", cell.Caption)
	}
	if cell.Prompt != "a merchant ship under sail" {
		t.Errorf("prompt = %q", cell.Prompt)
	}
}

func TestThumbnailPlanRejectsProse(t *testing.T) {
	t.Parallel()
	g := newGateway(t, http.StatusOK, completion("Here is the grid you asked for."))
	c := newClient(t, g, "")

	if _, err := c.ThumbnailPlan(context.Background(), thumbnailPlanRequest(4, 3)); err == nil {
		t.Fatal("expected an error")
	} else if !strings.Contains(err.Error(), "not JSON") {
		t.Errorf("error = %v", err)
	}
}

func TestThumbnailPlanClassifiesGatewayFailures(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		status    int
		retryable bool
	}{
		"rejected key": {status: http.StatusUnauthorized},
		"rate limited": {status: http.StatusTooManyRequests, retryable: true},
		"outage":       {status: http.StatusBadGateway, retryable: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			g := newGateway(t, tc.status, `{"error":{"message":"nope"}}`)
			c := newClient(t, g, "")

			_, err := c.ThumbnailPlan(context.Background(), thumbnailPlanRequest(4, 3))
			if err == nil {
				t.Fatal("expected an error")
			}
			if retryable := !errors.Is(err, provider.ErrUnavailable); retryable != tc.retryable {
				t.Errorf("retryable = %v, want %v (%v)", retryable, tc.retryable, err)
			}
		})
	}
}
