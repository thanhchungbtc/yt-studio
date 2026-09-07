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

	Voice    string
	Language string
	Speed    float64
}

// Narration is what a backend produced: where the audio is, and how long it
// runs.
//
// The length is measured from the bytes that were stored, and it is returned
// here rather than probed later because this is the only moment the audio is in
// hand. A caller reading it back off disk would be opening a file to recover a
// number the backend already had.
//
// Seconds is 0 when the backend could not read a duration out of what it made.
// That is a missing measurement and not a failure: the audio is stored either
// way, and every reader already treats 0 as "not measured".
type Narration struct {
	AssetID entity.AssetID
	Seconds float64
}

// TTS narrates one chapter per call.
type TTS interface {
	Speak(ctx context.Context, req SpeakRequest) (Narration, error)
}
