package mock

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// A hand-written minimal ISO base media file writer and reader, so the mock
// composer exercises content addressing and the asset store against a
// structurally valid MP4.
//
// The output is two real tracks — the chapter's PNGs under QuickTime's `png `
// sample entry, and the narration as 16-bit PCM (`sowt`) — with no
// transcoding and no media wrapper library.

// mp4Sample is one run of payload bytes: its size, known up front so the sample
// tables can be written before any payload is copied, and a reader opened
// lazily so the data is streamed rather than buffered.
type mp4Sample struct {
	size int64
	open func() (io.ReadCloser, error)
}

const (
	// mp4Timescale is the movie and video-track timescale, in ticks/second.
	mp4Timescale = 600
	// mp4FrameTicks is the fallback hold time for a slide when a clip carries no
	// narration to pace it against: five seconds.
	mp4FrameTicks = 3000
	mp4Width      = imageWidth
	mp4Height     = imageHeight

	// The audio track mirrors the WAV the TTS mock produces.
	mp4AudioTimescale      = wavSampleRate
	mp4AudioBytesPerSample = wavChannels * wavBitDepth / 8
	// wavHeaderBytes is the canonical RIFF/WAVE header length; the mock writes
	// exactly this shape, so the PCM payload starts here.
	wavHeaderBytes = 44
)

var errNotMP4 = errors.New("not a yt-studio mock mp4")

func be32(v uint32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return b[:]
}

func be16(v uint16) []byte {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	return b[:]
}

// box assembles a length-prefixed ISO box from its payload.
func box(kind string, payload ...[]byte) []byte {
	n := 8
	for _, p := range payload {
		n += len(p)
	}
	out := make([]byte, 0, n)
	out = append(out, be32(uint32(n))...) //nolint:gosec // mock output is far below 4 GiB
	out = append(out, kind...)
	for _, p := range payload {
		out = append(out, p...)
	}
	return out
}

func fullBox(kind string, version byte, flags uint32, payload ...[]byte) []byte {
	head := []byte{version, byte(flags >> 16), byte(flags >> 8), byte(flags)}
	return box(kind, append([][]byte{head}, payload...)...)
}

func totalSize(samples []mp4Sample) int64 {
	var n int64
	for _, s := range samples {
		n += s.size
	}
	return n
}

// writeMP4 streams a one- or two-track MP4 to w. Every size is known before a
// byte of payload moves, so chunk offsets are exact and nothing is buffered.
func writeMP4(w io.Writer, video, audio []mp4Sample) (int64, error) {
	if len(video) == 0 {
		return 0, errors.New("mp4: at least one video sample is required")
	}
	ftyp := box("ftyp", []byte("isom"), be32(512), []byte("isomiso2mp41"))

	videoBytes := totalSize(video)
	audioBytes := totalSize(audio)
	mdatPayload := videoBytes + audioBytes

	mdatHeader := make([]byte, 0, 8)
	mdatHeader = append(mdatHeader, be32(uint32(8+mdatPayload))...) //nolint:gosec // mock output is small
	mdatHeader = append(mdatHeader, "mdat"...)

	videoOffset := uint32(len(ftyp) + len(mdatHeader)) //nolint:gosec // small
	audioOffset := videoOffset + uint32(videoBytes)    //nolint:gosec // small
	moov := buildMoov(video, videoOffset, audioBytes, audioOffset)

	var written int64
	write := func(b []byte, what string) error {
		n, err := w.Write(b)
		written += int64(n)
		if err != nil {
			return fmt.Errorf("mp4: write %s: %w", what, err)
		}
		return nil
	}
	if err := write(ftyp, "ftyp"); err != nil {
		return written, err
	}
	if err := write(mdatHeader, "mdat header"); err != nil {
		return written, err
	}
	for i, s := range append(append(make([]mp4Sample, 0, len(video)+len(audio)), video...), audio...) {
		rc, err := s.open()
		if err != nil {
			return written, fmt.Errorf("mp4: open sample %d: %w", i, err)
		}
		copied, err := io.Copy(w, rc)
		written += copied
		closeErr := rc.Close()
		switch {
		case err != nil:
			return written, fmt.Errorf("mp4: copy sample %d: %w", i, err)
		case closeErr != nil:
			return written, fmt.Errorf("mp4: close sample %d: %w", i, closeErr)
		case copied != s.size:
			return written, fmt.Errorf("mp4: sample %d declared %d bytes but wrote %d", i, s.size, copied)
		}
	}
	if err := write(moov, "moov"); err != nil {
		return written, err
	}
	return written, nil
}

