package entity

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalidVideo is returned by the Video constructor for invalid input.
var ErrInvalidVideo = errors.New("invalid video")

// ErrInvalidTransition is returned when a lifecycle transition is not allowed.
var ErrInvalidTransition = errors.New("invalid state transition")

// VideoState is the lifecycle state of a Video.
type VideoState string

// The complete set of video lifecycle states.
const (
	// VideoStateDraft is a video that exists but whose DAG has not been enqueued.
	VideoStateDraft VideoState = "draft"
	// VideoStateRunning has at least one task in flight or ready.
	VideoStateRunning VideoState = "running"
	// VideoStateAwaitingApproval is parked on a gate and consumes no resources.
	VideoStateAwaitingApproval VideoState = "awaiting_approval"
	// VideoStateBlocked has no runnable task and at least one permanently failed
	// one.
	VideoStateBlocked VideoState = "blocked"
	// VideoStateCompleted finished the whole DAG, upload included.
	VideoStateCompleted VideoState = "completed"
	// VideoStateFailed exhausted retries on a task with no path forward.
	VideoStateFailed VideoState = "failed"
	// VideoStateCancelled was stopped by the operator.
	VideoStateCancelled VideoState = "cancelled"
)

// AllVideoStates lists every VideoState, for validation and the UI.
var AllVideoStates = []VideoState{
	VideoStateDraft,
	VideoStateRunning,
	VideoStateAwaitingApproval,
	VideoStateBlocked,
	VideoStateCompleted,
	VideoStateFailed,
	VideoStateCancelled,
}

// Valid reports whether the state is one of the known constants.
func (s VideoState) Valid() bool {
	switch s {
	case VideoStateDraft, VideoStateRunning, VideoStateAwaitingApproval,
		VideoStateBlocked, VideoStateCompleted, VideoStateFailed, VideoStateCancelled:
		return true
	default:
		return false
	}
}

// Terminal reports whether no further work happens without operator action.
func (s VideoState) Terminal() bool {
	switch s {
	case VideoStateCompleted, VideoStateFailed, VideoStateCancelled:
		return true
	case VideoStateDraft, VideoStateRunning, VideoStateAwaitingApproval, VideoStateBlocked:
		return false
	default:
		return false
	}
}

// videoTransitions is the whole lifecycle state machine, in one place.
var videoTransitions = map[VideoState][]VideoState{
	VideoStateDraft:            {VideoStateRunning, VideoStateCancelled},
	VideoStateRunning:          {VideoStateAwaitingApproval, VideoStateBlocked, VideoStateCompleted, VideoStateFailed, VideoStateCancelled},
	VideoStateAwaitingApproval: {VideoStateRunning, VideoStateCancelled, VideoStateFailed},
	VideoStateBlocked:          {VideoStateRunning, VideoStateCancelled, VideoStateFailed},
	VideoStateCompleted:        {},
	VideoStateFailed:           {VideoStateRunning, VideoStateCancelled},
	VideoStateCancelled:        {VideoStateRunning},
}

// CanTransitionTo reports whether from -> to is a legal lifecycle move.
func (s VideoState) CanTransitionTo(to VideoState) bool {
	if s == to {
		return true
	}
	for _, allowed := range videoTransitions[s] {
		if allowed == to {
			return true
		}
	}
	return false
}

// Metadata is the YouTube-facing description of a finished video, stored as
// JSON on the video row so a new field needs no migration.
type Metadata struct {
	Title       string
	Description string
	Tags        []string
	// ThumbnailText is the all-caps hook overlaid on the thumbnail, written with
	// the listing because it competes for the same glance the title does.
	ThumbnailText string
	CategoryID    string
	Privacy       string
}

// ThumbnailCell is one tile of the grid under the headline. Prompt is the
// subject only — the shared style clause is appended at generation time.
type ThumbnailCell struct {
	Caption string
	Prompt  string
}

// ThumbnailPlan is the grid the thumbnail is built from, stored as JSON on the
// video row. Slice order is reading order, left to right.
type ThumbnailPlan struct {
	Cells []ThumbnailCell
}

