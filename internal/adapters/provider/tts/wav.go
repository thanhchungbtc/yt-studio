package tts

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// The RIFF/WAVE handling the Python got free from the `wave` module. It is here
// rather than borrowed from the ffmpeg adapter, which reads durations out of
// headers where this needs the frames — and shelling out would put a second
// process on the slowest path in the pipeline.

// errNotWAV reports bytes this cannot read. A sentinel because CleanTail treats
// it as "leave the audio alone" rather than as a failure.
var errNotWAV = errors.New("tts: not a readable RIFF/WAVE file")

// headerSize is the canonical RIFF/WAVE header this package writes: 12 bytes of
// RIFF, a 24-byte `fmt ` chunk, an 8-byte `data` header.
const headerSize = 44

// pcmFormat is the only audio format handled here.
const pcmFormat = 1

// wavAudio is one decoded PCM stream: the format, and the frames.
type wavAudio struct {
	// Channels is 1 for the mono the models produce.
	Channels int
	// SampleWidth is bytes per sample per channel; 2 is the 16-bit PCM assumed
	// throughout.
	SampleWidth int
	// SampleRate is frames per second.
	SampleRate int
	// Frames is the data chunk's payload, interleaved by channel.
	Frames []byte
}

// bytesPerFrame is one frame across every channel — the unit a trim must land
// on, since cutting between channels swaps the ears for the rest of the file.
func (a wavAudio) bytesPerFrame() int { return a.Channels * a.SampleWidth }

// sameFormatAs reports whether two streams can be joined without resampling.
func (a wavAudio) sameFormatAs(b wavAudio) bool {
	return a.Channels == b.Channels && a.SampleWidth == b.SampleWidth && a.SampleRate == b.SampleRate
}

// format describes the stream for an error message.
func (a wavAudio) format() string {
	return fmt.Sprintf("%d Hz, %d ch, %d-bit", a.SampleRate, a.Channels, a.SampleWidth*8)
}

// decodeWAV reads a RIFF/WAVE blob into its format and its frames. The chunks
// are walked rather than the 44-byte header assumed: a LIST chunk before `data`
// is legal, and a fixed offset would read that metadata as a burst of noise.
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
		// A streaming writer cannot know the length and leaves it at zero or the
		// maximum, so the declared size is trusted only as far as the bytes here.
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
		// Chunks are padded to an even length and the pad byte is not counted;
		// missing it puts every later chunk one byte out.
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

/*
DurationOf reads a WAV's length from its header, without holding the audio.

The counterpart to DurationSeconds for a backend that streams its file to the
store instead of decoding it — the sample narrator is one, and reading a
recording into memory to learn how long it is would undo the reason it streams.

Only the chunk headers are read: `fmt ` carries the byte rate, `data` declares
its own size, and everything between is seeked over. So the cost is a handful of
small reads whatever the file weighs.

The reader is left where it was found, because the caller is about to send the
same handle to the store and a stream already advanced past its header would be
written as a headerless fragment.

Zero for anything unreadable, exactly as DurationSeconds does.
*/
func DurationOf(r io.ReadSeeker) float64 {
	start, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0
	}
	defer func() { _, _ = r.Seek(start, io.SeekStart) }()

	var header [12]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0
	}
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return 0
	}

	var byteRate, dataSize uint32
	var chunk [8]byte
	for {
		if _, err := io.ReadFull(r, chunk[:]); err != nil {
			break
		}
		id := string(chunk[0:4])
		size := binary.LittleEndian.Uint32(chunk[4:8])
		switch id {
		case "fmt ":
			var body [16]byte
			if _, err := io.ReadFull(r, body[:]); err != nil {
				return 0
			}
			byteRate = binary.LittleEndian.Uint32(body[8:12])
			// Chunks are word-aligned and `fmt ` may carry extension bytes past
			// the 16 read above; both are skipped by seeking the declared size.
			if _, err := r.Seek(int64(size)-16+int64(size&1), io.SeekCurrent); err != nil {
				return 0
			}
		case "data":
			dataSize = size
			// The size the header declares, not the bytes on disk: a streaming
			// writer leaves 0xFFFFFFFF here, and that is caught below rather
			// than turned into a duration measured in centuries.
			if dataSize == 0xFFFFFFFF {
				return 0
			}
			if byteRate == 0 {
				return 0
			}
			return float64(dataSize) / float64(byteRate)
		default:
			if _, err := r.Seek(int64(size)+int64(size&1), io.SeekCurrent); err != nil {
				return 0
			}
		}
	}
	return 0
}
