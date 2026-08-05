package thumbnail

import (
	"image"
	"image/color"
)

// Every tunable of the thumbnail's look, in one file.
//
// Nothing here is logic: it is the design expressed as numbers, so it can be
// changed by editing this file rather than by reading the drawing code. Each
// entry says what raising it does.
//
// They are constants rather than settings rows on purpose. The backend that
// exists to be re-styled without a rebuild is the browser one slide to come;
// a settings table full of pixel values would be a second place to change a
// layout and a first place to get it wrong.

// ------------------------------------------------------------------ frame ---

const (
	// YouTube's thumbnail size.
	frameWidth  = 1280
	frameHeight = 720

	// backgroundBrightness is how much of the backdrop photograph survives, out
	// of 255. Lower is darker: white type over an undimmed photograph is
	// unreadable, and the reference thumbnails are nearly black behind it.
	backgroundBrightness = 220
)

// ------------------------------------------------------------------- grid ---

// The grid is sized from the frame width first and the headline takes what is
// left over, which is what keeps the tiles running edge to edge.
const (
	// gridSideMargin is the gutter on each side of the grid. Lower it for a
	// wider grid; zero puts the tiles hard against the frame.
	gridSideMargin = 24
	// gridBottomMargin is the room under the last row of captions.
	gridBottomMargin = 14
	// headlineToGridGap separates the headline from the first row of tiles.
	headlineToGridGap = 18
	// tileSpacing is the gap between tiles, across and down.
	tileSpacing = 28
)

// ------------------------------------------------------------------- tile ---

const (
	// tileBorderWidth is the frame drawn around each tile. Zero for none.
	tileBorderWidth = 3
	// tileCornerRadius rounds the tile's corners, the border and the icon inside
	// it together. Zero is square. It is clamped to half the tile, so a large
	// value simply rounds a tile into a circle rather than misdrawing it.
	tileCornerRadius = 10
	// tileIconPadding is the inset from the border to the icon. Raise it to give
	// the artwork more air.
	tileIconPadding = 8
)

// ---------------------------------------------------------------- caption ---

// The caption type size is chosen once for the whole grid — the largest between
// the bounds at which every caption fits its tile. Set the two equal to pin the
// size and let long captions be cut instead.
const (
	// tileToCaptionGap is the space between a tile and the caption under it.
	tileToCaptionGap = 8
	captionFontMax   = 26
	captionFontMin   = 12
	// captionFontStep is how finely the fitting walks between those two.
	captionFontStep = 1
	// captionRowHeight is the height reserved under every tile whatever size the
	// captions settle at. Reserving the same band for all of them is what keeps
	// the rows aligned, so it wants to be a little over captionFontMax.
	captionRowHeight = captionFontMax + 2
)

// --------------------------------------------------------------- headline ---

// The headline is fitted into whatever height the grid left above it, at the
// largest size between the bounds that fits on one line. It wraps only when
// even the floor will not hold it.
//
// A fuller grid leaves the headline less room: the default twelve tiles in two
// rows of six sit near the middle of this range.
const (
	headlineSideMargin = 40
	headlineTopMargin  = 22
	headlineFontMax    = 120
	headlineFontMin    = 44
	// headlineFontStep is how finely the fitting walks between those two.
	headlineFontStep = 4
	// headlineLineGap is the space between wrapped lines.
	headlineLineGap = 6
	// headlineMaxLines is the most lines a headline may take before it is drawn
	// at the floor and allowed to overflow.
	headlineMaxLines = 2
	// headlineTracking is the letter-spacing, as a divisor of the type size:
	// spacing = size / headlineTracking. Lower is looser.
	headlineTracking = 22
)

// ------------------------------------------------------------------- icon ---

// The icons arrive as artwork on a solid dark field — that is what an image
// model returns when asked for line art, and what the samples are. Drawn as-is
// they would stamp that field over the tile, so the darkness becomes
// transparency instead. The ramp between the two thresholds is what stops the
// artwork's anti-aliased edges from turning into a hard jagged cut.
const (
	// iconTransparentBelow: luminance at or under this disappears entirely.
	// Raise it to eat a background that is not quite black.
	iconTransparentBelow = 48
	// iconOpaqueAbove: luminance at or over this is kept as-is. Lower it to keep
	// more of the artwork's darker shading.
	iconOpaqueAbove = 105
)

// ---------------------------------------------------------------- palette ---

// Colours are vars only because Go has no constant structs; treat them the same
// way. Alpha is honoured everywhere: a tileFillColor of A: 0 lets the backdrop
// show through the tiles entirely, and A: 255 makes them solid plates.
var (
	// headlineColor is the hook across the top; captionColor the labels under
	// each tile.
	headlineColor = image.NewUniform(color.RGBA{R: 246, G: 246, B: 244, A: 255})
	captionColor  = image.NewUniform(color.RGBA{R: 226, G: 226, B: 222, A: 255})

	// tileFillColor is the plate behind each icon. tileBorderColor is a hair off
	// pure white: at 255 it out-shouts the artwork it frames.
	tileFillColor   = color.RGBA{R: 6, G: 6, B: 8, A: 235}
	tileBorderColor = color.RGBA{R: 228, G: 228, B: 224, A: 255}
)

// -------------------------------------------------------------- resources ---

const (
	// backgroundFileName is read from the resources directory; defaultFontFile
	// from its fonts subdirectory. The font is also a settings row, so this is
	// only the fallback.
	backgroundFileName = "background.jpg"
	defaultFontFile    = "CabinSketch-Bold.ttf"
	// defaultGridRows applies when the settings row is unreadable.
	defaultGridRows = 2
)