var unityMatrix = []byte{
	0x00, 0x01, 0x00, 0x00, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0x00, 0x01, 0x00, 0x00, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0x40, 0x00, 0x00, 0x00,
}

func buildMoov(video []mp4Sample, videoOffset uint32, audioBytes int64, audioOffset uint32) []byte {
	audioSamples := uint32(audioBytes / mp4AudioBytesPerSample) //nolint:gosec // small

	// Each slide is held for its share of the narration, so the two tracks come
	// out the same length and a preview does not run silent partway through.
	frameTicks := uint32(mp4FrameTicks)
	audioTicks := audioSamples * mp4Timescale / mp4AudioTimescale
	if audioTicks > 0 && len(video) > 0 {
		frameTicks = audioTicks / uint32(len(video)) //nolint:gosec // small
		if frameTicks == 0 {
			frameTicks = 1
		}
	}
	videoDuration := frameTicks * uint32(len(video)) //nolint:gosec // small
	movieDuration := videoDuration
	if audioTicks > movieDuration {
		movieDuration = audioTicks
	}
	nextTrack := uint32(2)
	if audioBytes > 0 {
		nextTrack = 3
	}

	mvhd := fullBox("mvhd", 0, 0,
		be32(0), be32(0),
		be32(mp4Timescale), be32(movieDuration),
		be32(0x00010000), // rate 1.0
		be16(0x0100),     // volume 1.0
		make([]byte, 10), // reserved
		unityMatrix,
		make([]byte, 24), // predefined
		be32(nextTrack),
	)

	children := [][]byte{mvhd, videoTrak(video, videoOffset, videoDuration, frameTicks)}
	if audioBytes > 0 {
		children = append(children, audioTrak(audioSamples, audioOffset))
	}
	return box("moov", children...)
}

func videoTrak(video []mp4Sample, offset, duration, frameTicks uint32) []byte {
	tkhd := fullBox("tkhd", 0, 0x000007,
		be32(0), be32(0),
		be32(1), be32(0),
		be32(duration),
		make([]byte, 8),
		be16(0), be16(0), be16(0), be16(0),
		unityMatrix,
		be32(mp4Width<<16), be32(mp4Height<<16),
	)
	mdhd := fullBox("mdhd", 0, 0,
		be32(0), be32(0),
		be32(mp4Timescale), be32(duration),
		be16(0x55C4), be16(0), // 'und'
	)
	hdlr := fullBox("hdlr", 0, 0, be32(0), []byte("vide"), make([]byte, 12),
		append([]byte("yt-studio video"), 0))

	visual := make([]byte, 0, 78)
	visual = append(visual, make([]byte, 6)...)
	visual = append(visual, be16(1)...)
	visual = append(visual, make([]byte, 16)...)
	visual = append(visual, be16(mp4Width)...)
	visual = append(visual, be16(mp4Height)...)
	visual = append(visual, be32(0x00480000)...)
	visual = append(visual, be32(0x00480000)...)
	visual = append(visual, be32(0)...)
	visual = append(visual, be16(1)...)
	compressor := make([]byte, 32)
	copy(compressor, append([]byte{9}, "yt-studio"...))
	visual = append(visual, compressor...)
	visual = append(visual, be16(24)...)
	visual = append(visual, be16(0xFFFF)...)

	stsd := fullBox("stsd", 0, 0, be32(1), box("png ", visual))
	stts := fullBox("stts", 0, 0, be32(1), be32(uint32(len(video))), be32(frameTicks)) //nolint:gosec // small
	stsc := fullBox("stsc", 0, 0, be32(1), be32(1), be32(uint32(len(video))), be32(1)) //nolint:gosec // small

	stszPayload := make([][]byte, 0, len(video)+2)
	stszPayload = append(stszPayload, be32(0), be32(uint32(len(video)))) //nolint:gosec // small
	for _, s := range video {
		stszPayload = append(stszPayload, be32(uint32(s.size))) //nolint:gosec // small
	}
	stsz := fullBox("stsz", 0, 0, stszPayload...)
	stco := fullBox("stco", 0, 0, be32(1), be32(offset))

	stbl := box("stbl", stsd, stts, stsc, stsz, stco)
	minf := box("minf", fullBox("vmhd", 0, 1, be16(0), be16(0), be16(0), be16(0)), dinfBox(), stbl)
	return box("trak", tkhd, box("mdia", mdhd, hdlr, minf))
}

