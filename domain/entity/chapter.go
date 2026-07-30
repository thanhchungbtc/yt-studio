package entity

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrInvalidChapter is returned by the Chapter constructor for invalid input.
var ErrInvalidChapter = errors.New("invalid chapter")

// Chapter owns its ordinal, title, script, audio, images and composed clip.
type Chapter struct {
	ID      ChapterID
	VideoID VideoID
	// Ordinal is 1-based and unique within the video; it is the chapter's natural
	// key together with the video ref (DSS-14#7).
	Ordinal int
	Title   string
	Summary string
	Script  string

	ImagePrompts  []string
	AudioAssetID  *AssetID
	ImageAssetIDs []AssetID
	ClipAssetID   *AssetID

	DurationSeconds float64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// NewChapter validates and constructs a Chapter.
func NewChapter(videoID VideoID, ordinal int, title, summary string, now time.Time) (Chapter, error) {
	if strings.TrimSpace(string(videoID)) == "" {
		return Chapter{}, fmt.Errorf("%w: video id must not be empty", ErrInvalidChapter)
	}
	if ordinal < 1 {
		return Chapter{}, fmt.Errorf("%w: ordinal must be >= 1, got %d", ErrInvalidChapter, ordinal)
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return Chapter{}, fmt.Errorf("%w: title must not be empty", ErrInvalidChapter)
	}
	return Chapter{
		ID:        NewChapterID(videoID, ordinal),
		VideoID:   videoID,
		Ordinal:   ordinal,
		Title:     title,
		Summary:   strings.TrimSpace(summary),
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// NaturalKey returns the human-readable key of a chapter within its video, e.g.
// `DSS-14#7`. It is used in logs and golden-file fixtures.
func (c Chapter) NaturalKey(videoRef Ref) string {
	var b strings.Builder
	b.Grow(len(videoRef) + 4)
	b.WriteString(string(videoRef))
	b.WriteByte('#')
	b.WriteString(strconv.Itoa(c.Ordinal))
	return b.String()
}
