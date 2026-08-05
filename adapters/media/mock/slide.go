package mock

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strconv"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// Mock slide dimensions. Small enough that fifty chapters' worth costs a few
// megabytes, large enough to be a real image in the UI's chapter grid.
const (
	imageWidth  = 320
	imageHeight = 180
)

// Slide is the mock slide backend. Output is a real PNG encoded by the standard
// library, derived deterministically from the prompt.
type Slide struct {
	store provider.AssetStore
}

var _ provider.SlideProvider = (*Slide)(nil)

// NewSlide constructs the mock.
func NewSlide(store provider.AssetStore) *Slide {
	return &Slide{store: store}
}

// Generate produces exactly one slide.
func (i *Slide) Generate(ctx context.Context, req provider.SlideRequest) (entity.AssetID, error) {
	seed := seedOf(string(req.VideoID), strconv.Itoa(req.Ordinal), strconv.Itoa(req.Index), req.Prompt)
	img := renderStill(seed, imageWidth, imageHeight)

	var buf bytes.Buffer
	buf.Grow(imageWidth * imageHeight / 4)
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return "", fmt.Errorf("encode slide: %w", err)
	}
	stored, err := i.store.Put(ctx, entity.AssetKindImage, &buf)
	if err != nil {
		return "", fmt.Errorf("store slide: %w", err)
	}
	return stored.ID, nil
}

// renderStill paints a deterministic landscape: a two-stop sky gradient, a
// horizon, a ridge line and a light source. It is recognisably an image rather
// than noise, which makes the chapter grid in the UI genuinely reviewable.
//
// The size is a parameter because a thumbnail is the same picture at YouTube's
// dimensions rather than a second painter.
func renderStill(seed uint64, width, height int) *image.RGBA {
	r := deterministic(seed)
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	skyTop := color.RGBA{
		R: uint8(20 + r.IntN(60)),  //nolint:gosec // bounded
		G: uint8(30 + r.IntN(70)),  //nolint:gosec // bounded
		B: uint8(60 + r.IntN(120)), //nolint:gosec // bounded
		A: 255,
	}
	skyBottom := color.RGBA{
		R: uint8(140 + r.IntN(100)), //nolint:gosec // bounded
		G: uint8(110 + r.IntN(110)), //nolint:gosec // bounded
		B: uint8(90 + r.IntN(120)),  //nolint:gosec // bounded
		A: 255,
	}
	ground := color.RGBA{
		R: uint8(18 + r.IntN(40)), //nolint:gosec // bounded
		G: uint8(20 + r.IntN(45)), //nolint:gosec // bounded
		B: uint8(24 + r.IntN(50)), //nolint:gosec // bounded
		A: 255,
	}

	scaleTo := func(v int) int { return max(v*height/imageHeight, 1) }
	horizon := height/2 + r.IntN(height/5)
	ridgeAmp := scaleTo(6 + r.IntN(14))
	ridgeFreq := 1.0 + float64(r.IntN(4))
	ridgePhase := r.Float64() * math.Pi * 2
	sunX := r.IntN(width)
	sunY := r.IntN(horizon)
	sunR := scaleTo(8 + r.IntN(14))

	for y := range height {
		for x := range width {
			ridge := horizon - int(float64(ridgeAmp)*math.Sin(ridgeFreq*float64(x)/float64(width)*math.Pi*2+ridgePhase))
			var c color.RGBA
			if y >= ridge {
				shade := 1 - float64(y-ridge)/float64(height)*0.5
				c = scale(ground, shade)
			} else {
				t := float64(y) / float64(ridge)
				c = lerp(skyTop, skyBottom, t)
				if dx, dy := x-sunX, y-sunY; dx*dx+dy*dy <= sunR*sunR {
					c = blend(c, color.RGBA{R: 255, G: 244, B: 214, A: 255}, 0.85)
				}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func lerp(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t), //nolint:gosec // t is 0..1
		G: uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t), //nolint:gosec // t is 0..1
		B: uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t), //nolint:gosec // t is 0..1
		A: 255,
	}
}

func blend(base, top color.RGBA, alpha float64) color.RGBA {
	return lerp(base, top, alpha)
}

func scale(c color.RGBA, f float64) color.RGBA {
	clamp := func(v float64) uint8 {
		switch {
		case v < 0:
			return 0
		case v > 255:
			return 255
		default:
			return uint8(v)
		}
	}
	return color.RGBA{R: clamp(float64(c.R) * f), G: clamp(float64(c.G) * f), B: clamp(float64(c.B) * f), A: 255}
}
