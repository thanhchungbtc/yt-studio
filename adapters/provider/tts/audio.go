package tts

import (
	"fmt"
	"math"
)

// The audio helpers, ported one for one from the Python the backends replace.
// Free functions, because none of them touches the network or the store.

// ConcatWavs joins WAV blobs with silenceMillis between each pair. The first
// part's format defines the output; one that disagrees is an error rather than
// a resample, because chunks at different sample rates are a server problem.
func ConcatWavs(parts [][]byte, silenceMillis int) ([]byte, error) {
	switch len(parts) {
	case 0:
		return nil, nil
	case 1:
		// Nothing to join: the bytes the server sent are the bytes stored, and
		// their content address with them.
		return parts[0], nil
	}

	decoded := make([]wavAudio, 0, len(parts))
	for i, part := range parts {
		audio, err := decodeWAV(part)
		if err != nil {
			return nil, fmt.Errorf("chunk %d: %w", i+1, err)
		}
		if i > 0 && !audio.sameFormatAs(decoded[0]) {
			return nil, fmt.Errorf("%w: chunk %d is %s, chunk 1 is %s",
				ErrUnavailable, i+1, audio.format(), decoded[0].format())
		}
		decoded = append(decoded, audio)
	}

	first := decoded[0]
	silence := make([]byte, max(0, silenceMillis)*first.SampleRate/1000*first.bytesPerFrame())

	size := len(silence) * (len(decoded) - 1)
	for _, audio := range decoded {
		size += len(audio.Frames)
	}

	out := wavAudio{
		Channels:    first.Channels,
		SampleWidth: first.SampleWidth,
		SampleRate:  first.SampleRate,
		Frames:      make([]byte, 0, size),
	}
	for i, audio := range decoded {
		out.Frames = append(out.Frames, audio.Frames...)
		if i < len(decoded)-1 {
			out.Frames = append(out.Frames, silence...)
		}
	}
	return encodeWAV(out), nil
}

// CleanTail trims trailing near-silence and fades the last fadeMillis to zero:
// the trim removes the click some generations end on, the fade covers a click
// loud enough to survive the threshold. Anything it cannot read comes back
// unchanged — a tail that cannot be cleaned is no reason to lose a chapter.
//
// silenceThreshold is a fraction of full scale: 0.005 is roughly -46 dBFS.
func CleanTail(audio []byte, fadeMillis int, silenceThreshold float64) []byte {
	decoded, err := decodeWAV(audio)
	if err != nil {
		return audio
	}
	frameBytes := decoded.bytesPerFrame()
	if decoded.SampleWidth != 2 || decoded.SampleRate <= 0 || frameBytes == 0 || len(decoded.Frames) == 0 {
		return audio
	}
	frames := len(decoded.Frames) / frameBytes
	if frames == 0 {
		return audio
	}

	threshold := int32(silenceThreshold * math.MaxInt16)
	// Backwards: the answer is at the end by definition, and a ten-minute
	// chapter is tens of millions of samples.
	for frames > 0 && frameMagnitude(decoded.Frames, frames-1, decoded.Channels) <= threshold {
		frames--
	}
	if frames == 0 {
		// Every sample is below the threshold. Like the Python, keep the audio
		// whole: generated silence is still the chapter's narration.
		frames = len(decoded.Frames) / frameBytes
	}

	// Copied, not sliced: the fade writes into these bytes.
	out := wavAudio{
		Channels:    decoded.Channels,
		SampleWidth: decoded.SampleWidth,
		SampleRate:  decoded.SampleRate,
		Frames:      append([]byte(nil), decoded.Frames[:frames*frameBytes]...),
	}
	fadeOut(out.Frames, min(decoded.SampleRate*fadeMillis/1000, frames), decoded.Channels)
	return encodeWAV(out)
}

// frameMagnitude is the loudest channel of one frame, as a magnitude.
func frameMagnitude(frames []byte, frame, channels int) int32 {
	loudest := int32(0)
	base := frame * channels * 2
	for c := range channels {
		sample := int32(int16(uint16(frames[base+c*2]) | uint16(frames[base+c*2+1])<<8)) //nolint:gosec // 16-bit PCM
		if sample < 0 {
			sample = -sample
		}
		if sample > loudest {
			loudest = sample
		}
	}
	return loudest
}

// fadeOut ramps the last fadeFrames linearly to zero, in place.
func fadeOut(frames []byte, fadeFrames, channels int) {
	if fadeFrames <= 1 {
		// A one-frame ramp starts at full scale and never reaches zero.
		return
	}
	total := len(frames) / (channels * 2)
	for i := range fadeFrames {
		gain := 1 - float64(i)/float64(fadeFrames-1)
		base := (total - fadeFrames + i) * channels * 2
		for c := range channels {
			at := base + c*2
			sample := int16(uint16(frames[at]) | uint16(frames[at+1])<<8) //nolint:gosec // 16-bit PCM
			// Truncating, not rounding, to match the numpy cast.
			faded := uint16(int16(float64(sample) * gain)) //nolint:gosec // the gain is within [0,1]
			frames[at] = byte(faded)
			frames[at+1] = byte(faded >> 8)
		}
	}
}