func audioTrak(sampleCount, offset uint32) []byte {
	tkhd := fullBox("tkhd", 0, 0x000007,
		be32(0), be32(0),
		be32(2), be32(0),
		be32(sampleCount*mp4Timescale/mp4AudioTimescale),
		make([]byte, 8),
		be16(0), be16(0), be16(0x0100), be16(0), // volume 1.0
		unityMatrix,
		be32(0), be32(0),
	)
	mdhd := fullBox("mdhd", 0, 0,
		be32(0), be32(0),
		be32(mp4AudioTimescale), be32(sampleCount),
		be16(0x55C4), be16(0),
	)
	hdlr := fullBox("hdlr", 0, 0, be32(0), []byte("soun"), make([]byte, 12),
		append([]byte("yt-studio audio"), 0))

	audio := make([]byte, 0, 28)
	audio = append(audio, make([]byte, 6)...) // reserved
	audio = append(audio, be16(1)...)         // data reference index
	audio = append(audio, be16(0)...)         // version
	audio = append(audio, be16(0)...)         // revision
	audio = append(audio, be32(0)...)         // vendor
	audio = append(audio, be16(wavChannels)...)
	audio = append(audio, be16(wavBitDepth)...)
	audio = append(audio, be16(0)...) // compression id
	audio = append(audio, be16(0)...) // packet size
	audio = append(audio, be32(wavSampleRate<<16)...)

	stsd := fullBox("stsd", 0, 0, be32(1), box("sowt", audio))
	stts := fullBox("stts", 0, 0, be32(1), be32(sampleCount), be32(1))
	stsc := fullBox("stsc", 0, 0, be32(1), be32(1), be32(sampleCount), be32(1))
	stsz := fullBox("stsz", 0, 0, be32(mp4AudioBytesPerSample), be32(sampleCount))
	stco := fullBox("stco", 0, 0, be32(1), be32(offset))

	stbl := box("stbl", stsd, stts, stsc, stsz, stco)
	minf := box("minf", fullBox("smhd", 0, 0, be16(0), be16(0)), dinfBox(), stbl)
	return box("trak", tkhd, box("mdia", mdhd, hdlr, minf))
}

func dinfBox() []byte {
	return box("dinf", fullBox("dref", 0, 0, be32(1), fullBox("url ", 0, 1)))
}

// mp4Tracks is what readMP4 recovers from a clip: enough to re-emit its payload
// without decoding anything.
type mp4Tracks struct {
	video []mp4Sample
	audio []mp4Sample
}

// opener yields a fresh seekable handle on the same asset. Taking a factory
// rather than a path keeps this file free of any assumption that the asset
// store is a filesystem.
type opener func() (io.ReadSeekCloser, error)

