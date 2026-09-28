package app

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/tbui/yt-studio/domain/entity"
)

// YouTube turns a description into chapter links only when the timestamps in it
// satisfy all of these. Breaking one does not produce a partial set — it
// produces none — which is why the timeline is withheld entirely rather than
// offered malformed.
const (
	// minChaptersForTimeline is YouTube's floor. Fewer and it ignores the list.
	minChaptersForTimeline = 3
	// minChapterSeconds is the shortest chapter YouTube will accept.
	minChapterSeconds = 10
)

// DescriptionWithChapters returns the description with a chapter timeline
// appended, or the description unchanged when the video cannot carry one.
//
// Composed here rather than stored on the listing because the timeline is a
// property of the render: it moves when the cut is rebuilt, and a copy sitting
// in an editable field would go quietly wrong the first time it did. The cost
// is that the listing screen shows the prose alone while the published video
// has both.
//
// offsets are where each chapter begins in the finished cut, in seconds, as the
// concat recorded them. They are trusted only when there is exactly one per
// chapter: a shorter array is a timeline from a render of a different shape,
// and lining it up by index would point every line at the wrong place rather
// than at nothing.
func DescriptionWithChapters(
	description string,
	offsets []float64,
	chapters []entity.Chapter,
	maxRunes int,
) string {
	timeline := chapterTimeline(offsets, chapters)
	if timeline == "" {
		return clipRunes(description, maxRunes)
	}

	// The timeline is budgeted first and the prose is what gives way. The
	// uploader clips the whole description to the same ceiling, so appending
	// blindly would cut the list off mid-chapter on a long video — and a
	// truncated list is one YouTube reads as no list at all.
	const separator = "\n\n"
	room := maxRunes - utf8.RuneCountInString(timeline) - utf8.RuneCountInString(separator)
	if room <= 0 {
		// A timeline that cannot share the field with any prose is one nobody
		// asked for. The description is what the operator wrote.
		return clipRunes(description, maxRunes)
	}
	prose := strings.TrimRight(clipRunes(description, room), " \n\t")
	if prose == "" {
		return timeline
	}
	return prose + separator + timeline
}

// chapterTimeline renders the lines, or "" when this video may not have them.
func chapterTimeline(offsets []float64, chapters []entity.Chapter) string {
	if len(chapters) < minChaptersForTimeline || len(offsets) != len(chapters) {
		return ""
	}
	if offsets[0] != 0 {
		return ""
	}
	// Every gap between consecutive starts. The final chapter's length is the
	// one this cannot check — it needs the cut's total duration, which is not
	// on the video — so a last chapter under ten seconds would still lose the
	// whole list. Rare, and the outcome is what it was before this existed.
	for i := 1; i < len(offsets); i++ {
		if offsets[i]-offsets[i-1] < minChapterSeconds {
			return ""
		}
	}

	var b strings.Builder
	for i, c := range chapters {
		title := strings.Join(strings.Fields(c.Title), " ")
		if title == "" {
			// A line with a timestamp and nothing after it is not a chapter, and
			// YouTube reads the malformed line rather than skipping it.
			return ""
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(timestamp(offsets[i]))
		b.WriteByte(' ')
		b.WriteString(title)
	}
	return b.String()
}

// timestamp formats an offset the way YouTube reads one: m:ss under an hour and
// h:mm:ss past it, which is also what the upload screen's chapter list shows.
func timestamp(seconds float64) string {
	whole := int(seconds + 0.5)
	if whole < 0 {
		whole = 0
	}
	h, m, s := whole/3600, whole/60%60, whole%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// clipRunes truncates to a rune count, so a multi-byte description is measured
// the way YouTube measures it.
func clipRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	return string([]rune(s)[:maxRunes])
}
