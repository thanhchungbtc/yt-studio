package ffmpeg

import (
	"strings"
	"testing"
)

// The filter graphs are the output: a wrong offset moves a transition and a
// wrong label breaks the map. They are asserted literally, character for
// character, because that is the only thing ffmpeg reads.

func TestSlideXfadeGraph(t *testing.T) {
	t.Parallel()

	got := slideXfadeGraph([]float64{2.5, 2.5, 2.5})
	want := "[0:v][1:v]xfade=transition=dissolve:duration=0.5:offset=2.0000[v01];" +
		"[v01][2:v]xfade=transition=dissolve:duration=0.5:offset=4.0000[vout]"
	if got != want {
		t.Errorf("graph mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestSlideXfadeGraphTwoSlides(t *testing.T) {
	t.Parallel()

	// Two slides is one transition, which must land on [vout] directly.
	got := slideXfadeGraph([]float64{1.25, 1.25})
	want := "[0:v][1:v]xfade=transition=dissolve:duration=0.5:offset=0.7500[vout]"
	if got != want {
		t.Errorf("graph mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestSlideXfadeGraphUsesProbedDurations(t *testing.T) {
	t.Parallel()

	// Unequal clips: the second offset must accumulate the real length of the
	// middle clip, not the first one's.
	got := slideXfadeGraph([]float64{3.0, 2.0, 4.0})
	if !strings.Contains(got, "offset=2.5000[v01]") {
		t.Errorf("first offset wrong: %s", got)
	}
	if !strings.Contains(got, "offset=4.0000[vout]") {
		t.Errorf("second offset wrong: %s", got)
	}
}

func TestLayOutTitles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		size        int
		wantChapter int
		wantVideo   int
	}{
		// 48 + 4 + 22 = 74, centred in 130 leaves 28 above.
		{name: "largest", size: 48, wantChapter: 28, wantVideo: 80},
		{name: "smallest", size: 20, wantChapter: 42, wantVideo: 66},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := layOutTitles(tc.size)
			if got.chapterY != tc.wantChapter || got.videoY != tc.wantVideo {
				t.Errorf("layout = (%d, %d), want (%d, %d)",
					got.chapterY, got.videoY, tc.wantChapter, tc.wantVideo)
			}
		})
	}
}

func TestSectionGeometry(t *testing.T) {
	t.Parallel()

	// Ten per cent larger than the chapter frame, rounded down to even, centred on
	// 1920x1080.
	w, h, x, y := sectionGeometry()
	if w != 1478 || h != 844 || x != 221 || y != 118 {
		t.Errorf("geometry = (%d, %d) at (%d, %d), want (1478, 844) at (221, 118)", w, h, x, y)
	}
	if w%2 != 0 || h%2 != 0 {
		t.Error("both dimensions must be even for the encoder")
	}
}

func TestChapterCompositeGraph(t *testing.T) {
	t.Parallel()

	got := chapterCompositeGraph("THE FALL", "Rome", "/f/CabinSketch-Bold.ttf", layOutTitles(48), 12.5)
	want := "[0:v]scale=1344:768,format=yuv420p[chalk];" +
		"[1:v]scale=1344:638:force_original_aspect_ratio=decrease," +
		"pad=1344:638:(ow-iw)/2:(oh-ih)/2:black,format=yuva420p[scaled];" +
		"[scaled]lumakey=threshold=0.1:tolerance=0.15:softness=0.05[keyed];" +
		"[chalk][keyed]overlay=0:130[comp];" +
		"[comp]drawtext=text='THE FALL':x=20:y=28:fontsize=48" +
		":fontfile=/f/CabinSketch-Bold.ttf:fontcolor=white:box=0[t1];" +
		"[t1]drawtext=text='Rome':x=20:y=80:fontsize=22" +
		":fontfile=/f/CabinSketch-Bold.ttf:fontcolor=white:box=0[out];" +
		"[out]tpad=start_mode=clone:start_duration=1.0:stop_mode=clone:stop_duration=2.5[out_padded];" +
		"[1:a]adelay=1000:all=1,apad=whole_dur=12.500[aout]"
	if got != want {
		t.Errorf("graph mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestChapterCompositeGraphWithoutFont(t *testing.T) {
	t.Parallel()

	got := chapterCompositeGraph("TITLE", "", "", layOutTitles(48), 5)
	if strings.Contains(got, "fontfile") {
		t.Errorf("no font file was given, yet the graph names one: %s", got)
	}
	if !strings.Contains(got, "text='':x=20") {
		t.Errorf("an absent video title must still draw an empty line: %s", got)
	}
}

func TestConcatGraphSingleClip(t *testing.T) {
	t.Parallel()

	// One chapter has nothing to crossfade with, so it is overlaid directly.
	got := concatGraph([]float64{30}, 0)
	want := "[0:v]scale=1920:1080:force_original_aspect_ratio=increase," +
		"crop=1920:1080,format=yuv420p[bg];" +
		"[1:v]scale=1478:844[sec_scaled];" +
		"[bg][sec_scaled]overlay=221:118[overlaid];" +
		"[overlaid]fade=t=in:st=0:d=1[final_v];" +
		"[1:a]afade=t=in:st=0:d=1[narration]"
	if got != want {
		t.Errorf("graph mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestConcatGraphChainsClipsAndMixesMusic(t *testing.T) {
	t.Parallel()

	got := concatGraph([]float64{10, 20, 30}, 4)
	for _, want := range []string{
		"[1:v][2:v]xfade=transition=dissolve:duration=1.0:offset=9.0000[v01]",
		"[1:a][2:a]acrossfade=d=1.0[a01]",
		"[v01][3:v]xfade=transition=dissolve:duration=1.0:offset=28.0000[sec_v]",
		"[a01][3:a]acrossfade=d=1.0[sec_a]",
		"[4:a]volume=0.35[bgm]",
		"[narration][bgm]amix=inputs=2:duration=first:dropout_transition=3:normalize=0[final_a]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("graph is missing %q\ngot: %s", want, got)
		}
	}
}

func TestConcatGraphWithoutMusic(t *testing.T) {
	t.Parallel()

	got := concatGraph([]float64{10, 20}, 0)
	if strings.Contains(got, "amix") || strings.Contains(got, "final_a") {
		t.Errorf("no music input, yet the graph mixes one: %s", got)
	}
}

func TestEscapeDrawtext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{in: "PLAIN TITLE", want: "PLAIN TITLE"},
		{in: "ROME: THE END", want: `ROME\: THE END`},
		{in: "DON'T PANIC", want: "DONT PANIC"},
		{in: `BACK\SLASH`, want: `BACK\\SLASH`},
	}
	for _, tc := range tests {
		if got := escapeDrawtext(tc.in); got != tc.want {
			t.Errorf("escapeDrawtext(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
