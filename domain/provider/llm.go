// Package provider declares the ports through which the server reaches
// generative backends and the store their output lands in.
//
// The rule every backend obeys: a provider call never spans more than one unit
// of work. No multi-chapter calls, no fan-out inside a provider. All
// orchestration — lifecycle, the cross-chapter DAG, resource pools, retries,
// persistence, gates — belongs to the server.
//
// The one deliberate exception is image prompting, where coalescing happens
// behind the interface: the DAG still holds N individually retryable
// per-chapter tasks and the provider serves them from one primed batch.
package provider

import (
	"context"
	"errors"

	"github.com/tbui/yt-studio/domain/entity"
)

// ErrUnavailable reports a backend that cannot run at all: a missing binary, a
// missing resource file, an unconfigured credential. It is worth its own
// sentinel because it is the one provider failure that retrying cannot fix —
// the operator has to change something first.
var ErrUnavailable = errors.New("backend unavailable")

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

// BlueprintOutline is the whole video plan one chapter is written inside.
//
// The writer sees every chapter in order, not just its own. That is what lets
// it build on ground an earlier chapter already covered and leave room for one
// still to come, instead of re-deriving the same idea under a different title
// forty minutes later.
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
	// Ordinal says which chapter of the outline to write. The chapter's own
	// title, brief and budget are read out of Blueprint rather than repeated
	// here, so there is no second copy to disagree with it.
	Ordinal   int
	Blueprint BlueprintOutline
	// TargetWords is the resolved budget: what the blueprint assigned this
	// chapter, or the default when it assigned none.
	TargetWords int
}

// Script is one chapter's narration plus the asset it was written to.
type Script struct {
	Text      string
	WordCount int
	AssetID   entity.AssetID
}

// ImagePrompt is one still's prompt, addressed by chapter ordinal and the
// still's index within that chapter.
type ImagePrompt struct {
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
	VideoID  entity.VideoID
	VideoRef entity.Ref
	Title    string
	Topic    string
	// Headline is the hook the metadata task wrote. The plan sees it so the
	// captions say something the headline does not already say.
	Headline string
	Chapters []BlueprintChapter
	// Cells is exactly how many tiles to write, not a target. The DAG already
	// holds one icon task per cell by the time this is called, so a plan that
	// comes back short leaves tasks with no prompt to read.
	Cells int
}

// ThumbnailPlan is the grid plus the asset the JSON was written to.
type ThumbnailPlan struct {
	Plan    entity.ThumbnailPlan
	AssetID entity.AssetID
}

// LLMProvider covers every text generation step of the pipeline.
type LLMProvider interface {
	Blueprint(ctx context.Context, req BlueprintRequest) (Blueprint, error)
	Script(ctx context.Context, req ScriptRequest) (Script, error)
	// ImagePrompts returns every chapter's prompts for one video. Callers are the
	// N per-chapter tasks; the implementation coalesces them behind singleflight
	// so exactly one real generation happens per video.
	ImagePrompts(ctx context.Context, videoID entity.VideoID) ([]ImagePrompt, error)
	Metadata(ctx context.Context, req MetadataRequest) (Metadata, error)
	ThumbnailPlan(ctx context.Context, req ThumbnailPlanRequest) (ThumbnailPlan, error)
}
