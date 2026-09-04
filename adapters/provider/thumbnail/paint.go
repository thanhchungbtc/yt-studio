package thumbnail

import (
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // the background is a JPEG; decoding it is why this is here
	"math"
	"slices"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
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

// scrim darkens the background so white text over it is legible, without
// losing the photograph's texture entirely.
func scrim(img *image.RGBA) {
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = uint8(int(img.Pix[i]) * backgroundBrightness / 255)     //nolint:gosec // 0..255
		img.Pix[i+1] = uint8(int(img.Pix[i+1]) * backgroundBrightness / 255) //nolint:gosec // 0..255
		img.Pix[i+2] = uint8(int(img.Pix[i+2]) * backgroundBrightness / 255) //nolint:gosec // 0..255
		img.Pix[i+3] = 255
	}
}

// drawHeadline centres each line of the fitted headline, greying the words the
// layout marked minor so the ones that carry the hook read first, and
// thickening every stroke by weight pixels.
func drawHeadline(canvas *image.RGBA, h headlineLayout, weight float64) {
	if len(h.lines) == 0 {
		return
	}
	// Thickening costs a mask, a dilation and a composite per colour, so the
	// face's own weight is still drawn the direct way.
	if weight > 0 {
		drawHeadlineBold(canvas, h, weight)
		return
	}
	for i, line := range h.lines {
		w := measure(h.face, line, h.tracking).Ceil()
		x := (frameWidth - w) / 2
		// The baseline sits above the bottom of the line box, which is what keeps
		// descenders inside the block rather than in the gap below it.
		baseline := h.top + i*h.lineH + h.size
		drawString(canvas, h.face, line, x, baseline, h.tracking, headlineInk(h, i))
	}
}

// headlineInk maps each byte of line i to the colour its glyph is drawn in.
func headlineInk(h headlineLayout, i int) inker {
	return wordInk(h.lines[i], h.dim[i], headlineColor, headlineMinorColor)
}

// wordInk maps each byte of a line to one of two images, by whether the word
// that byte falls in was marked dim.
//
// A table rather than a lookup per glyph: the alternative is finding which word
// an offset falls in on every one of up to thirty glyphs, and the line is
// walked once here anyway. Lines arrive from wrap already single-space joined,
// so a run of non-spaces is exactly one word, and the nth run carries the nth
// flag.
func wordInk(line string, dim []bool, major, demoted image.Image) inker {
	if !slices.Contains(dim, true) {
		return solid(major)
	}
	table := make([]image.Image, len(line))
	for start, word := 0, 0; start < len(line); word++ {
		end := start
		for end < len(line) && line[end] != ' ' {
			end++
		}
		col := major
		if word < len(dim) && dim[word] {
			col = demoted
		}
		// The separating space is inked with the word it follows. Which colour a
		// blank glyph takes cannot show, but leaving the entry nil could.
		for i := start; i < end+1 && i < len(line); i++ {
			table[i] = col
		}
		start = end + 1
	}
	return func(offset int) image.Image {
		if offset < 0 || offset >= len(table) || table[offset] == nil {
			return major
		}
		return table[offset]
	}
}

// The two images a mask is drawn with: ink where the class is wanted, nothing
// where it is not. Only the alpha channel of the result is read.
var (
	maskInk   = image.NewUniform(color.RGBA{R: 255, G: 255, B: 255, A: 255})
	maskEmpty = image.NewUniform(color.RGBA{})
)

