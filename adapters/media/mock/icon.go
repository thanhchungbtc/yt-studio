package mock

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strconv"

	"github.com/tbui/yt-studio/adapters/mockcore"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// Icon dimensions the mock falls back to when a caller asks for none.
const defaultIconSize = 256

// Icon is the mock thumbnail-icon backend. Output is white line art on black,
// which is the register the real grid is drawn in — a mock that returned the
// landscape slides use would make a wrong layout look right.
type Icon struct {
	store  provider.AssetStore
	tuning Tuning
}

var _ provider.ThumbnailIconGenerator = (*Icon)(nil)

// NewIcon constructs the mock.
func NewIcon(store provider.AssetStore, tuning Tuning) *Icon {
	return &Icon{store: store, tuning: tuning}
}

// Icon generates exactly one tile's icon.
func (i *Icon) Icon(ctx context.Context, req provider.ThumbnailIconRequest) (entity.AssetID, error) {
	if err := mockcore.Simulate(ctx, i.tuning, 2); err != nil {
		return "", err
	}
	if req.Prompt == "" {
		return "", errors.New("mock icon: an icon needs a prompt")
	}
	size := req.Size
	if size < 1 {
		size = defaultIconSize
	}

	// The index is deliberately out of the seed: two cells that asked for the
	// same thing are the same picture, and content addressing should say so.
	img := renderIcon(mockcore.SeedOf(req.Prompt, strconv.Itoa(size)), size)

	var buf bytes.Buffer
	buf.Grow(size * size / 8)
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return "", fmt.Errorf("encode icon: %w", err)
	}
	stored, err := i.store.Put(ctx, entity.AssetKindThumbnailIcon, &buf)
	if err != nil {
		return "", fmt.Errorf("store icon: %w", err)
	}
	return stored.ID, nil
}

// renderIcon draws a deterministic glyph: a ring, a few spokes and a bar, all
// in one stroke weight on black. It is not the subject the prompt asked for —
// no mock could be — but it is the right shape, weight and palette, so a grid
// of them shows whether the layout works.
func renderIcon(seed uint64, size int) *image.RGBA {
	r := mockcore.Deterministic(seed)
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	black := color.RGBA{A: 255}
	for y := range size {
		for x := range size {
			img.SetRGBA(x, y, black)
		}
	}

	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	stroke := max(size/32, 2)
	cx, cy := size/2, size/2
	radius := size/2 - size/6

	// A ring, always: it is what makes ten of these read as one set.
	drawRing(img, cx, cy, radius, stroke, white)

	// Spokes at deterministic angles, cut short of the ring so the weight stays
	// even where they meet it.
	spokes := 2 + r.IntN(4)
	phase := r.Float64() * math.Pi * 2
	for s := range spokes {
		angle := phase + float64(s)*2*math.Pi/float64(spokes)
		drawLine(img, cx, cy,
			cx+int(float64(radius)*0.72*math.Cos(angle)),
			cy+int(float64(radius)*0.72*math.Sin(angle)),
			stroke, white)
	}

	// A base bar, so the glyph has a bottom and the tiles do not read as ten
	// variations of a clock face.
	if r.IntN(2) == 0 {
		barY := cy + radius + stroke*2
		if barY < size-stroke {
			drawLine(img, cx-radius/2, barY, cx+radius/2, barY, stroke, white)
		}
	}
	return img
}

// drawRing strokes a circle by filling every pixel whose distance from the
// centre is within half a stroke of the radius.
func drawRing(img *image.RGBA, cx, cy, radius, stroke int, c color.RGBA) {
	inner := float64(radius) - float64(stroke)/2
	outer := float64(radius) + float64(stroke)/2
	bounds := img.Bounds()
	for y := max(cy-radius-stroke, 0); y < min(cy+radius+stroke, bounds.Dy()); y++ {
		for x := max(cx-radius-stroke, 0); x < min(cx+radius+stroke, bounds.Dx()); x++ {
			dx, dy := float64(x-cx), float64(y-cy)
			if d := math.Hypot(dx, dy); d >= inner && d <= outer {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

// drawLine walks the longer axis and stamps a square nib, which is enough for
// a glyph at this size and keeps the weight visually equal to the ring's.
func drawLine(img *image.RGBA, x0, y0, x1, y1, stroke int, c color.RGBA) {
	steps := max(abs(x1-x0), abs(y1-y0))
	if steps == 0 {
		steps = 1
	}
	half := stroke / 2
	bounds := img.Bounds()
	for i := 0; i <= steps; i++ {
		x := x0 + (x1-x0)*i/steps
		y := y0 + (y1-y0)*i/steps
		for ny := y - half; ny <= y+half; ny++ {
			for nx := x - half; nx <= x+half; nx++ {
				if nx >= 0 && ny >= 0 && nx < bounds.Dx() && ny < bounds.Dy() {
					img.SetRGBA(nx, ny, c)
				}
			}
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
