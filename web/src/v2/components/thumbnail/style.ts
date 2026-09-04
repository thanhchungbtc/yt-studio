/**
 * Every tunable of the thumbnail's look, mirrored from the Go renderer, and
 * turned into a value the operator can change.
 *
 * `adapters/provider/thumbnail/style.go` is the original: a block of constants
 * with a note on each saying what raising it does. Its own header explains why
 * they are constants there — "the backend that exists to be restyled without a
 * rebuild is the HTML one still to come" — and this is that backend. So the
 * numbers are the same numbers, transcribed entry for entry, but they are
 * fields on a `Style` rather than compile-time constants.
 *
 * `defaultStyle` is exactly what the Go renderer uses. That matters more than
 * it looks: a thumbnail built here without touching a control composes the
 * image the pipeline would have composed, so the builder is a place to *check*
 * the render before it publishes and not only a place to depart from it. The
 * moment a control moves, the two diverge — which is legitimate, because an
 * override is by definition an image the renderer would not have made, but it
 * is a thing the operator should be told rather than left to discover.
 *
 * When style.go changes, this changes. There is no clever way to share them:
 * one is compiled into a binary and the other is shipped to a browser, and
 * generating this file would put a build step between a designer and a number.
 */

/* ------------------------------------------------------------------ frame */

/**
 * YouTube's thumbnail size, from `entity.ThumbnailWidth/Height`.
 *
 * Not on `Style`, because it is not a look: an image outside these is refused
 * at upload whatever drew it. That is why they live in the entity on the Go
 * side rather than among the renderer's style constants.
 */
export const frameWidth = 1280
export const frameHeight = 720

/**
 * How finely the fitting loops walk between their bounds.
 *
 * Also not on `Style`. These change how long the search takes and which of two
 * adjacent sizes it lands on, not what the design is, and a control for them
 * would be a control for the algorithm rather than for the picture.
 */
export const captionFontStep = 1
export const headlineFontStep = 4

/* ------------------------------------------------------------------ style */

export interface Style {
  /** A filename in the resources fonts directory. */
  font: string
  /** How many rows the grid is laid out in. */
  rows: number

  /* grid — sized from the frame width first; the headline takes what is left */

  /** The gutter each side of the grid. Zero puts tiles against the frame. */
  gridSideMargin: number
  /** The room under the last row of captions. */
  gridBottomMargin: number
  /** Separates the headline from the first row of tiles. */
  headlineToGridGap: number
  /** The gap between tiles, across and down. */
  tileSpacing: number

  /* tile */

  /** The frame drawn around each tile. Zero for none. */
  tileBorderWidth: number
  /** Rounds the tile, its border and its icon together. Zero is square. */
  tileCornerRadius: number
  /** The inset from the border to the icon. Raise it for more air. */
  tileIconPadding: number
  /** The plate behind each icon. */
  tileFillColor: string
  /** How opaque that plate is, out of 255. At 0 the backdrop shows through. */
  tileFillAlpha: number
  /** A hair off pure white: at 255 it out-shouts the artwork it frames. */
  tileBorderColor: string
  /**
   * How opaque that border is, out of 255.
   *
   * The Go renderer has no equivalent field: the alpha is carried by
   * `tileBorderColor` itself, which is declared there as an RGBA and blended by
   * the painter at whatever alpha it holds. So the machinery for a translucent
   * border was always in place and only the number was fixed. 204 is that
   * number — four fifths — which is why leaving this alone still composes what
   * the renderer composes.
   */
  tileBorderAlpha: number

  /* caption — one size for the whole grid, the largest at which all fit */

  /** The space between a tile and the caption under it. */
  tileToCaptionGap: number
  captionFontMax: number
  captionFontMin: number
  captionColor: string

  /* headline — fitted into whatever height the grid left above it */

