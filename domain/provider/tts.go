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
}

// TTS narrates one chapter per call.
type TTS interface {
	Speak(ctx context.Context, req SpeakRequest) (entity.AssetID, error)
}
