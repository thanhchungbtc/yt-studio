// Command icon draws the macOS application icon and writes an .icns.
//
// It is a generator rather than a checked-in export because the icon is
// geometry, not a photograph: every measurement below is a ratio of the canvas,
// so the 16-pixel version is drawn at 16 pixels rather than resampled down from
// 1024 and left muddy. Regenerating is `make icon`.
//
// Nothing here is imported by the application. It exists so the artwork has a
// source, the same way the UI does.
package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/image/vector"
)

// The macOS icon grid. Artwork occupies the middle of the canvas and the rest
// is transparent margin — a bundle whose art runs to the edge sits visibly
// larger in the Dock than every icon beside it.
const artworkFraction = 0.805

// squircleExponent shapes the rounded square. Apple's corner is a continuous
// curve rather than a circular arc, which a superellipse of this order
// reproduces. Calibrated by rendering it beside Notes.app and Music.app at the
// same size: below about six the tile reads as a blob next to them, because the
// corner never settles into a straight edge.
const squircleExponent = 6.4

// The palette, taken from the dark theme in web/src/styles.css so the icon and
// the interface are recognisably the same product. --accent is 212 96% 62% and
// --violet is 265 85% 72%; the gradient runs between deeper relatives of both,
// because a fill at interface lightness looks washed out at 1024 pixels.
var (
	gradientTop    = color.NRGBA{0x4C, 0x92, 0xFF, 0xFF} // lifted accent blue
	gradientBottom = color.NRGBA{0x6D, 0x3C, 0xE8, 0xFF} // deep violet
)

func main() {
	out := flag.String("out", "cmd/desktop/icon.icns", "where to write the .icns")
	preview := flag.String("preview", "", "also write a PNG contact sheet here")
	flag.Parse()

	if err := run(*out, *preview); err != nil {
		fmt.Fprintln(os.Stderr, "icon:", err)
		os.Exit(1)
	}
}

// iconsetSizes is what iconutil expects: each logical size at 1x and 2x.
var iconsetSizes = []struct {
	name string
	px   int
}{
	{"icon_16x16.png", 16},
	{"icon_16x16@2x.png", 32},
	{"icon_32x32.png", 32},
	{"icon_32x32@2x.png", 64},
	{"icon_128x128.png", 128},
	{"icon_128x128@2x.png", 256},
	{"icon_256x256.png", 256},
	{"icon_256x256@2x.png", 512},
	{"icon_512x512.png", 512},
	{"icon_512x512@2x.png", 1024},
}

func run(out, preview string) error {
	dir, err := os.MkdirTemp("", "yt-studio-iconset-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	iconset := filepath.Join(dir, "icon.iconset")
	if err := os.MkdirAll(iconset, 0o755); err != nil {
		return err
	}
	for _, s := range iconsetSizes {
		if err := writePNG(filepath.Join(iconset, s.name), draw2x(s.px)); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	//nolint:gosec // every path here is one this program chose
	cmd := exec.CommandContext(ctx, "iconutil", "--convert", "icns", "--output", out, iconset)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("iconutil: %w: %s", err, output)
	}

	if preview != "" {
		if err := writePNG(preview, contactSheet()); err != nil {
			return err
		}
	}
	fmt.Println(out)
	return nil
}

// draw2x renders at twice the requested size and averages down. The glyph has
// long near-vertical edges, and supersampling is what keeps them from crawling:
// the rasteriser antialiases a single path well, but several stacked
// translucent shapes accumulate seams along their shared borders.
func draw2x(size int) *image.NRGBA {
	big := drawIcon(size * 2)
	small := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a int
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					c := big.NRGBAAt(x*2+dx, y*2+dy)
					// Weight colour by coverage, or a transparent pixel's
					// arbitrary colour drags the average toward it.
					r += int(c.R) * int(c.A)
					g += int(c.G) * int(c.A)
					b += int(c.B) * int(c.A)
					a += int(c.A)
				}
			}
			if a == 0 {
				continue
			}
			small.SetNRGBA(x, y, color.NRGBA{
				R: uint8(r / a), G: uint8(g / a), B: uint8(b / a), A: uint8(a / 4),
			})
		}
	}
	return small
}

// drawIcon composes one icon at the given edge length.
func drawIcon(size int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	f := float64(size)

	// The artwork square, centred, leaving the Dock's margin.
	art := f * artworkFraction
	x0 := (f - art) / 2
	y0 := (f - art) / 2

	// --- the tile -----------------------------------------------------------
	body := squircle(size, x0, y0, art, art, squircleExponent)
	compositeGradient(dst, body, art, y0)

	// A light along the top edge and shadow along the bottom, both inside the
	// tile: it is the cheapest way to read as a physical object rather than a
	// flat colour, and it survives being shrunk to 16 pixels where a drop
	// shadow would not.
	inset := art * 0.012
	rim := squircle(size, x0+inset, y0+inset, art-2*inset, art-2*inset, squircleExponent)
	compositeEdge(dst, body, rim, y0, art)

	// --- the glyph ----------------------------------------------------------
	drawGlyph(dst, size, x0, y0, art)
	return dst
}