// The frame a thumbnail is published at, and the ceiling on the file. These
// are YouTube's numbers rather than a layout choice, which is why they are here
// and not among the renderer's style constants: an image outside them is
// refused at upload whatever drew it.
const (
	ThumbnailWidth    = 1280
	ThumbnailHeight   = 720
	MaxThumbnailBytes = 2 << 20
)

// MaxThumbnailDesignBytes bounds the editor document. Nothing on this side
// parses it, so this is the only thing standing between a runaway browser blob
// and a row that ships in every video response.
const MaxThumbnailDesignBytes = 256 << 10

// ThumbnailDesign is the browser thumbnail editor's document: element
// positions, text and colours as the editor wrote them.
//
// Deliberately opaque. The browser both authors this and is the only thing that
// can render it -- the image it produces is uploaded finished -- so a struct
// here would be a second definition of the same shape, kept in step for no
// reader. Validation asks only what storing it requires.
type ThumbnailDesign json.RawMessage

// MarshalJSON passes the document through verbatim. Without this a []byte would
// reach the API base64-encoded, and the editor would have to decode its own
// document back out of its own video.
func (d ThumbnailDesign) MarshalJSON() ([]byte, error) {
	if len(d) == 0 {
		return []byte("null"), nil
	}
	return d, nil
}

// UnmarshalJSON keeps the raw bytes rather than interpreting them.
func (d *ThumbnailDesign) UnmarshalJSON(b []byte) error {
	*d = append((*d)[:0], b...)
	return nil
}

// Validate reports whether the design can be stored: well-formed JSON, and
// bounded. An empty design is valid and means the editor was never opened.
func (d ThumbnailDesign) Validate() error {
	if len(d) == 0 {
		return nil
	}
	if len(d) > MaxThumbnailDesignBytes {
		return fmt.Errorf("%w: thumbnail design is %d bytes, over the %d limit",
			ErrInvalidVideo, len(d), MaxThumbnailDesignBytes)
	}
	if !json.Valid(d) {
		return fmt.Errorf("%w: thumbnail design is not valid JSON", ErrInvalidVideo)
	}
	return nil
}

// UploadRecord is the durable receipt of an upload attempt.
type UploadRecord struct {
	VideoID    string
	URL        string
	DryRun     bool
	UploadedAt time.Time
}

// Video owns lifecycle state, the blueprint, the final artifact and the upload
// record.
type Video struct {
	ID        VideoID
	ChannelID ChannelID
	// Ref is the stable human-readable natural key, e.g. DSS-14.
	Ref   Ref
	Title string
	Topic string
	State VideoState

	ChapterCount     int
	SlidesPerChapter int
	// ThumbnailCells is how many tiles the grid has. On the row rather than read
	// from settings, so a video's graph is explainable from its own record.
	ThumbnailCells int
	// TargetDurationMinutes is how long the finished video should run; zero
	// means unset.
	TargetDurationMinutes int

	BlueprintAssetID *AssetID
	FinalAssetID     *AssetID
	ThumbnailAssetID *AssetID
	// ThumbnailOverrideAssetID is a thumbnail the operator built by hand, which
	// wins over the rendered one. Held apart from ThumbnailAssetID so that
	// re-running the thumbnail task -- which redrawing any single icon does --
	// cannot discard it. Nil means the rendered thumbnail publishes.
	ThumbnailOverrideAssetID *AssetID
	// ThumbnailDesign is the browser editor's document, opaque to the server.
	ThumbnailDesign ThumbnailDesign
	// ThumbnailIconAssetIDs is one slot per cell, sized when the plan is written
	// so an out-of-order icon has a slot to land in rather than an array to grow.
	ThumbnailIconAssetIDs []AssetID
	ThumbnailPlan         *ThumbnailPlan
	Metadata              *Metadata
	Upload                *UploadRecord

	Error       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
}

// EffectiveThumbnailAssetID is the image that fronts the published video: the
// one the operator built if there is one, otherwise what the renderer produced.
// Every caller asks here, so the upload, the gate and the screen cannot come to
// different answers about which of the two is live. Empty means neither exists.
func (v Video) EffectiveThumbnailAssetID() AssetID {
	if v.ThumbnailOverrideAssetID != nil && *v.ThumbnailOverrideAssetID != "" {
		return *v.ThumbnailOverrideAssetID
	}
	if v.ThumbnailAssetID != nil {
		return *v.ThumbnailAssetID
	}
	return ""
}

