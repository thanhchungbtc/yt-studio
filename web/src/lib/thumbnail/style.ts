/**
 * The builtin renderer's look, as numbers.
 *
 * A deliberate port of `adapters/provider/thumbnail/style.go`, so the editor
 * opens on what the server would have produced rather than on a blank frame.
 * It is the *starting state*, not a contract: once a document exists the
 * operator moves things freely and none of this is consulted again. That is
 * what makes the duplication safe — a drift here costs a slightly different
 * seed, not a thumbnail that renders two ways.
 */
export const STYLE = {
  /** How much of the backdrop survives, out of 255. Lower is darker. */
  scrim: 220,

  gridSideMargin: 24,
  gridBottomMargin: 14,
  headlineToGridGap: 18,
  tileSpacing: 28,

  tileBorderWidth: 3,
  tileCornerRadius: 10,
  tileIconPadding: 8,

  tileToCaptionGap: 8,
  captionFontMax: 26,
  captionFontMin: 12,
  /** Reserved under every tile whatever size the captions settle at, which is
   *  what keeps the rows aligned. A little over captionFontMax. */
  captionRowHeight: 28,

  headlineSideMargin: 40,
  headlineTopMargin: 22,
  headlineFontMax: 120,
  headlineFontMin: 44,
  headlineFontStep: 4,
  headlineLineGap: 6,
  headlineMaxLines: 2,
  /** Letter-spacing as a divisor of the type size: spacing = size / this. */
  headlineTracking: 22,

  /** Luminance at or under this disappears; at or over the other is kept. */
  iconKeyBelow: 48,
  iconKeyAbove: 105,

  headlineColor: '#f6f6f4',
  captionColor: '#e2e2de',
  /** Alpha 235/255: the backdrop shows faintly through each plate. */
  tileFillColor: 'rgba(6, 6, 8, 0.922)',
  /** A hair off pure white; at 255 it out-shouts the artwork it frames. */
  tileBorderColor: '#e4e4e0',
} as const

/** The tracking the Go renderer uses at a given headline size, in pixels. */
export function trackingFor(size: number): number {
  return Math.max(Math.floor(size / STYLE.headlineTracking), 1)
}