// drawHeadlineBold draws the headline thickened by weight pixels.
//
// The typeface has one weight and there is no heavier file to load, so the
// stroke is grown after rasterising rather than before: the line goes into a
// mask, the mask is dilated, and the colour is composited through it. That is
// what a stroker does to an outline, done on the raster because x/image/font
// has no stroker and the browser half of this renderer would not match one if
// it did -- a max filter over an alpha channel is arithmetic both sides can run
// to the same answer.
//
// One mask per colour rather than one for the line: dilating a single mask
// would grow the major and minor words into each other and lose which was
// which, and with it the whole point of drawing them differently.
func drawHeadlineBold(canvas *image.RGBA, h headlineLayout, weight float64) {
	// Room for the ink to grow into, and for the descenders the line box does
	// not reserve -- the headline is upper-cased, but a comma still drops.
	pad := int(math.Ceil(weight)) + 2
	band := image.Rect(0, h.top-pad, frameWidth, h.top+h.height()+h.size/4+pad).
		Intersect(canvas.Bounds())
	if band.Empty() {
		return
	}

	for _, demoted := range []bool{false, true} {
		col, ink, rest := image.Image(headlineColor), maskInk, maskEmpty
		if demoted {
			if !slices.ContainsFunc(h.dim, func(line []bool) bool { return slices.Contains(line, true) }) {
				continue // nothing is demoted, so there is no second mask
			}
			col, ink, rest = headlineMinorColor, maskEmpty, maskInk
		}
		// Bounded to the band, so the scratch is a strip and not a second frame.
		scratch := image.NewRGBA(band)
		for i, line := range h.lines {
			w := measure(h.face, line, h.tracking).Ceil()
			x := (frameWidth - w) / 2
			baseline := h.top + i*h.lineH + h.size
			drawString(scratch, h.face, line, x, baseline, h.tracking, wordInk(line, h.dim[i], ink, rest))
		}
		draw.DrawMask(canvas, band, col, image.Point{}, dilate(scratch, weight), band.Min, draw.Over)
	}
}

// dilate grows a mask's ink by weight pixels: each pixel takes the strongest
// alpha within that radius of it.
//
// The kernel is round, because a square one thickens the diagonals by half as
// much again as the uprights and the letters come out lumpy. Its rim is
// feathered -- a tap at the edge of the radius contributes only the fraction of
// it that falls inside -- which is what makes weight a dial rather than a set
// of steps: at 0.5 a neighbour carries half its ink, so the stroke thickens
// smoothly instead of jumping a whole pixel when the radius crosses one.
func dilate(src *image.RGBA, weight float64) *image.Alpha {
	bounds := src.Bounds()
	out := image.NewAlpha(bounds)

	// The kernel is the same at every pixel, so it is built once rather than
	// per-pixel, and the taps that contribute nothing are dropped here instead
	// of tested a hundred thousand times below.
	type tap struct {
		dx, dy int
		part   float64
	}
	reach := int(math.Ceil(weight))
	taps := make([]tap, 0, (2*reach+1)*(2*reach+1))
	for dy := -reach; dy <= reach; dy++ {
		for dx := -reach; dx <= reach; dx++ {
			part := min(max(weight-math.Hypot(float64(dx), float64(dy))+0.5, 0), 1)
			if part > 0 {
				taps = append(taps, tap{dx: dx, dy: dy, part: part})
			}
		}
	}

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			var strongest float64
			for _, t := range taps {
				px, py := x+t.dx, y+t.dy
				if px < bounds.Min.X || px >= bounds.Max.X || py < bounds.Min.Y || py >= bounds.Max.Y {
					continue
				}
				if a := float64(src.Pix[src.PixOffset(px, py)+3]) * t.part; a > strongest {
					strongest = a
				}
			}
			out.SetAlpha(x, y, color.Alpha{A: uint8(strongest)}) //nolint:gosec // 0..255
		}
	}
	return out
}

// drawTile paints one cell: a rounded plate, its border, and the icon over
// them with its own background keyed away.
func drawTile(canvas *image.RGBA, box image.Rectangle, icon image.Image) {
	radius := min(tileCornerRadius, min(box.Dx(), box.Dy())/2)
	paintPlate(canvas, box, radius)

	inset := tileBorderWidth + tileIconPadding
	inner := box.Inset(inset)
	if inner.Dx() <= 0 || inner.Dy() <= 0 {
		return
	}
	// Into a buffer rather than straight onto the canvas: the keying needs the
	// icon's own pixels before they are composited. CatmullRom because these are
	// line art, and aliased strokes make a thumbnail look cheap.
	scaled := image.NewRGBA(image.Rect(0, 0, inner.Dx(), inner.Dy()))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), icon, icon.Bounds(), draw.Src, nil)
	keyOutBackground(scaled)

	// Masked to the same rounding, or a square icon pokes out through the
	// tile's corners at any radius wider than its padding.
	mask := roundedMask(scaled.Bounds(), max(radius-inset, 0))
	draw.DrawMask(canvas, inner, scaled, image.Point{}, mask, image.Point{}, draw.Over)
}

