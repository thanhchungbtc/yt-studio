// Package entity holds the dependency-free core domain model of yt-studio.
//
// Nothing in this package may import another yt-studio package. Identifiers are
// distinct named types so that passing a ChapterID where a VideoID belongs is a
// compile error, not a runtime surprise (§8.2).
package entity

import (
	"strconv"
	"strings"
)

// ChannelID is the opaque identifier of a Channel.
type ChannelID string

// VideoID is the opaque identifier of a Video.
type VideoID string

// ChapterID is the opaque identifier of a Chapter.
type ChapterID string

// AssetID is the content address (sha256, hex) of an Asset. Two identical byte
// streams always produce the same AssetID, which is what makes re-running a
// task that produces identical output a no-op (§3).
type AssetID string

// TaskID is the deterministic identifier of a Task. It is derived from the
// video, kind, ordinal and index rather than randomly generated, so enqueueing
// the same DAG twice is an idempotent upsert and golden-file tests stay stable.
type TaskID string

// String returns the underlying text of the identifier.
func (id ChannelID) String() string { return string(id) }

// String returns the underlying text of the identifier.
func (id VideoID) String() string { return string(id) }

// String returns the underlying text of the identifier.
func (id ChapterID) String() string { return string(id) }

// String returns the underlying text of the identifier.
func (id AssetID) String() string { return string(id) }

// String returns the underlying text of the identifier.
func (id TaskID) String() string { return string(id) }

// Short returns the first 12 characters of a content address, for log lines.
func (id AssetID) Short() string {
	if len(id) <= 12 {
		return string(id)
	}
	return string(id[:12])
}

// NewChapterID derives the deterministic chapter identifier for an ordinal.
func NewChapterID(videoID VideoID, ordinal int) ChapterID {
	var b strings.Builder
	b.Grow(len(videoID) + 8)
	b.WriteString(string(videoID))
	b.WriteString(":ch:")
	b.WriteString(strconv.Itoa(ordinal))
	return ChapterID(b.String())
}

// NewTaskID derives the deterministic task identifier for a node in the DAG.
//
// ordinal is the chapter ordinal (or -1 for video-level tasks) and index
// distinguishes sibling tasks for the same chapter, such as the two images of a
// chapter (or -1 when there is only one).
func NewTaskID(videoID VideoID, kind TaskKind, ordinal, index int) TaskID {
	var b strings.Builder
	b.Grow(len(videoID) + len(kind) + 12)
	b.WriteString(string(videoID))
	b.WriteByte(':')
	b.WriteString(string(kind))
	if ordinal >= 0 {
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(ordinal))
	}
	if index >= 0 {
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(index))
	}
	return TaskID(b.String())
}