  headlineSideMargin: number
  headlineTopMargin: number
  headlineFontMax: number
  headlineFontMin: number
  /** The space between wrapped lines. */
  headlineLineGap: number
  /** The most lines before it is drawn at the floor and allowed to overflow. */
  headlineMaxLines: number
  /** Letter-spacing as a divisor of the type size: spacing = size / this. */
  headlineTracking: number
  headlineColor: string
  /**
   * The headline's function words, drawn in `headlineMinorColor` rather than
   * at full white so the words that carry the hook read first.
   *
   * A string rather than a parsed list, because it mirrors a settings row that
   * an operator types into -- `thumbnail.headline.minor_words` -- and the
   * separators it forgives, commas or whitespace, are forgiven here too.
   * Empty draws the whole headline in `headlineColor`.
   */
  headlineMinorWords: string
  /**
   * A dimmer neutral rather than a second hue, which is the whole design: the
   * gap the eye resolves at browse size is a luminance gap, and a hue-only
   * demotion collapses at 320px.
   *
   * Tied to `headlineColor` and not readable alone — dragging that swatch to a
   * darker hue without bringing this one down leaves the demoted words brighter
   * than the ones they are demoted against. See `headlineMinorColor` in
   * style.go for the numbers.
   */
  headlineMinorColor: string

  /* image */

  /** How much of the backdrop survives, out of 255. Lower is darker. */
  backgroundBrightness: number
  /** Luminance at or under this disappears entirely from an icon. */
  iconTransparentBelow: number
  /** Luminance at or over this is kept as-is. */
  iconOpaqueAbove: number
}

/** Exactly what the Go renderer uses. See the note at the top of the file. */
export const defaultStyle: Style = {
  font: 'CabinSketch-Bold.ttf',
  rows: 2,

  gridSideMargin: 24,
  gridBottomMargin: 14,
  headlineToGridGap: 18,
  tileSpacing: 28,

  tileBorderWidth: 3,
  tileCornerRadius: 10,
  tileIconPadding: 8,
  tileFillColor: '#060608',
  tileFillAlpha: 235,
  tileBorderColor: '#e4e4e0',
  tileBorderAlpha: 204,

  tileToCaptionGap: 8,
  captionFontMax: 26,
  captionFontMin: 12,
  captionColor: '#e2e2de',

  headlineSideMargin: 40,
  headlineTopMargin: 22,
  headlineFontMax: 120,
  headlineFontMin: 44,
  headlineLineGap: 6,
  headlineMaxLines: 2,
  headlineTracking: 22,
  headlineColor: '#ff2600',
  // The seeded value of `thumbnail.headline.minor_words`, from
  // `entity.DefaultHeadlineMinorWords`.
  headlineMinorWords:
    'a,an,and,are,as,at,be,but,by,for,from,in,is,of,on,or,so,than,that,the,this,to,was,were,with',
  headlineMinorColor: '#585854',

  backgroundBrightness: 220,
  iconTransparentBelow: 48,
  iconOpaqueAbove: 105,
}

/**
 * The height reserved under every tile, whatever size the captions settle at.
 *
 * Derived rather than stored, exactly as in style.go where it is
 * `captionFontMax + 2`. Reserving the same band for all of them is what keeps
 * the rows aligned, so it wants to be a little over the ceiling.
 */
export function captionRowHeight(style: Style): number {
  return style.captionFontMax + 2
}

/** Whether anything has been moved off what the renderer would have used. */
export function isDefault(style: Style): boolean {
  return (Object.keys(defaultStyle) as (keyof Style)[]).every(
    (key) => style[key] === defaultStyle[key],
  )
}

/* -------------------------------------------------------------- resources */

export const backgroundURL = '/resources/background.jpg'

/** Where a typeface is served from, by the name the settings row carries. */
export function fontURL(file: string): string {
  return `/resources/fonts/${file}`
}

/* ---------------------------------------------------------------- colours */

/** `#rrggbb` and an alpha out of 255, as the channels the painter blends. */
export function rgba(hex: string, alpha: number): { r: number; g: number; b: number; a: number } {
  const value = hex.replace('#', '')
  return {
    r: parseInt(value.slice(0, 2), 16) || 0,
    g: parseInt(value.slice(2, 4), 16) || 0,
    b: parseInt(value.slice(4, 6), 16) || 0,
    a: alpha,
  }
}