// readMP4 re-reads a file written by writeMP4 and returns its payload as lazily
// opened, seeked readers, so concatenation is a genuine stream copy and no clip
// is ever held in memory.
func readMP4(open opener, size int64) (mp4Tracks, error) {
	rc, err := open()
	if err != nil {
		return mp4Tracks{}, fmt.Errorf("mp4: open: %w", err)
	}
	defer func() { _ = rc.Close() }()

	var moov []byte
	for offset := int64(0); offset < size; {
		var header [8]byte
		if err := readAt(rc, header[:], offset); err != nil {
			return mp4Tracks{}, fmt.Errorf("mp4: read box header: %w", err)
		}
		boxSize := int64(binary.BigEndian.Uint32(header[:4]))
		kind := string(header[4:8])
		if boxSize < 8 || offset+boxSize > size {
			return mp4Tracks{}, fmt.Errorf("%w: box %q has size %d at offset %d", errNotMP4, kind, boxSize, offset)
		}
		if kind == "moov" {
			moov = make([]byte, boxSize-8)
			if err := readAt(rc, moov, offset+8); err != nil {
				return mp4Tracks{}, fmt.Errorf("mp4: read moov: %w", err)
			}
		}
		offset += boxSize
	}
	if moov == nil {
		return mp4Tracks{}, fmt.Errorf("%w: no moov box", errNotMP4)
	}

	var tracks mp4Tracks
	for _, trak := range childBoxes(moov, "trak") {
		mdia := findBox(trak, "mdia")
		if mdia == nil {
			continue
		}
		hdlr := findBox(mdia, "hdlr")
		if len(hdlr) < 12 {
			continue
		}
		samples, err := samplesFromStbl(open, findBox(mdia, "minf", "stbl"))
		if err != nil {
			return mp4Tracks{}, err
		}
		switch string(hdlr[8:12]) {
		case "vide":
			tracks.video = samples
		case "soun":
			tracks.audio = samples
		}
	}
	if len(tracks.video) == 0 {
		return mp4Tracks{}, fmt.Errorf("%w: no video samples", errNotMP4)
	}
	return tracks, nil
}

func readAt(rs io.ReadSeeker, p []byte, offset int64) error {
	if _, err := rs.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	_, err := io.ReadFull(rs, p)
	return err
}

func samplesFromStbl(open opener, stbl []byte) ([]mp4Sample, error) {
	stsz := findBox(stbl, "stsz")
	stco := findBox(stbl, "stco")
	if len(stsz) < 12 || len(stco) < 12 {
		return nil, fmt.Errorf("%w: truncated sample table", errNotMP4)
	}
	uniform := int64(binary.BigEndian.Uint32(stsz[4:8]))
	count := int(binary.BigEndian.Uint32(stsz[8:12]))
	at := int64(binary.BigEndian.Uint32(stco[8:12]))

	if uniform > 0 {
		// Fixed-size samples (the PCM track): one contiguous run is enough, and it
		// keeps the concat path to a single copy per input file.
		return []mp4Sample{sectionSample(open, at, uniform*int64(count))}, nil
	}
	if len(stsz) < 12+4*count {
		return nil, fmt.Errorf("%w: sample size table is short", errNotMP4)
	}
	out := make([]mp4Sample, 0, count)
	for i := range count {
		sz := int64(binary.BigEndian.Uint32(stsz[12+4*i : 16+4*i]))
		out = append(out, sectionSample(open, at, sz))
		at += sz
	}
	return out, nil
}

// sectionSample is a byte range of an asset, opened on demand.
func sectionSample(open opener, start, size int64) mp4Sample {
	return mp4Sample{
		size: size,
		open: func() (io.ReadCloser, error) {
			rc, err := open()
			if err != nil {
				return nil, err
			}
			if _, err := rc.Seek(start, io.SeekStart); err != nil {
				return nil, errors.Join(err, rc.Close())
			}
			return sectionCloser{Reader: io.LimitReader(rc, size), closer: rc}, nil
		},
	}
}

// findBox walks a nested box path inside a container payload.
func findBox(payload []byte, path ...string) []byte {
	cur := payload
	for _, want := range path {
		found := false
		for off := 0; off+8 <= len(cur); {
			size := int(binary.BigEndian.Uint32(cur[off : off+4]))
			if size < 8 || off+size > len(cur) {
				return nil
			}
			if string(cur[off+4:off+8]) == want {
				cur = cur[off+8 : off+size]
				found = true
				break
			}
			off += size
		}
		if !found {
			return nil
		}
	}
	return cur
}

// childBoxes returns every direct child of the given kind.
func childBoxes(payload []byte, kind string) [][]byte {
	var out [][]byte
	for off := 0; off+8 <= len(payload); {
		size := int(binary.BigEndian.Uint32(payload[off : off+4]))
		if size < 8 || off+size > len(payload) {
			return out
		}
		if string(payload[off+4:off+8]) == kind {
			out = append(out, payload[off+8:off+size])
		}
		off += size
	}
	return out
}

type sectionCloser struct {
	io.Reader
	closer io.Closer
}

func (s sectionCloser) Close() error { return s.closer.Close() }