// paintPlate fills the tile and strokes its border in one pass: the border is
// opaque and the plate is not, so filling first would tint the plate.
func paintPlate(canvas *image.RGBA, box image.Rectangle, radius int) {
	inner := box.Inset(tileBorderWidth)
	innerRadius := max(radius-tileBorderWidth, 0)

	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			outside := coverage(x, y, box, radius)
			if outside <= 0 {
				continue
			}
			within := coverage(x, y, inner, innerRadius)
			if ring := outside - within; ring > 0 {
				blend(canvas, x, y, tileBorderColor, ring)
			}
			if within > 0 {
				blend(canvas, x, y, tileFillColor, within)
			}
		}
	}
}

// coverage is how much of the pixel at (x, y) falls inside a rounded rectangle,
// from 0 to 1: the signed distance to the edge, read across the half pixel
// either side of it, which is what makes the corners an arc not a staircase.
func coverage(x, y int, box image.Rectangle, radius int) float64 {
	px, py := float64(x)+0.5, float64(y)+0.5
	cx := float64(box.Min.X+box.Max.X) / 2
	cy := float64(box.Min.Y+box.Max.Y) / 2

	// Distance from the centre, measured to the box inset by its own corner
	// radius: inside that inset rectangle the corners play no part.
	qx := math.Abs(px-cx) - (float64(box.Dx())/2 - float64(radius))
	qy := math.Abs(py-cy) - (float64(box.Dy())/2 - float64(radius))
	d := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) +
		math.Min(math.Max(qx, qy), 0) - float64(radius)

	return math.Min(math.Max(0.5-d, 0), 1)
}

// roundedMask is the same shape as an alpha channel, for compositing something
// else through it.
func roundedMask(bounds image.Rectangle, radius int) *image.Alpha {
	m := image.NewAlpha(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			a := coverage(x, y, bounds, radius) * 255
			m.SetAlpha(x, y, color.Alpha{A: uint8(a)}) //nolint:gosec // 0..255
		}
	}
	return m
}

// blend lays a colour over one pixel at a coverage, honouring the colour's own
// alpha. The canvas is opaque throughout, so there is no destination alpha to
// carry.
func blend(canvas *image.RGBA, x, y int, c color.RGBA, cov float64) {
	a := cov * float64(c.A) / 255
	if a <= 0 {
		return
	}
	was := canvas.RGBAAt(x, y)
	canvas.SetRGBA(x, y, color.RGBA{
		R: uint8(float64(c.R)*a + float64(was.R)*(1-a)), //nolint:gosec // 0..255
		G: uint8(float64(c.G)*a + float64(was.G)*(1-a)), //nolint:gosec // 0..255
		B: uint8(float64(c.B)*a + float64(was.B)*(1-a)), //nolint:gosec // 0..255
		A: 255,
	})
}

// keyOutBackground turns an icon's dark field into transparency, in place.
// Alpha comes from luminance through the ramp above, and the colours are
// re-premultiplied against it: image/draw works in premultiplied alpha, and
// the originals would leave a grey haze where the background was.
func keyOutBackground(img *image.RGBA) {
	for i := 0; i < len(img.Pix); i += 4 {
		r, g, b := int(img.Pix[i]), int(img.Pix[i+1]), int(img.Pix[i+2])
		// Rec. 601 luma: the eye reads green as most of an image's brightness,
		// and a flat average would key blue artwork differently from red.
		lum := (299*r + 587*g + 114*b) / 1000

		var alpha int
		switch {
		case lum <= iconTransparentBelow:
			alpha = 0
		case lum >= iconOpaqueAbove:
			alpha = 255
		default:
			alpha = (lum - iconTransparentBelow) * 255 / (iconOpaqueAbove - iconTransparentBelow)
		}

		img.Pix[i] = uint8(r * alpha / 255)   //nolint:gosec // alpha is 0..255
		img.Pix[i+1] = uint8(g * alpha / 255) //nolint:gosec // alpha is 0..255
		img.Pix[i+2] = uint8(b * alpha / 255) //nolint:gosec // alpha is 0..255
		img.Pix[i+3] = uint8(alpha)           //nolint:gosec // alpha is 0..255
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
	drawString(canvas, face, text, x, baseline, fixed.Int26_6(0), solid(captionColor))
}
