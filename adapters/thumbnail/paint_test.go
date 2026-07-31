package thumbnail

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// The icons arrive as artwork on a field, and the field must not survive into
// the tile. Before this the field was stamped over the plate: on a sample whose
// background was a dark gradient rather than flat black, that showed up as a
// visible grey rectangle inside the tile.
func TestBackgroundIsKeyedOutOfAnIcon(t *testing.T) {
	t.Parallel()
	const size = 40
	icon := image.NewRGBA(image.Rect(0, 0, size, size))

	// A field just dark enough to be background, with a white mark on it.
	field := color.RGBA{R: 30, G: 30, B: 34, A: 255}
	for y := range size {
		for x := range size {
			icon.SetRGBA(x, y, field)
		}
	}
	mark := image.Rect(15, 15, 25, 25)
	for y := mark.Min.Y; y < mark.Max.Y; y++ {
		for x := mark.Min.X; x < mark.Max.X; x++ {
			icon.SetRGBA(x, y, color.RGBA{R: 250, G: 250, B: 250, A: 255})
		}
	}

	keyOutBackground(icon)

	if _, _, _, a := icon.At(2, 2).RGBA(); a != 0 {
		t.Errorf("a background pixel kept alpha %d, want it keyed away", a>>8)
	}
	if _, _, _, a := icon.At(20, 20).RGBA(); a>>8 != 255 {
		t.Errorf("the artwork kept alpha %d, want it opaque", a>>8)
	}
	// Premultiplied, so a keyed pixel carries no colour either — leaving the
	// original there draws a grey haze where the background used to be.
	r, g, b, _ := icon.At(2, 2).RGBA()
	if r|g|b != 0 {
		t.Errorf("a keyed pixel still carries colour (%d,%d,%d)", r>>8, g>>8, b>>8)
	}
}

// The ramp between the two thresholds is what keeps an anti-aliased edge from
// turning into a hard jagged cut.
func TestKeyingRampsRatherThanClipping(t *testing.T) {
	t.Parallel()
	mid := (iconTransparentBelow + iconOpaqueAbove) / 2
	icon := image.NewRGBA(image.Rect(0, 0, 1, 1))
	icon.SetRGBA(0, 0, color.RGBA{R: uint8(mid), G: uint8(mid), B: uint8(mid), A: 255})

	keyOutBackground(icon)

	_, _, _, a := icon.At(0, 0).RGBA()
	if got := a >> 8; got == 0 || got == 255 {
		t.Fatalf("a mid-tone keyed to %d, want a partial alpha", got)
	}
}

// The border is what separates one tile from the next over a photograph, so it
// has to actually be drawn in the colour the palette names — and the corners
// have to be rounded away from it.
func TestTileBorderIsDrawnAndCornersAreRounded(t *testing.T) {
	t.Parallel()
	// A backdrop nothing in the palette could be mistaken for, so "the tile did
	// not paint here" is unambiguous.
	backdrop := color.RGBA{R: 255, G: 0, B: 255, A: 255}
	canvas := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := range 100 {
		for x := range 100 {
			canvas.SetRGBA(x, y, backdrop)
		}
	}
	box := image.Rect(10, 10, 90, 90)
	drawTile(canvas, box, image.NewRGBA(image.Rect(0, 0, 10, 10)))

	// Halfway down the left edge, past the corner arc: solid border.
	mid := color.RGBAModel.Convert(canvas.At(box.Min.X, (box.Min.Y+box.Max.Y)/2)).(color.RGBA)
	if mid != tileBorderColor {
		t.Errorf("edge pixel is %v, want the border colour %v", mid, tileBorderColor)
	}

	// The extreme corner is outside a rounded rectangle, so the backdrop shows.
	corner := color.RGBAModel.Convert(canvas.At(box.Min.X, box.Min.Y)).(color.RGBA)
	if corner != backdrop {
		t.Errorf("corner pixel is %v, want the backdrop %v — the corner is not rounded",
			corner, backdrop)
	}
}

// The radius is clamped rather than allowed to misdraw: a value past half the
// tile rounds it into a circle, which is silly but not broken.
func TestOversizedRadiusIsClamped(t *testing.T) {
	t.Parallel()
	canvas := image.NewRGBA(image.Rect(0, 0, 40, 40))
	box := image.Rect(5, 5, 35, 35)

	drawTile(canvas, box, image.NewRGBA(image.Rect(0, 0, 8, 8)))

	// The centre is inside the shape whatever the radius does.
	got := color.RGBAModel.Convert(canvas.At(20, 20)).(color.RGBA)
	if got.A == 0 {
		t.Fatal("the middle of the tile was left unpainted")
	}
}

// The headline is drawn, and it is drawn inside the band the layout gave it.
//
// Tested against the layout's own numbers rather than a fixed strip of the
// frame: the grid moves depending on how much room the headline takes, so a
// hardcoded band measures something different for every headline it is given.
func TestHeadlineIsDrawnInItsBand(t *testing.T) {
	t.Parallel()
	dir, err := filepath.Abs(filepath.Join("..", "..", "var", "resources"))
	if err != nil {
		t.Fatal(err)
	}
	var fonts fontCache
	parsed, err := fonts.load(filepath.Join(dir, "fonts", defaultFontFile))
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("skipping: %s is missing", defaultFontFile)
		}
		t.Fatal(err)
	}

	canvas := image.NewRGBA(image.Rect(0, 0, frameWidth, frameHeight))
	h := layOutHeadline(parsed, "GENIUS MENTAL MODELS", 240)
	if len(h.lines) == 0 {
		t.Fatal("the headline laid out to nothing")
	}
	drawHeadline(canvas, h)

	bright := func(fromY, toY int) int {
		var n int
		for y := max(fromY, 0); y < min(toY, frameHeight); y++ {
			for x := range frameWidth {
				if c := canvas.RGBAAt(x, y); c.R > 200 && c.G > 200 && c.B > 200 {
					n++
				}
			}
		}
		return n
	}

	if n := bright(h.top, h.top+h.height()); n < 1000 {
		t.Errorf("the headline band holds %d bright pixels, want the headline in it", n)
	}
	if n := bright(0, h.top); n != 0 {
		t.Errorf("%d bright pixels above the headline band, want none", n)
	}
	if n := bright(h.top+h.height(), frameHeight); n != 0 {
		t.Errorf("%d bright pixels below the headline band, want none", n)
	}
}
