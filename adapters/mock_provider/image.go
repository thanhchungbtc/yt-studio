package mockprovider

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

// Mock still dimensions. Small enough that fifty chapters' worth costs a few
// megabytes, large enough to be a real image in the UI's chapter grid.
const (
	imageWidth  = 320
	imageHeight = 180
)

// Image is the mock still backend. Output is a real PNG encoded by the standard
// library, derived deterministically from the prompt.
type Image struct {
	store  provider.AssetStore
	tuning Tuning
}

var _ provider.ImageProvider = (*Image)(nil)

// NewImage constructs the mock.
func NewImage(store provider.AssetStore, tuning Tuning) *Image {
	return &Image{store: store, tuning: tuning}
}

// Generate produces exactly one still.
func (i *Image) Generate(ctx context.Context, req provider.ImageRequest) (entity.AssetID, error) {
	if err := simulate(ctx, i.tuning, 2); err != nil {
		return "", err
	}
	seed := seedOf(string(req.VideoID), strconv.Itoa(req.Ordinal), strconv.Itoa(req.Index), req.Prompt)
	img := renderStill(seed)

	var buf bytes.Buffer
	buf.Grow(imageWidth * imageHeight / 4)
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return "", fmt.Errorf("encode still: %w", err)
	}
	stored, err := i.store.Put(ctx, entity.AssetKindImage, &buf)
	if err != nil {
		return "", fmt.Errorf("store still: %w", err)
	}
	return stored.ID, nil
}

// renderStill paints a deterministic landscape: a two-stop sky gradient, a
// horizon, a ridge line and a light source. It is recognisably an image rather
// than noise, which makes the chapter grid in the UI genuinely reviewable.
func renderStill(seed uint64) *image.RGBA {
	r := deterministic(seed)
	img := image.NewRGBA(image.Rect(0, 0, imageWidth, imageHeight))

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

	horizon := imageHeight/2 + r.IntN(imageHeight/5)
	ridgeAmp := 6 + r.IntN(14)
	ridgeFreq := 1.0 + float64(r.IntN(4))
	ridgePhase := r.Float64() * math.Pi * 2
	sunX := r.IntN(imageWidth)
	sunY := r.IntN(horizon)
	sunR := 8 + r.IntN(14)

	for y := range imageHeight {
		for x := range imageWidth {
			ridge := horizon - int(float64(ridgeAmp)*math.Sin(ridgeFreq*float64(x)/imageWidth*math.Pi*2+ridgePhase))
			var c color.RGBA
			if y >= ridge {
				shade := 1 - float64(y-ridge)/float64(imageHeight)*0.5
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
