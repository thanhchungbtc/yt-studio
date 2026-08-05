package ffmpeg

import (
	"fmt"
	"strconv"
	"strings"
)

// The filter graphs live here as pure functions of their inputs, so the exact
// string handed to ffmpeg can be asserted in a test without a binary.

// slideXfadeGraph dissolves a chapter's slide clips into one track.
//
// Each transition starts fade seconds before the outgoing clip ends, so the
// offset walks forward by the clip's duration minus the overlap. The durations
// are the probed ones: a slide clip is a whole number of frames, and computing
// the offsets from the requested length instead would drift a frame per image.
func slideXfadeGraph(durations []float64) string {
	parts := make([]string, 0, len(durations)-1)
	prev := "[0:v]"
	offset := durations[0] - slideCrossfade
	for i := 1; i < len(durations); i++ {
		last := i == len(durations)-1
		out := fmt.Sprintf("[v%02d]", i)
		if last {
			out = "[vout]"
		}
		parts = append(parts, fmt.Sprintf(
			"%s[%d:v]xfade=transition=dissolve:duration=%s:offset=%s%s",
			prev, i, slideCrossfadeArg, f4(offset), out))
		prev = out
		if !last {
			offset += durations[i] - slideCrossfade
		}
	}
	return strings.Join(parts, ";")
}

// chapterLayout is where the two title lines sit inside the strip.
type chapterLayout struct {
	chapterSize int
	chapterY    int
	videoY      int
}

// layOutTitles centres both lines vertically in the title strip, keeping a
// four-pixel floor so a very large chapter title never rides off the top.
func layOutTitles(chapterSize int) chapterLayout {
	totalHeight := chapterSize + 4 + videoTitleSize
	chapterY := max(4, (titleStripHeight-totalHeight)/2)
	return chapterLayout{
		chapterSize: chapterSize,
		chapterY:    chapterY,
		videoY:      chapterY + chapterSize + 4,
	}
}

// chapterCompositeGraph is the chapter's final look: the slideshow keyed onto
// the chalkboard below a titled strip, padded for the crossfades that join it
// to its neighbours.
//
// Input 0 is the chalkboard background, input 1 the dissolved slideshow with its
// narration.
func chapterCompositeGraph(chapterTitle, videoTitle, fontFile string, layout chapterLayout, totalDuration float64) string {
	contentWidth, contentHeight := imageWidth, imageHeight-titleStripHeight

	fontArg := ""
	if fontFile != "" {
		fontArg = ":fontfile=" + escapeFilterPath(fontFile)
	}

	return strings.Join([]string{
		fmt.Sprintf("[0:v]scale=%d:%d,format=yuv420p[chalk]", imageWidth, imageHeight),
		fmt.Sprintf("[1:v]scale=%d:%d:force_original_aspect_ratio=decrease,"+
			"pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black,format=yuva420p[scaled]",
			contentWidth, contentHeight, contentWidth, contentHeight),
		// The slides are keyed rather than pasted: the chalkboard shows through
		// wherever the artwork is near-black, which is what makes a slide look drawn
		// on the board instead of stuck to it.
		"[scaled]lumakey=threshold=0.1:tolerance=0.15:softness=0.05[keyed]",
		fmt.Sprintf("[chalk][keyed]overlay=%d:%d[comp]", 0, titleStripHeight),
		fmt.Sprintf("[comp]drawtext=text='%s':x=20:y=%d:fontsize=%d%s:fontcolor=white:box=0[t1]",
			escapeDrawtext(chapterTitle), layout.chapterY, layout.chapterSize, fontArg),
		fmt.Sprintf("[t1]drawtext=text='%s':x=20:y=%d:fontsize=%d%s:fontcolor=white:box=0[out]",
			escapeDrawtext(videoTitle), layout.videoY, videoTitleSize, fontArg),
		// Cloning the first and last frames gives the neighbouring chapters something
		// to dissolve into without freezing on a black frame.
		fmt.Sprintf("[out]tpad=start_mode=clone:start_duration=%s:stop_mode=clone:stop_duration=%s[out_padded]",
			headPadArg, tailPadArg),
		fmt.Sprintf("[1:a]adelay=%s:all=1,apad=whole_dur=%s[aout]", adelayMillis, f3(totalDuration)),
	}, ";")
}

