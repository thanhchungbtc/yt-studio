package entity

import (
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

// AllVideoStates lists every VideoState, for validation, the UI and tests.
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

// Terminal reports whether no further work will happen for this video without
// operator action.
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

// Metadata is the YouTube-facing description of a finished video.
//
// It is stored as JSON on the video row, so a field added here needs no
// migration.
type Metadata struct {
	Title       string
	Description string
	Tags        []string
	// ThumbnailText is the all-caps hook overlaid on the thumbnail. It is
	// written with the rest of the listing because it competes for the same
	// glance the title does.
	ThumbnailText string
	CategoryID    string
	Privacy       string
}

// ThumbnailCell is one tile of the grid under the thumbnail's headline.
//
// The prompt is the subject only — "a lit alarm clock, side view". The style
// clause every icon shares is appended when the icon is generated, not stored
// here, so restyling the whole grid costs the icons rather than the words.
type ThumbnailCell struct {
	Caption string
	Prompt  string
}

// ThumbnailPlan is the grid the thumbnail is built from: what each tile says
// and what it pictures.
//
// It is stored as JSON on the video row, so a field added here needs no
// migration. Slice order is grid order — reading order, left to right.
type ThumbnailPlan struct {
	Cells []ThumbnailCell
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
	// ThumbnailCells is how many tiles the thumbnail's grid has. It is on the row
	// rather than read from settings at expansion because the DAG gets one icon
	// task per cell and can never grow after: a video's graph has to be
	// explainable from the video's own record.
	ThumbnailCells int
	// TargetDurationMinutes is how long the finished video should run. Zero
	// means unset, and the length is whatever ChapterCount chapters of the
	// channel's usual size come to.
	TargetDurationMinutes int

	BlueprintAssetID *AssetID
	FinalAssetID     *AssetID
	ThumbnailAssetID *AssetID
	// ThumbnailIconAssetIDs is one slot per cell, filled by the icon tasks as
	// they land. It is sized when the plan is written, so an out-of-order write
	// has a slot to go in rather than an array to grow.
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
	// The thumbnail's grid. The ceiling is what still reads at 1280x720 on a
	// phone: past two dozen tiles nobody can tell what any of them are.
	MinThumbnailCells = 1
	MaxThumbnailCells = 24
	// MaxDurationMinutes bounds a target length at twelve hours, which is well
	// past the longest thing this channel would publish.
	MaxDurationMinutes = 720
)

// The narration constants. They are what turn a word count into a duration and
// back, so a video cannot be planned or timed without them.
const (
	// DefaultWordsPerChapter is the spoken length of one chapter when nothing
	// has assigned it a budget of its own.
	DefaultWordsPerChapter = 450
	// DefaultWordsPerMinute is an unhurried narration speed, chosen for a
	// channel someone falls asleep to rather than for a briefing.
	DefaultWordsPerMinute = 130
)

// ChapterCountBand returns the inclusive range of chapter counts an accepted
// blueprint may have, for a video briefed with target chapters.
//
// A video's chapter count is a target, not a contract. The outline is written
// by a model, and a 50-chapter brief that comes back as 45 is a good blueprint
// that happened to find 45 natural breaks — the DAG is built from what the
// operator approves, so there is nothing for it to contradict. The band exists
// to separate that from a blueprint that came back with three chapters, which
// is a model failure and should stop the line.
//
// The slack is rounded up, so a small target still has room to move.
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