// NewVideo validates and constructs a Video in the draft state.
//
//nolint:revive // the parameter list is the video's shape
func NewVideo(id VideoID, channelID ChannelID, ref Ref, title, topic string, chapterCount, slidesPerChapter, thumbnailCells, targetDurationMinutes int, now time.Time) (Video, error) {
	if strings.TrimSpace(string(id)) == "" {
		return Video{}, fmt.Errorf("%w: id must not be empty", ErrInvalidVideo)
	}
	if strings.TrimSpace(string(channelID)) == "" {
		return Video{}, fmt.Errorf("%w: channel id must not be empty", ErrInvalidVideo)
	}
	if _, _, err := ParseRef(string(ref)); err != nil {
		return Video{}, fmt.Errorf("%w: %w", ErrInvalidVideo, err)
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return Video{}, fmt.Errorf("%w: title must not be empty", ErrInvalidVideo)
	}
	if chapterCount < MinChapterCount || chapterCount > MaxChapterCount {
		return Video{}, fmt.Errorf("%w: chapter count must be %d..%d, got %d",
			ErrInvalidVideo, MinChapterCount, MaxChapterCount, chapterCount)
	}
	if slidesPerChapter < MinSlidesPerChapter || slidesPerChapter > MaxSlidesPerChapter {
		return Video{}, fmt.Errorf("%w: slides per chapter must be %d..%d, got %d",
			ErrInvalidVideo, MinSlidesPerChapter, MaxSlidesPerChapter, slidesPerChapter)
	}
	if thumbnailCells < MinThumbnailCells || thumbnailCells > MaxThumbnailCells {
		return Video{}, fmt.Errorf("%w: thumbnail cells must be %d..%d, got %d",
			ErrInvalidVideo, MinThumbnailCells, MaxThumbnailCells, thumbnailCells)
	}
	if targetDurationMinutes < 0 || targetDurationMinutes > MaxDurationMinutes {
		return Video{}, fmt.Errorf("%w: target duration must be 0..%d minutes, got %d",
			ErrInvalidVideo, MaxDurationMinutes, targetDurationMinutes)
	}
	return Video{
		ID:                    id,
		ChannelID:             channelID,
		Ref:                   ref,
		Title:                 title,
		Topic:                 strings.TrimSpace(topic),
		State:                 VideoStateDraft,
		ChapterCount:          chapterCount,
		SlidesPerChapter:      slidesPerChapter,
		ThumbnailCells:        thumbnailCells,
		TargetDurationMinutes: targetDurationMinutes,
		CreatedAt:             now,
		UpdatedAt:             now,
	}, nil
}

// Bounds on video shape, enforced by the constructor.
const (
	MinChapterCount     = 1
	MaxChapterCount     = 500
	MinSlidesPerChapter = 1
	MaxSlidesPerChapter = 20
	// The ceiling is what still reads at 1280x720 on a phone.
	MinThumbnailCells = 1
	MaxThumbnailCells = 24
	// MaxDurationMinutes bounds a target length at twelve hours.
	MaxDurationMinutes = 720
)

// The narration constants, which turn a word count into a duration and back.
const (
	// DefaultWordsPerChapter is one chapter's spoken length absent a budget.
	DefaultWordsPerChapter = 450
	// DefaultWordsPerMinute is an unhurried narration speed, for a channel
	// someone falls asleep to rather than a briefing.
	DefaultWordsPerMinute = 130
)

// ChapterCountBand is the inclusive range an accepted blueprint may land in.
// A chapter count is a target, not a contract: 45 against a brief of 50 is a
// good outline, three is a model failure that should stop the line. Slack is
// rounded up so a small target still has room to move.
func ChapterCountBand(target, tolerancePercent int) (minCount, maxCount int) {
	if tolerancePercent < 0 {
		tolerancePercent = 0
	}
	slack := (target*tolerancePercent + 99) / 100
	minCount = max(target-slack, MinChapterCount)
	maxCount = min(target+slack, MaxChapterCount)
	if maxCount < minCount {
		maxCount = minCount
	}
	return minCount, maxCount
}
