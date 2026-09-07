package sample

import (
	"context"
	"fmt"
	"os"

	"github.com/tbui/yt-studio/adapters/provider/tts"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// TTS narrates every chapter with the same recording, so a whole video costs
// one row and one file. Nothing keys off narration being unique: the chapter's
// own audio_asset_id is written per chapter, and the asset table's chapter_id
// is provenance only.
type TTS struct {
	lib   *Library
	store provider.AssetStore
}

var _ provider.TTS = (*TTS)(nil)

// NewTTS wires the backend to the shared library.
func NewTTS(lib *Library, store provider.AssetStore) *TTS {
	return &TTS{lib: lib, store: store}
}

// Speak stores the sample narration and returns its content address and length.
// Streamed rather than read, so memory stays flat however long the recording is
// — which is why the duration comes from the header rather than from decoding.
func (t *TTS) Speak(ctx context.Context, _ provider.SpeakRequest) (provider.Narration, error) {
	if err := t.lib.Check(); err != nil {
		return provider.Narration{}, err
	}
	file, err := os.Open(t.lib.audio) //nolint:gosec // path comes from the resources directory
	if err != nil {
		return provider.Narration{}, fmt.Errorf("%w: %s: %w", ErrUnavailable, t.lib.audio, err)
	}
	defer func() { _ = file.Close() }()

	// Before the store reads it, and DurationOf rewinds to where it started, so
	// what gets written is still the whole file.
	seconds := tts.DurationOf(file)

	stored, err := t.store.Put(ctx, entity.AssetKindAudio, file)
	if err != nil {
		return provider.Narration{}, fmt.Errorf("store narration: %w", err)
	}
	return provider.Narration{AssetID: stored.ID, Seconds: seconds}, nil
}
