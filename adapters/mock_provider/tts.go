package mockprovider

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// Mock audio format. Small on purpose — the point is a genuinely valid RIFF
// file that exercises content addressing and the asset store, not three hours
// of narration.
const (
	wavSampleRate = 8000
	wavChannels   = 1
	wavBitDepth   = 16
	wavSeconds    = 1.0
	// wordsPerMinute is used only to report the narration length a real backend
	// would have produced, for the UI's chapter durations.
	wordsPerMinute = 150.0
)

// TTS is the mock narration backend.
type TTS struct {
	store  provider.AssetStore
	tuning Tuning
}

var _ provider.TTSProvider = (*TTS)(nil)

// NewTTS constructs the mock.
func NewTTS(store provider.AssetStore, tuning Tuning) *TTS {
	return &TTS{store: store, tuning: tuning}
}

// Speak narrates exactly one chapter and returns the audio's content address.
func (t *TTS) Speak(ctx context.Context, req provider.SpeakRequest) (entity.AssetID, error) {
	if err := simulate(ctx, t.tuning, 2); err != nil {
		return "", err
	}
	// The tone is derived from the request, so two chapters never produce the
	// same bytes and the content addressing is genuinely exercised.
	seed := seedOf(string(req.VideoID), strconv.Itoa(req.Ordinal), req.Voice, req.Text)
	buf := renderWAV(seed)

	stored, err := t.store.Put(ctx, entity.AssetKindAudio, bytes.NewReader(buf))
	if err != nil {
		return "", fmt.Errorf("store audio: %w", err)
	}
	return stored.ID, nil
}

// EstimateDurationSeconds reports how long a real backend would have narrated
// the given text. The mock's own file is one second regardless; this is what
// the UI shows as a chapter duration.
func EstimateDurationSeconds(text string) float64 {
	return float64(len(strings.Fields(text))) / wordsPerMinute * 60
}

// renderWAV builds a complete 16-bit PCM RIFF/WAVE file: a two-tone chord with
// a short fade, so the waveform is non-trivial and the file is genuinely valid.
func renderWAV(seed uint64) []byte {
	r := deterministic(seed)
	// Continuous rather than integer parameters: fifty chapters drawing from a
	// few hundred discrete tones collide often enough that two chapters would
	// share a content address.
	base := 110.0 + r.Float64()*220.0
	harmonic := base * (1.2 + r.Float64()*0.9)
	phase := r.Float64() * 2 * math.Pi

	samples := int(wavSampleRate * wavSeconds)
	dataBytes := samples * wavChannels * wavBitDepth / 8

	buf := bytes.NewBuffer(make([]byte, 0, 44+dataBytes))
	buf.WriteString("RIFF")
	writeLE32(buf, uint32(36+dataBytes)) //nolint:gosec // one second of 8 kHz audio
	buf.WriteString("WAVE")

	buf.WriteString("fmt ")
	writeLE32(buf, 16)            // subchunk size
	writeLE16(buf, 1)             // PCM
	writeLE16(buf, wavChannels)   //
	writeLE32(buf, wavSampleRate) //
	writeLE32(buf, wavSampleRate*wavChannels*wavBitDepth/8)
	writeLE16(buf, wavChannels*wavBitDepth/8)
	writeLE16(buf, wavBitDepth)

	buf.WriteString("data")
	writeLE32(buf, uint32(dataBytes)) //nolint:gosec // as above

	for i := range samples {
		tSec := float64(i) / wavSampleRate
		envelope := math.Min(1, math.Min(tSec*8, (wavSeconds-tSec)*8))
		v := math.Sin(2*math.Pi*base*tSec+phase)*0.6 + math.Sin(2*math.Pi*harmonic*tSec)*0.25
		writeLE16(buf, uint16(int16(v*envelope*20000))) //nolint:gosec // deliberate wrap-free range
	}
	return buf.Bytes()
}

func writeLE16(b *bytes.Buffer, v uint16) {
	var tmp [2]byte
	binary.LittleEndian.PutUint16(tmp[:], v)
	b.Write(tmp[:])
}

func writeLE32(b *bytes.Buffer, v uint32) {
	var tmp [4]byte
	binary.LittleEndian.PutUint32(tmp[:], v)
	b.Write(tmp[:])
}
