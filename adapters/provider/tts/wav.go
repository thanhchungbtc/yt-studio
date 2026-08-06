package tts

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// The RIFF/WAVE handling the Python got free from the `wave` module.
//
// It lives in this package rather than being borrowed from the ffmpeg adapter
// on purpose: what that package reads is a duration out of a header, and what
// this one needs is the frames themselves. Shelling out to ffmpeg to glue two
// generations together would also put a second process on the TTS path, which
// is the one path that is already slow.

// errNotWAV reports bytes that are not a RIFF/WAVE file, or are one this cannot
// read. cleanTail treats it as "leave the audio alone" rather than as a
// failure, which is why it is a sentinel rather than a message.
var errNotWAV = errors.New("xtts: not a readable RIFF/WAVE file")

// headerSize is the canonical RIFF/WAVE header this package writes: 12 bytes of
// RIFF, a 24-byte `fmt ` chunk, an 8-byte `data` header.
const headerSize = 44

// pcmFormat is the only audio format handled here.
const pcmFormat = 1

// wavAudio is one decoded PCM stream: the format that describes the frames, and
// the frames.
type wavAudio struct {
	// Channels is 1 for the mono the models produce, 2 if a server is
	// configured otherwise.
	Channels int
	// SampleWidth is bytes per sample per channel; 2 is the 16-bit PCM
	// everything here assumes.
	SampleWidth int
	// SampleRate is frames per second.
	SampleRate int
	// Frames is the raw payload of the data chunk, interleaved by channel.
	Frames []byte
}

// bytesPerFrame is one frame across every channel — the unit a trim or a splice
// must land on, since cutting between the channels of a frame swaps the ears
// for the rest of the file.
func (a wavAudio) bytesPerFrame() int { return a.Channels * a.SampleWidth }

// sameFormatAs reports whether two streams can be joined without resampling.
func (a wavAudio) sameFormatAs(b wavAudio) bool {
	return a.Channels == b.Channels && a.SampleWidth == b.SampleWidth && a.SampleRate == b.SampleRate
}

// format describes the stream for an error message.
func (a wavAudio) format() string {
	return fmt.Sprintf("%d Hz, %d ch, %d-bit", a.SampleRate, a.Channels, a.SampleWidth*8)
}

// decodeWAV reads a RIFF/WAVE blob into its format and its frames.
//
// The chunks are walked rather than the canonical 44-byte header assumed: a
// LIST chunk between `fmt ` and `data` is legal and some builds emit one, and a
// fixed offset would read that metadata as audio — a burst of noise at the head
// of every chunk of narration.
func decodeWAV(blob []byte) (wavAudio, error) {
	if len(blob) < 12 || string(blob[0:4]) != "RIFF" || string(blob[8:12]) != "WAVE" {
		return wavAudio{}, errNotWAV
	}

	var audio wavAudio
	var haveFormat, haveData bool

	for at := 12; at+8 <= len(blob); {
		id := string(blob[at : at+4])
		size := int(binary.LittleEndian.Uint32(blob[at+4 : at+8]))
		body := blob[at+8:]
		// A streaming writer cannot know the length in advance and leaves it at
		// zero or at the maximum, so the declared size is trusted only as far as
		// the bytes that are actually here.
		if size < 0 || size > len(body) {
			size = len(body)
		}

		switch id {
		case "fmt ":
			if size < 16 {
				return wavAudio{}, errNotWAV
			}
			if binary.LittleEndian.Uint16(body[0:2]) != pcmFormat {
				return wavAudio{}, errNotWAV
			}
			audio.Channels = int(binary.LittleEndian.Uint16(body[2:4]))
			audio.SampleRate = int(binary.LittleEndian.Uint32(body[4:8]))
			audio.SampleWidth = int(binary.LittleEndian.Uint16(body[14:16])) / 8
			haveFormat = true
		case "data":
			audio.Frames = body[:size]
			haveData = true
		default:
			// Anything else is metadata: LIST, fact, cue.
		}
		if haveFormat && haveData {
			break
		}
		// Chunks are padded to an even length, and the pad byte is not counted in
		// the size — missing it puts every later chunk one byte out.
		at += 8 + size + size%2
	}

	if !haveFormat || !haveData || audio.Channels <= 0 || audio.SampleWidth <= 0 || audio.SampleRate <= 0 {
		return wavAudio{}, errNotWAV
	}
	// Frames must divide evenly, or a trim lands mid-frame later on.
	audio.Frames = audio.Frames[:len(audio.Frames)/audio.bytesPerFrame()*audio.bytesPerFrame()]
	return audio, nil
}

// encodeWAV writes the canonical header and the frames after it.
func encodeWAV(audio wavAudio) []byte {
	byteRate := audio.SampleRate * audio.bytesPerFrame()

	out := make([]byte, headerSize+len(audio.Frames))
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(headerSize-8+len(audio.Frames))) //nolint:gosec // a chapter of narration
	copy(out[8:12], "WAVE")

	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16)
	binary.LittleEndian.PutUint16(out[20:22], pcmFormat)
	binary.LittleEndian.PutUint16(out[22:24], uint16(audio.Channels))        //nolint:gosec // 1 or 2
	binary.LittleEndian.PutUint32(out[24:28], uint32(audio.SampleRate))      //nolint:gosec // from the decoded header
	binary.LittleEndian.PutUint32(out[28:32], uint32(byteRate))              //nolint:gosec // as above
	binary.LittleEndian.PutUint16(out[32:34], uint16(audio.bytesPerFrame())) //nolint:gosec // as above
	binary.LittleEndian.PutUint16(out[34:36], uint16(audio.SampleWidth*8))   //nolint:gosec // 16

	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(len(audio.Frames))) //nolint:gosec // as above
	copy(out[headerSize:], audio.Frames)
	return out
}