// drawGlyph puts a play triangle above a short chapter rule.
//
// One shape and one mark, because the icon has to survive being drawn at
// sixteen pixels in a menu bar and a stack of cards does not: at that size
// every translucent edge collapses into the one below it and the result is a
// grey smudge. What is left is a play triangle — unmistakable at any size — and
// beneath it a rule broken into three segments, which is the only thing that
// says the video being played is assembled out of chapters.
//
// The segments are unequal on purpose. Three equal ticks read as a progress bar
// or a hamburger menu; unequal ones read as a timeline, which is what they are.
func drawGlyph(dst *image.NRGBA, size int, x0, y0, art float64) {
	radius := art * 0.292
	// The triangle's centroid is its optical centre, and sits at the same point
	// as the geometry's. Nudged a little left of the tile's middle because the
	// shape's bounding box runs further right than left, and the eye reads the
	// box before it reads the mass.
	cx := x0 + art/2 - radius*0.06
	cy := y0 + art*0.500

	tri := playTriangle(size, cx, cy, radius)

	// One shape, and nothing else on the tile.
	//
	// Two other marks were tried here and both failed for the same reason. A
	// stack of cards behind the triangle read as a folder at 512 pixels and as
	// a smudge at 32. Slicing the triangle into chapter bands looked deliberate
	// at 1024 and turned it into a paper aeroplane everywhere else, because a
	// horizontal cut through a triangle pointing right leaves a long dart in
	// the middle that no longer reads as a play button.
	//
	// What survives every size is the silhouette everyone already knows. The
	// tile carries the character instead, which is the arrangement most of the
	// icons this one will sit beside in the Dock have settled on too.

	// Pure white at the top easing to a faintly cooled white at the bottom.
	// Flat white on a saturated tile looks like a sticker; a few points of
	// shading sits it in the light the tile's top edge already establishes.
	compositeVertical(dst, tri,
		color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF},
		color.NRGBA{0xF0, 0xF4, 0xFE, 0xFF},
		cy-radius*0.866, radius*1.732)
}

// squircle rasterises a superellipse: |x/a|^n + |y/b|^n = 1.
func squircle(size int, x, y, w, h, n float64) *image.Alpha {
	r := vector.NewRasterizer(size, size)
	a, b := w/2, h/2
	cx, cy := x+a, y+b

	const steps = 720
	for i := 0; i <= steps; i++ {
		t := 2 * math.Pi * float64(i) / steps
		// The signed superellipse parameterisation: cos/sin raised to 2/n,
		// keeping the sign so all four quadrants are drawn.
		ct, st := math.Cos(t), math.Sin(t)
		px := cx + a*math.Copysign(math.Pow(math.Abs(ct), 2/n), ct)
		py := cy + b*math.Copysign(math.Pow(math.Abs(st), 2/n), st)
		if i == 0 {
			r.MoveTo(float32(px), float32(py))
			continue
		}
		r.LineTo(float32(px), float32(py))
	}
	r.ClosePath()
	return mask(r, size)
}

// playTriangle rasterises an equilateral triangle pointing right, with its
// corners rounded. Rounded because a needle-sharp vertex is the one detail that
// always looks broken when it lands between two pixels.
func playTriangle(size int, cx, cy, radius float64) *image.Alpha {
	// Optical centring: a triangle's centre of area sits behind its point, so
	// the shape is nudged right to look centred rather than measure centred.
	cx += radius * 0.10

	// Vertices at 0, 120 and 240 degrees from the positive x axis, which puts
	// one of them on the right.
	pts := [3][2]float64{}
	for i := 0; i < 3; i++ {
		t := 2 * math.Pi * float64(i) / 3
		pts[i] = [2]float64{cx + radius*math.Cos(t), cy + radius*math.Sin(t)}
	}

	round := radius * 0.20
	r := vector.NewRasterizer(size, size)
	for i := 0; i < 3; i++ {
		prev := pts[(i+2)%3]
		cur := pts[i]
		next := pts[(i+1)%3]

		in := normalize(prev[0]-cur[0], prev[1]-cur[1])
		out := normalize(next[0]-cur[0], next[1]-cur[1])

		start := [2]float64{cur[0] + in[0]*round, cur[1] + in[1]*round}
		end := [2]float64{cur[0] + out[0]*round, cur[1] + out[1]*round}

		if i == 0 {
			r.MoveTo(float32(start[0]), float32(start[1]))
		} else {
			r.LineTo(float32(start[0]), float32(start[1]))
		}
		// The vertex itself is the control point, which is what turns the
		// corner into a curve rather than a bevel.
		r.QuadTo(float32(cur[0]), float32(cur[1]), float32(end[0]), float32(end[1]))
	}
	r.ClosePath()
	return mask(r, size)
}

