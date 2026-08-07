package thumbnail

import (
	"image"
	"os"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// fontCache parses a TTF once: fitting a headline measures it at a dozen sizes,
// and re-parsing a quarter-megabyte font per step is waste. Faces are cached
// separately by font and size, since two renders share both.
type fontCache struct {
	once sync.Once
	font *sfnt.Font
	err  error
}

func (f *fontCache) load(path string) (*sfnt.Font, error) {
	f.once.Do(func() {
		data, err := os.ReadFile(path) //nolint:gosec // path is operator-configured
		if err != nil {
			f.err = err
			return
		}
		f.font, f.err = sfnt.Parse(data)
	})
	return f.font, f.err
}

// faceOf builds a face at a pixel size. Faces are cached because fitting walks
// sizes and every cell measures a caption.
func faceOf(parsed *sfnt.Font, size int) (font.Face, error) {
	facesMu.Lock()
	defer facesMu.Unlock()
	key := faceKey{parsed, size}
	if face, ok := faces[key]; ok {
		return face, nil
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size: float64(size),
		// 72 dpi makes one point one pixel, so the size asked for here is the
		// pixel height the glyphs are rasterised at.
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, err
	}
	if faces == nil {
		faces = make(map[faceKey]font.Face, 16)
	}
	faces[key] = face
	return face, nil
}

type faceKey struct {
	font *sfnt.Font
	size int
}

var (
	facesMu sync.Mutex
	faces   map[faceKey]font.Face
)

// trackingFor is the extra space between glyphs: a face at its natural advances
// reads as ordinary text rather than as a title.
func trackingFor(size int) fixed.Int26_6 {
	return fixed.I(max(size/headlineTracking, 1))
}

// measure returns the drawn width of a string at a face, tracking included.
func measure(face font.Face, s string, tracking fixed.Int26_6) fixed.Int26_6 {
	var total fixed.Int26_6
	var prev rune
	for i, r := range s {
		if i > 0 {
			total += face.Kern(prev, r) + tracking
		}
		advance, ok := face.GlyphAdvance(r)
		if !ok {
			advance, _ = face.GlyphAdvance(' ')
		}
		total += advance
		prev = r
	}
	return total
}

// drawString draws one line at a baseline, glyph by glyph so tracking can be
// applied between them.
func drawString(dst *image.RGBA, face font.Face, s string, x, baseline int, tracking fixed.Int26_6, col image.Image) {
	d := font.Drawer{
		Dst:  dst,
		Src:  col,
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.I(x), Y: fixed.I(baseline)},
	}
	var prev rune
	for i, r := range s {
		if i > 0 {
			d.Dot.X += face.Kern(prev, r) + tracking
		}
		d.DrawString(string(r))
		prev = r
	}
}

// headlineLayout is a fitted headline: the lines, the size they fit at, and
// where they sit.
type headlineLayout struct {
	lines    []string
	size     int
	face     font.Face
	tracking fixed.Int26_6
	top      int
	lineH    int
}

// height is what the headline occupies.
func (h headlineLayout) height() int {
	if len(h.lines) == 0 {
		return 0
	}
	return len(h.lines) * h.lineH
}

// layOutHeadline fits the hook into the band the grid left above it: as large
// as it goes on one line, wrapping only when even the floor will not hold it.
// Two bounds, the frame's width and whatever height the grid did not take, so a
// fuller grid shrinks the headline rather than pushing tiles off the frame. One
// that fits neither is drawn at the floor and allowed to overflow — a clipped
// word beats a video that cannot produce a thumbnail.
func layOutHeadline(parsed *sfnt.Font, headline string, maxHeight int) headlineLayout {
	words := strings.Fields(strings.ToUpper(headline))
	if len(words) == 0 {
		return headlineLayout{}
	}
	maxWidth := fixed.I(frameWidth - 2*headlineSideMargin)

	// Every size is tried at one line before any is tried at two, so
	// small-and-single beats large-and-wrapped: wrapping costs the grid a third
	// of its height.
	for lines := 1; lines <= headlineMaxLines; lines++ {
		for size := headlineFontMax; size >= headlineFontMin; size -= headlineFontStep {
			face, err := faceOf(parsed, size)
			if err != nil {
				continue
			}
			tracking := trackingFor(size)
			wrapped := wrap(face, words, maxWidth, tracking)
			if len(wrapped) > lines {
				continue
			}
			candidate := headlineLayout{
				lines: wrapped, size: size, face: face, tracking: tracking,
				top: headlineTopMargin, lineH: size + headlineLineGap,
			}
			if candidate.height() <= maxHeight {
				return candidate
			}
		}
	}

	face, err := faceOf(parsed, headlineFontMin)
	if err != nil {
		return headlineLayout{}
	}
	tracking := trackingFor(headlineFontMin)
	lines := wrap(face, words, maxWidth, tracking)
	if len(lines) > headlineMaxLines {
		lines = lines[:headlineMaxLines]
	}
	return headlineLayout{
		lines: lines, size: headlineFontMin, face: face, tracking: tracking,
		top: headlineTopMargin, lineH: headlineFontMin + headlineLineGap,
	}
}

// wrap greedily breaks words into lines that fit.
func wrap(face font.Face, words []string, maxWidth, tracking fixed.Int26_6) []string {
	lines := make([]string, 0, headlineMaxLines+1)
	current := ""
	for _, w := range words {
		candidate := w
		if current != "" {
			candidate = current + " " + w
		}
		if measure(face, candidate, tracking) <= maxWidth || current == "" {
			current = candidate
			continue
		}
		lines = append(lines, current)
		current = w
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

// fitCaptions picks one size for every caption in the grid: the largest at
// which they all fit.
//
// Sizing each caption to its own tile would be easy and would look wrong — ten
// tiles with ten different type sizes read as ten unrelated pictures, and the
// grid's whole job is to read as a set.
func fitCaptions(parsed *sfnt.Font, captions []string, maxWidth int) font.Face {
	limit := fixed.I(maxWidth)
	for size := captionFontMax; size >= captionFontMin; size -= captionFontStep {
		face, err := faceOf(parsed, size)
		if err != nil {
			continue
		}
		if fitsAll(face, captions, limit) {
			return face
		}
	}
	face, err := faceOf(parsed, captionFontMin)
	if err != nil {
		return nil
	}
	// At the floor the long ones are cut rather than shrunk further: a caption
	// nobody can read is not worth the room it takes.
	return face
}

func fitsAll(face font.Face, captions []string, limit fixed.Int26_6) bool {
	for _, c := range captions {
		if measure(face, c, 0) > limit {
			return false
		}
	}
	return true
}

// captionText normalises a caption and cuts it to the tile if it still does not
// fit at the size the grid settled on.
func captionText(face font.Face, caption string, maxWidth int) string {
	caption = strings.Join(strings.Fields(caption), " ")
	if caption == "" || face == nil {
		return ""
	}
	limit := fixed.I(maxWidth)
	if measure(face, caption, 0) <= limit {
		return caption
	}
	return truncate(face, caption, limit)
}

// truncate drops characters until what is left fits, with an ellipsis to say
// that it was cut.
func truncate(face font.Face, s string, limit fixed.Int26_6) string {
	runes := []rune(s)
	for len(runes) > 1 {
		runes = runes[:len(runes)-1]
		candidate := strings.TrimSpace(string(runes)) + "..."
		if measure(face, candidate, 0) <= limit {
			return candidate
		}
	}
	return string(runes)
}
