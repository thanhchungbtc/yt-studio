package thumbnail

import (
	"image"
	"image/color"
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
	mid := (iconKeyLow + iconKeyHigh) / 2
	icon := image.NewRGBA(image.Rect(0, 0, 1, 1))
	icon.SetRGBA(0, 0, color.RGBA{R: uint8(mid), G: uint8(mid), B: uint8(mid), A: 255})

	keyOutBackground(icon)

	_, _, _, a := icon.At(0, 0).RGBA()
	if got := a >> 8; got == 0 || got == 255 {
		t.Fatalf("a mid-tone keyed to %d, want a partial alpha", got)
	}
}

// The border is what separates one tile from the next over a photograph, so it
// has to actually be drawn in the colour the palette names.
func TestTileBorderIsDrawn(t *testing.T) {
	t.Parallel()
	canvas := image.NewRGBA(image.Rect(0, 0, 100, 100))
	icon := image.NewRGBA(image.Rect(0, 0, 10, 10))
	box := image.Rect(10, 10, 90, 90)

	drawTile(canvas, box, icon)

	got := color.RGBAModel.Convert(canvas.At(box.Min.X, box.Min.Y+20)).(color.RGBA)
	if got != tileBorderColor {
		t.Fatalf("border pixel is %v, want %v", got, tileBorderColor)
	}
}