// sectionGeometry is the chapter track's size and position on the 1080p canvas:
// ten per cent larger than the chapter's own frame, centred, both dimensions
// even because the encoder requires it.
func sectionGeometry() (width, height, x, y int) {
	scale := 1.1
	width = int(concatImageWidth*scale) / 2 * 2
	height = int(concatImageHeight*scale) / 2 * 2
	return width, height, (concatWidth - width) / 2, (concatHeight - height) / 2
}

// concatGraph joins the chapter clips over the looping background, with the
// music mixed under the narration.
//
// Input 0 is the background video, inputs 1..n the chapter clips, and input n+1
// the music when there is any.
func concatGraph(durations []float64, bgMusicIndex int) string {
	n := len(durations)
	secWidth, secHeight, padX, padY := sectionGeometry()

	parts := []string{
		fmt.Sprintf("[0:v]scale=%d:%d:force_original_aspect_ratio=increase,"+
			"crop=%d:%d,format=yuv420p[bg]",
			concatWidth, concatHeight, concatWidth, concatHeight),
	}

	if n == 1 {
		parts = append(parts,
			fmt.Sprintf("[1:v]scale=%d:%d[sec_scaled]", secWidth, secHeight),
			fmt.Sprintf("[bg][sec_scaled]overlay=%d:%d[overlaid]", padX, padY),
			"[overlaid]fade=t=in:st=0:d=1[final_v]",
			"[1:a]afade=t=in:st=0:d=1[narration]")
	} else {
		prevVideo, prevAudio := "[1:v]", "[1:a]"
		offset := durations[0] - chapterCrossfade
		for i := 1; i < n; i++ {
			last := i == n-1
			videoOut, audioOut := fmt.Sprintf("[v%02d]", i), fmt.Sprintf("[a%02d]", i)
			if last {
				videoOut, audioOut = "[sec_v]", "[sec_a]"
			}
			parts = append(parts,
				fmt.Sprintf("%s[%d:v]xfade=transition=dissolve:duration=%s:offset=%s%s",
					prevVideo, i+1, crossfadeArg, f4(offset), videoOut),
				fmt.Sprintf("%s[%d:a]acrossfade=d=%s%s",
					prevAudio, i+1, crossfadeArg, audioOut))
			prevVideo, prevAudio = videoOut, audioOut
			if !last {
				offset += durations[i] - chapterCrossfade
			}
		}
		parts = append(parts,
			fmt.Sprintf("[sec_v]scale=%d:%d[sec_scaled]", secWidth, secHeight),
			fmt.Sprintf("[bg][sec_scaled]overlay=%d:%d[overlaid]", padX, padY),
			"[overlaid]fade=t=in:st=0:d=1[final_v]",
			"[sec_a]afade=t=in:st=0:d=1[narration]")
	}

	if bgMusicIndex > 0 {
		parts = append(parts,
			fmt.Sprintf("[%d:a]volume=%s[bgm]", bgMusicIndex, bgMusicVolume),
			"[narration][bgm]amix=inputs=2:duration=first:dropout_transition=3:normalize=0[final_a]")
	}
	return strings.Join(parts, ";")
}

// f3, f4 and f6 format a duration for a filter option or a -t flag. The
// precision is part of the output: a coarser offset moves a transition by a
// frame.
func f3(v float64) string { return strconv.FormatFloat(v, 'f', 3, 64) }
func f4(v float64) string { return strconv.FormatFloat(v, 'f', 4, 64) }
func f6(v float64) string { return strconv.FormatFloat(v, 'f', 6, 64) }
