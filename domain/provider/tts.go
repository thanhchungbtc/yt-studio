package provider

import (
	"context"

	"github.com/tbui/yt-studio/domain/entity"
)

// SpeakRequest asks for the narration of exactly one chapter.
type SpeakRequest struct {
	VideoID   entity.VideoID
	ChapterID entity.ChapterID
	Ordinal   int
	Text      string

	ChapterTitle string

	// Voice, Language and Speed are how this chapter should sound. They cross the
	// port rather than being read by whichever backend is selected, because a
	// voice belongs to the channel the video is for: it is studio-wide today and
	// per-channel the day a second channel wants its own, and a backend reading
	// it from a settings row could never follow it there.
	//
	// Empty Voice is meaningful — it asks the server for its own default, which
	// beats this end guessing a filename it cannot verify. A backend that knows
	// nothing of voices ignores all three.
	Voice    string
	Language string
	Speed    float64
}

// TTS narrates one chapter per call.
type TTS interface {
	Speak(ctx context.Context, req SpeakRequest) (entity.AssetID, error)
}
