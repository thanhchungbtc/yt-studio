package ffmpeg

import (
	"os"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// escapeDrawtext makes a caption safe to embed in a filter graph.
//
// drawtext parses its own escapes inside a value that is already delimited by
// single quotes, so a backslash has to survive twice and a colon would end the
// option. An apostrophe cannot be escaped from inside a quoted value at all and
// is dropped, which is what the reference pipeline does.
func escapeDrawtext(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, "")
	s = strings.ReplaceAll(s, `:`, `\:`)
	return s
}

// escapeFilterPath escapes a filename used as a filter option value. Ordinary
// paths pass through unchanged.
func escapeFilterPath(p string) string {
	p = strings.ReplaceAll(p, `\`, `\\`)
	p = strings.ReplaceAll(p, `:`, `\:`)
	p = strings.ReplaceAll(p, `'`, `\'`)
	return p
}

// fontCache parses a TTF once and keeps the faces it has measured with. A
// fifty-chapter video measures fifty titles; re-parsing a 260 KB font each time
// is pure waste.
type fontCache struct {
	once  sync.Once
	font  *sfnt.Font
	err   error
	mu    sync.Mutex
	faces map[int]font.Face
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

func (f *fontCache) face(parsed *sfnt.Font, size int) (font.Face, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if face, ok := f.faces[size]; ok {
		return face, nil
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size: float64(size),
		// 72 dpi makes one point one pixel, so the size passed here is the pixel size
		// drawtext will render at.
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, err
	}
	if f.faces == nil {
		f.faces = make(map[int]font.Face, 8)
	}
	f.faces[size] = face
	return face, nil
}

// fitFontSize picks the largest size in [titleFontMin, titleFontMax] whose
// rendered width fits maxWidth, stepping down by two.
//
// A title that does not fit even at the minimum is drawn at the minimum and
// allowed to overflow — the reference pipeline does the same, and a clipped
// title is better than a chapter that fails to compose.
func (c *Composer) fitFontSize(text string, maxWidth int) int {
	parsed, err := c.fonts.load(c.res.TitleFont)
	if err != nil {
		// No measurable font: drawtext falls back to its built-in, and so do we.
		return titleFontMax
	}
	for size := titleFontMax; size >= titleFontMin; size -= titleFontStep {
		face, err := c.fonts.face(parsed, size)
		if err != nil {
			return titleFontMax
		}
		if textWidth(face, text) <= maxWidth {
			return size
		}
	}
	return titleFontMin
}

// textWidth is the ink width of a single line: the horizontal extent of what
// actually gets drawn, which is what the reference measures.
func textWidth(face font.Face, text string) int {
	bounds, _ := font.BoundString(face, text)
	width := bounds.Max.X - bounds.Min.X
	if width < 0 {
		return 0
	}
	return width.Ceil()
}
