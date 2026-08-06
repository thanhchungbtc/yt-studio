package tts

import (
	"strings"
	"testing"
)

// wav builds a mono 16-bit WAV carrying n frames of a constant amplitude, which
// is all any of these tests needs from the audio itself.
func wav(sampleRate, frames int, amplitude int16) []byte {
	payload := make([]byte, frames*2)
	for i := 0; i < frames; i++ {
		payload[i*2] = byte(amplitude)
		payload[i*2+1] = byte(amplitude >> 8)
	}
	return encodeWAV(wavAudio{Channels: 1, SampleWidth: 2, SampleRate: sampleRate, Frames: payload})
}

func TestChunkTextBySentenceKeepsEveryWord(t *testing.T) {
	t.Parallel()
	text := "One two three. Four five six! Seven eight nine? Ten eleven twelve."
	chunks := chunkTextBySentence(text, 20)
	if len(chunks) < 2 {
		t.Fatalf("expected the text to split, got %d chunk(s)", len(chunks))
	}
	// The join is what reaches the listener; losing or duplicating a sentence is
	// the failure that matters, not where the boundaries landed.
	rejoined := strings.Join(chunks, " ")
	for _, want := range []string{"One two three.", "Four five six!", "Seven eight nine?", "Ten eleven twelve."} {
		if !strings.Contains(rejoined, want) {
			t.Errorf("chunks lost %q; got %q", want, rejoined)
		}
	}
}

func TestChunkTextBySentenceEdgeCases(t *testing.T) {
	t.Parallel()
	if got := chunkTextBySentence("", 100); got != nil {
		t.Errorf("empty text = %v, want nil", got)
	}
	if got := chunkTextBySentence("   \n\t ", 100); got != nil {
		t.Errorf("blank text = %v, want nil", got)
	}
	// A zero floor divided unguarded in the Python this replaces. It must cost
	// the chunking, not the chapter.
	if got := chunkTextBySentence("One. Two.", 0); len(got) == 0 {
		t.Error("a zero floor produced no chunks; the chapter would be silent")
	}
}

func TestConcatWavsPassesOneThrough(t *testing.T) {
	t.Parallel()
	only := wav(22050, 100, 1000)
	got, err := concatWavs([][]byte{only}, 200)
	if err != nil {
		t.Fatalf("concatWavs: %v", err)
	}
	// Byte-identical, not merely equivalent: re-encoding a single part would
	// change its content address and defeat the dedupe on a re-run.
	if string(got) != string(only) {
		t.Error("a single part was re-encoded rather than passed through")
	}
}

func TestConcatWavsJoinsWithSilence(t *testing.T) {
	t.Parallel()
	const rate = 22050
	const silenceMillis = 200
	a, b := wav(rate, 1000, 500), wav(rate, 1500, 500)

	joined, err := concatWavs([][]byte{a, b}, silenceMillis)
	if err != nil {
		t.Fatalf("concatWavs: %v", err)
	}
	decoded, err := decodeWAV(joined)
	if err != nil {
		t.Fatalf("decodeWAV: %v", err)
	}
	wantFrames := 1000 + 1500 + rate*silenceMillis/1000
	if got := len(decoded.Frames) / decoded.bytesPerFrame(); got != wantFrames {
		t.Errorf("joined frames = %d, want %d", got, wantFrames)
	}
	if decoded.SampleRate != rate {
		t.Errorf("sample rate = %d, want %d", decoded.SampleRate, rate)
	}
}

func TestConcatWavsRejectsMismatchedFormats(t *testing.T) {
	t.Parallel()
	// Splicing 22.05k onto 44.1k would play at the wrong pitch rather than fail,
	// which is why the format is checked rather than assumed.
	if _, err := concatWavs([][]byte{wav(22050, 100, 500), wav(44100, 100, 500)}, 0); err == nil {
		t.Error("joining mismatched sample rates succeeded; it should refuse")
	}
}

func TestCleanTailTrimsTrailingSilence(t *testing.T) {
	t.Parallel()
	const rate = 22050
	loud := make([]byte, 0, 2000*2)
	for i := 0; i < 1000; i++ {
		loud = append(loud, 0x00, 0x40) // ~0.5 full scale
	}
	for i := 0; i < 1000; i++ {
		loud = append(loud, 0x00, 0x00) // silence to trim
	}
	blob := encodeWAV(wavAudio{Channels: 1, SampleWidth: 2, SampleRate: rate, Frames: loud})

	cleaned := cleanTail(blob, defaultFadeMillis, defaultSilenceThreshold)
	decoded, err := decodeWAV(cleaned)
	if err != nil {
		t.Fatalf("decodeWAV: %v", err)
	}
	got := len(decoded.Frames) / decoded.bytesPerFrame()
	if got >= 2000 {
		t.Errorf("trailing silence survived: %d frames of 2000", got)
	}
	if got < 900 {
		t.Errorf("the trim ate into the speech: %d frames left of 1000 loud ones", got)
	}
}

func TestPrependChapterTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		body    string
		title   string
		isIntro bool
		want    string
	}{
		{"announces the title", "Body text.", "The Long Winter", false, "The Long Winter.\n\nBody text."},
		{"intro is spoken as written", "Body text.", "The Long Winter", true, "Body text."},
		{"an empty title announces nothing", "Body text.", "", false, "Body text."},
		{"a blank title announces nothing", "Body text.", "   ", false, "Body text."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := prependChapterTitle(tt.body, tt.title, tt.isIntro); got != tt.want {
				t.Errorf("prependChapterTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeStripsOnlySurroundingSpace(t *testing.T) {
	t.Parallel()
	if got := normalize("  Two  spaces inside.\n"); got != "Two  spaces inside." {
		t.Errorf("normalize() = %q; it must not touch the interior", got)
	}
}

func TestWAVRoundTrip(t *testing.T) {
	t.Parallel()
	original := wavAudio{Channels: 1, SampleWidth: 2, SampleRate: 22050, Frames: []byte{1, 2, 3, 4, 5, 6, 7, 8}}
	decoded, err := decodeWAV(encodeWAV(original))
	if err != nil {
		t.Fatalf("decodeWAV: %v", err)
	}
	if decoded.Channels != original.Channels || decoded.SampleWidth != original.SampleWidth ||
		decoded.SampleRate != original.SampleRate || string(decoded.Frames) != string(original.Frames) {
		t.Errorf("round trip = %+v, want %+v", decoded, original)
	}
}

func TestDecodeWAVRejectsNonWAV(t *testing.T) {
	t.Parallel()
	// An HTML error page fetched instead of audio is the realistic case: it must
	// fail here rather than be stored and composed into a video.
	if _, err := decodeWAV([]byte("<html>404</html>")); err == nil {
		t.Error("decodeWAV accepted non-WAV bytes")
	}
}
