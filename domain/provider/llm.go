// Package provider declares the five ports through which the daemon reaches
// generative backends and the store their output lands in.
//
// The rule every backend obeys: a provider call never spans more than one unit
// of work. No multi-chapter calls, no fan-out inside a provider. All
// orchestration — lifecycle, the cross-chapter DAG, resource pools, retries,
// persistence, gates — belongs to the daemon.
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
	VideoID      entity.VideoID
	VideoRef     entity.Ref
	ChannelSlug  entity.Slug
	Title        string
	Topic        string
	ChapterCount int
	Style        entity.StyleConfig
}

// BlueprintChapter is one outlined chapter.
type BlueprintChapter struct {
	Ordinal int
	Title   string
	Summary string
}

// Blueprint is the outline plus the asset the JSON was written to.
type Blueprint struct {
	Title    string
	Summary  string
	Chapters []BlueprintChapter
	AssetID  entity.AssetID
}

// ScriptRequest asks for one chapter's narration. It carries the blueprint
// context it needs so the provider never reads the database.
type ScriptRequest struct {
	VideoID          entity.VideoID
	ChapterID        entity.ChapterID
	Ordinal          int
	ChapterTitle     string
	ChapterSummary   string
	BlueprintTitle   string
	BlueprintSummary string
	Style            entity.StyleConfig
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
	Style    entity.StyleConfig
}

// Metadata is the generated listing plus the asset the JSON was written to.
type Metadata struct {
	Metadata entity.Metadata
	AssetID  entity.AssetID
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
}
