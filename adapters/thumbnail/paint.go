package thumbnail

import (
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // the background is a JPEG; decoding it is why this is here

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// The palette. One red, one white, one grey: the design's whole range, and the
// reason a thumbnail from this renderer reads as one of a set.
var (
	inkWhite   = image.NewUniform(color.RGBA{R: 246, G: 246, B: 244, A: 255})
	inkCaption = image.NewUniform(color.RGBA{R: 226, G: 226, B: 222, A: 255})
	ruleRed    = color.RGBA{R: 219, G: 36, B: 36, A: 255}
	tileFill   = color.RGBA{R: 6, G: 6, B: 8, A: 235}
	tileEdge   = color.RGBA{R: 128, G: 128, B: 134, A: 255}
)

// cover scales an image to fill the frame and centre-crops the overflow, so a
// background of any aspect fills the thumbnail without distortion.
func cover(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return image.NewRGBA(image.Rect(0, 0, w, h))
	}
	// The larger of the two ratios is the one that covers.
	scale := max(float64(w)/float64(b.Dx()), float64(h)/float64(b.Dy()))
	sw, sh := int(float64(b.Dx())*scale), int(float64(b.Dy())*scale)

	scaled := image.NewRGBA(image.Rect(0, 0, sw, sh))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), src, b, draw.Src, nil)

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	offset := image.Pt((sw-w)/2, (sh-h)/2)
	draw.Draw(dst, dst.Bounds(), scaled, offset, draw.Src)
	return dst
}

// scrim darkens the background so white text over it is legible. The reference
// thumbnails are nearly black behind their headline; this is what gets a
// photograph there without losing its texture entirely.
func scrim(img *image.RGBA) {
	// Enough of the photograph to be a photograph, dark enough for white type to
	// sit on it without an outline.
	const keep = 150 // of 255
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = uint8(int(img.Pix[i]) * keep / 255)
		img.Pix[i+1] = uint8(int(img.Pix[i+1]) * keep / 255)
		img.Pix[i+2] = uint8(int(img.Pix[i+2]) * keep / 255)
		img.Pix[i+3] = 255
	}
}

// drawHeadline centres each line and lays the red rule under the block.
func drawHeadline(canvas *image.RGBA, h headlineLayout) {
	if len(h.lines) == 0 {
		return
	}
	for i, line := range h.lines {
		w := measure(h.face, line, h.tracking).Ceil()
		x := (width - w) / 2
		// The baseline sits a little above the bottom of the line box, which is
		// what keeps descenders inside the block rather than into the rule.
		baseline := h.top + i*h.lineH + h.size
		drawString(canvas, h.face, line, x, baseline, h.tracking, inkWhite)
	}

	// The rule runs to the right margin, under the back half of the headline —
	// the one piece of colour in the design.
	y := h.top + len(h.lines)*h.lineH + ruleGap
	drawRule(canvas, width-margin-ruleWidth, y, ruleWidth, ruleHeight)
}

// drawRule paints the underline as a slight rightward taper ending in a point,
// which is nearer the hand-drawn sweep of the reference than a flat bar and
// costs no asset to ship.
func drawRule(canvas *image.RGBA, x, y, w, h int) {
	for i := range w {
		// The stroke thins over the last fifth, so it reads as a stroke that was
		// drawn rather than a rectangle that was placed.
		thickness := h
		if tail := w * 4 / 5; i > tail && w > tail {
			remaining := float64(w-i) / float64(w-tail)
			thickness = max(int(float64(h)*remaining), 1)
		}
		// A gentle arc: the middle rides a pixel or two higher than the ends.
		lift := 0
		if w > 0 {
			t := float64(i) / float64(w)
			lift = int(4 * (t - t*t) * 4)
		}
		top := y - lift
		for dy := range thickness {
			py := top + dy
			if py >= 0 && py < height && x+i >= 0 && x+i < width {
				canvas.SetRGBA(x+i, py, ruleRed)
			}
		}
	}
}

// drawTile paints one cell: a dark plate, a thin border, and the icon scaled
// into what is left.
func drawTile(canvas *image.RGBA, box image.Rectangle, icon image.Image) {
	draw.Draw(canvas, box, image.NewUniform(tileFill), image.Point{}, draw.Over)

	// The border is what separates one tile from the next over a photograph.
	for i := range tileBorder {
		outline(canvas, box.Inset(i), tileEdge)
	}

	inner := box.Inset(tileBorder + tilePad)
	if inner.Dx() <= 0 || inner.Dy() <= 0 {
		return
	}
	// CatmullRom rather than nearest neighbour: these icons are line art, and
	// aliased strokes are the first thing that makes a thumbnail look cheap.
	xdraw.CatmullRom.Scale(canvas, inner, icon, icon.Bounds(), draw.Over, nil)
}

func outline(canvas *image.RGBA, box image.Rectangle, c color.RGBA) {
	for x := box.Min.X; x < box.Max.X; x++ {
		set(canvas, x, box.Min.Y, c)
		set(canvas, x, box.Max.Y-1, c)
	}
	for y := box.Min.Y; y < box.Max.Y; y++ {
		set(canvas, box.Min.X, y, c)
		set(canvas, box.Max.X-1, y, c)
	}
}

func set(canvas *image.RGBA, x, y int, c color.RGBA) {
	if x >= 0 && y >= 0 && x < width && y < height {
		canvas.SetRGBA(x, y, c)
	}
}

// drawCaption centres one caption under its tile, at the size the whole grid
// settled on.
func drawCaption(canvas *image.RGBA, face font.Face, caption string, box image.Rectangle, top int) {
	text := captionText(face, caption, box.Dx())
	if text == "" {
		return
	}
	w := measure(face, text, 0).Ceil()
	x := box.Min.X + (box.Dx()-w)/2
	metrics := face.Metrics()
	baseline := top + metrics.Ascent.Ceil()
	drawString(canvas, face, text, x, baseline, fixed.Int26_6(0), inkCaption)
}
