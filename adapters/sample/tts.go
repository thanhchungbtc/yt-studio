package sample

import (
	"context"
	"fmt"
	"os"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// TTS narrates every chapter with the same recording.
//
// A real backend would speak the chapter's own script; this one hands back the
// sample unchanged, which means every chapter shares one content address and
// therefore one row and one file. That is a deliberate trade: the point is real
// speech in the composed video, and nothing in the system keys off narration
// being unique — the chapter's own audio_asset_id is written per chapter, and
// the asset table's chapter_id is provenance only.
type TTS struct {
	lib   *Library
	store provider.AssetStore
}

var _ provider.TTSProvider = (*TTS)(nil)

// NewTTS wires the backend to the shared library.
func NewTTS(lib *Library, store provider.AssetStore) *TTS {
	return &TTS{lib: lib, store: store}
}

// Speak stores the sample narration and returns its content address.
//
// The file is streamed rather than read: it is megabytes per call, and the
// store hashes and copies with a pooled buffer, so memory stays flat however
// long the recording is.
func (t *TTS) Speak(ctx context.Context, _ provider.SpeakRequest) (entity.AssetID, error) {
	if err := t.lib.Check(); err != nil {
		return "", err
	}
	file, err := os.Open(t.lib.audio) //nolint:gosec // path comes from the resources directory
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrUnavailable, t.lib.audio, err)
	}
	defer func() { _ = file.Close() }()

	stored, err := t.store.Put(ctx, entity.AssetKindAudio, file)
	if err != nil {
		return "", fmt.Errorf("store narration: %w", err)
	}
	return stored.ID, nil
}
