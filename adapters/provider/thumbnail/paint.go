package thumbnail

import (
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // the background is a JPEG; decoding it is why this is here
	"math"

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

// drawHeadline centres each line of the fitted headline.
func drawHeadline(canvas *image.RGBA, h headlineLayout) {
	if len(h.lines) == 0 {
		return
	}
	for i, line := range h.lines {
		w := measure(h.face, line, h.tracking).Ceil()
		x := (frameWidth - w) / 2
		// The baseline sits above the bottom of the line box, which is what keeps
		// descenders inside the block rather than in the gap below it.
		baseline := h.top + i*h.lineH + h.size
		drawString(canvas, h.face, line, x, baseline, h.tracking, headlineColor)
	}
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
	drawString(canvas, face, text, x, baseline, fixed.Int26_6(0), captionColor)
}