func normalize(x, y float64) [2]float64 {
	d := math.Hypot(x, y)
	if d == 0 {
		return [2]float64{0, 0}
	}
	return [2]float64{x / d, y / d}
}

// mask draws the rasterised path into an alpha coverage mask.
func mask(r *vector.Rasterizer, size int) *image.Alpha {
	m := image.NewAlpha(image.Rect(0, 0, size, size))
	r.Draw(m, m.Bounds(), image.Opaque, image.Point{})
	return m
}

// compositeGradient fills a mask with the vertical brand gradient.
func compositeGradient(dst *image.NRGBA, m *image.Alpha, height, top float64) {
	b := dst.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		t := (float64(y) - top) / height
		t = clamp(t)
		// Eased rather than linear, so the middle of the tile holds one colour
		// and the shift happens near the edges where it reads as light.
		t = t * t * (3 - 2*t)
		row := color.NRGBA{
			R: lerp(gradientTop.R, gradientBottom.R, t),
			G: lerp(gradientTop.G, gradientBottom.G, t),
			B: lerp(gradientTop.B, gradientBottom.B, t),
			A: 0xFF,
		}
		for x := b.Min.X; x < b.Max.X; x++ {
			if a := m.AlphaAt(x, y).A; a > 0 {
				blend(dst, x, y, row, a)
			}
		}
	}
}

// compositeEdge lights the top edge and darkens the bottom, using the ring
// between the tile and an inset copy of it.
func compositeEdge(dst *image.NRGBA, body, inner *image.Alpha, top, height float64) {
	b := dst.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		t := clamp((float64(y) - top) / height)
		for x := b.Min.X; x < b.Max.X; x++ {
			ring := int(body.AlphaAt(x, y).A) - int(inner.AlphaAt(x, y).A)
			if ring <= 0 {
				continue
			}
			// Full strength at the very top and bottom, nothing across the
			// middle, so the sides do not get a bright outline.
			if t < 0.5 {
				strength := (0.5 - t) * 2
				blend(dst, x, y, color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF},
					uint8(float64(ring)*strength*0.22))
			} else {
				strength := (t - 0.5) * 2
				blend(dst, x, y, color.NRGBA{0x1A, 0x10, 0x40, 0xFF},
					uint8(float64(ring)*strength*0.20))
			}
		}
	}
}

// compositeVertical fills a mask with a vertical gradient between two colours.
func compositeVertical(dst *image.NRGBA, m *image.Alpha, from, to color.NRGBA, top, height float64) {
	b := dst.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		t := clamp((float64(y) - top) / height)
		c := color.NRGBA{
			R: lerp(from.R, to.R, t), G: lerp(from.G, to.G, t), B: lerp(from.B, to.B, t), A: 0xFF,
		}
		for x := b.Min.X; x < b.Max.X; x++ {
			if a := m.AlphaAt(x, y).A; a > 0 {
				blend(dst, x, y, c, a)
			}
		}
	}
}

// blend puts src over dst at the given coverage, in non-premultiplied space.
func blend(dst *image.NRGBA, x, y int, src color.NRGBA, alpha uint8) {
	if alpha == 0 {
		return
	}
	d := dst.NRGBAAt(x, y)
	sa := float64(alpha) / 255
	da := float64(d.A) / 255
	outA := sa + da*(1-sa)
	if outA == 0 {
		dst.SetNRGBA(x, y, color.NRGBA{})
		return
	}
	mix := func(s, dv uint8) uint8 {
		return uint8((float64(s)*sa + float64(dv)*da*(1-sa)) / outA)
	}
	dst.SetNRGBA(x, y, color.NRGBA{
		R: mix(src.R, d.R), G: mix(src.G, d.G), B: mix(src.B, d.B),
		A: uint8(outA * 255),
	})
}

func lerp(a, b uint8, t float64) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*t) }

func clamp(t float64) float64 {
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	return t
}

// contactSheet lays the sizes out on one canvas, so a change can be judged at
// the size it will be seen rather than only at 1024.
func contactSheet() *image.NRGBA {
	sizes := []int{512, 256, 128, 64, 32, 16}
	const pad = 24
	width := pad
	for _, s := range sizes {
		width += s + pad
	}
	sheet := image.NewNRGBA(image.Rect(0, 0, width, 512+2*pad))
	draw.Draw(sheet, sheet.Bounds(), &image.Uniform{color.NRGBA{0x14, 0x16, 0x1C, 0xFF}},
		image.Point{}, draw.Src)

	x := pad
	for _, s := range sizes {
		icon := draw2x(s)
		at := image.Rect(x, pad+(512-s)/2, x+s, pad+(512-s)/2+s)
		draw.Draw(sheet, at, icon, image.Point{}, draw.Over)
		x += s + pad
	}
	return sheet
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path) //nolint:gosec // paths are ours
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		return err
	}
	return f.Close()
}
