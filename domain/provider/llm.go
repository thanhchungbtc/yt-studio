package provider

import (
	"context"
	"encoding/json"

	"github.com/tbui/yt-studio/domain/entity"
)

// BlueprintRequest asks for the chapter outline of a whole video.
type BlueprintRequest struct {
	VideoID     entity.VideoID
	VideoRef    entity.Ref
	ChannelSlug entity.Slug
	Title       string
	Topic       string
	// ChapterCount is the number of chapters asked for. It is a target, not a
	// contract: the outline that comes back is what the video becomes.
	ChapterCount int
	// TargetDurationMinutes is how long the finished video should run. Zero
	// means unset, and the budget falls back to the default chapter size.
	TargetDurationMinutes int
	// Options is material the caller has that a backend may be able to use. Every
	// field is an offer, not an instruction: what a backend makes of one is its
	// own decision, and ignoring all of them is a complete implementation.
	Options BlueprintOptions
}

// BlueprintOptions are the offers that ride with a blueprint request.
type BlueprintOptions struct {
	// Blueprint is an outline prepared outside the app, in the document shape a
	// backend writes and stores. Nil is the ordinary case.
	//
	// Opaque above this port: callers carry the bytes from wherever the operator
	// put them to whichever backend is selected, and never read them. So one
	// backend may return such an outline as written and another treat it as a
	// draft to sharpen, without either being a use case's business.
	Blueprint json.RawMessage
}

// BlueprintChapter is one outlined chapter.
type BlueprintChapter struct {
	Ordinal int
	Title   string
	Summary string
	// EstimatedWords is this chapter's share of the video's spoken-word budget,
	// assigned by whoever planned it. Zero means unassigned.
	EstimatedWords int
}

// BlueprintOutline is the whole video plan one chapter is written inside: the
// writer sees every chapter in order, so it can build on what came before
// rather than re-deriving the same idea under a different title.
type BlueprintOutline struct {
	Title    string
	Summary  string
	Chapters []BlueprintChapter
}

// Chapter returns the outlined chapter at an ordinal.
func (o BlueprintOutline) Chapter(ordinal int) (BlueprintChapter, bool) {
	for _, c := range o.Chapters {
		if c.Ordinal == ordinal {
			return c, true
		}
	}
	return BlueprintChapter{}, false
}

// Blueprint is the outline plus the asset the JSON was written to.
type Blueprint struct {
	BlueprintOutline
	AssetID entity.AssetID
}

// ScriptRequest asks for one chapter's narration. It carries the blueprint
// context it needs so the provider never reads the database.
type ScriptRequest struct {
	VideoID   entity.VideoID
	ChapterID entity.ChapterID
	// Ordinal says which chapter of the outline to write; its title, brief and
	// budget are read out of Blueprint, so there is no second copy to disagree.
	Ordinal   int
	Blueprint BlueprintOutline
	// TargetWords is the resolved budget: what the blueprint assigned this
	// chapter, or the default when it assigned none.
	TargetWords int
	// Options is material the caller can offer, on the same terms as
	// BlueprintOptions: every field is an offer a backend may ignore.
	Options ScriptOptions
}

// ScriptOptions are the offers that ride with a script request.
type ScriptOptions struct {
	// PreparedScripts is narration written outside the app, keyed by the chapter
	// ordinals in ScriptRequest.Ordinal.
	//
	// The bytes are the whole prepared document — the same ones the blueprint
	// request was offered — because the two are pasted together and travel
	// together. The name is what this request may take from them, not an
	// inventory of what they hold; a backend declares the part it reads.
	//
	// Opaque above this port, like BlueprintOptions.Blueprint. Ordinals are the
	// renumbered ones: a chapter's position in the outline, counted from 1.
	PreparedScripts json.RawMessage
}

// Script is one chapter's narration plus the asset it was written to.
type Script struct {
	Text      string
	WordCount int
	AssetID   entity.AssetID
}

// SlidePrompt is one slide's prompt, addressed by chapter ordinal and the
// slide's index within that chapter.
type SlidePrompt struct {
	Ordinal int
	Index   int
	Prompt  string
}

// MetadataRequest asks for the YouTube-facing description of a finished video.
type MetadataRequest struct {
	VideoID  entity.VideoID
	VideoRef entity.Ref
	Title    string
	Topic    string
	Chapters []BlueprintChapter
}

// Metadata is the generated listing plus the asset the JSON was written to.
type Metadata struct {
	Metadata entity.Metadata
	AssetID  entity.AssetID
}

// ThumbnailPlanRequest asks for the grid that sits under the thumbnail's
// headline: which ideas from the video earn a tile, and what each tile shows.
type ThumbnailPlanRequest struct {
	VideoID   entity.VideoID
	VideoRef  entity.Ref
	Blueprint BlueprintOutline
	// Headline is the hook the metadata task wrote. The plan sees it so the
	// captions say something the headline does not already say.
	Headline string
	// Cells is exactly how many tiles to write, not a target: the DAG already
	// holds one icon task per cell, so a short plan leaves tasks with no prompt.
	Cells int
}

// ThumbnailPlan is the grid plus the asset the JSON was written to.
type ThumbnailPlan struct {
	Plan    entity.ThumbnailPlan
	AssetID entity.AssetID
}

// LLM covers every text generation step of the pipeline.
type LLM interface {
	Blueprint(ctx context.Context, req BlueprintRequest) (Blueprint, error)
	Script(ctx context.Context, req ScriptRequest) (Script, error)
	// SlidePrompts returns every chapter's prompts for one video. Callers are the
	// N per-chapter tasks; the implementation coalesces them behind singleflight
	// so exactly one real generation happens per video.
	SlidePrompts(ctx context.Context, videoID entity.VideoID) ([]SlidePrompt, error)
	Metadata(ctx context.Context, req MetadataRequest) (Metadata, error)
	ThumbnailPlan(ctx context.Context, req ThumbnailPlanRequest) (ThumbnailPlan, error)
}
