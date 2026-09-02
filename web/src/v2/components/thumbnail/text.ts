import { captionFontStep, frameWidth, headlineFontStep, type Style } from './style'

/**
 * Measuring and fitting, ported from `adapters/provider/thumbnail/text.go`.
 *
 * The Go renderer draws every headline glyph by glyph so it can put tracking
 * between them, and measures the same way. This does both the same way, for the
 * same reason and with the same arithmetic, including where Go rounds.
 *
 * Two things are load-bearing and easy to get wrong:
 *
 * Kerning. Go asks the face for `Kern(prev, r)` and adds it between glyphs. The
 * browser will not hand over a kern table, but it does not have to: the kern
 * between two glyphs is what `measureText` loses when you measure them apart,
 * so `measureText("AV") - measureText("A") - measureText("V")` *is* the pair's
 * kern. The same number, arrived at backwards.
 *
 * Rounding, which is subtler and matters more. Go builds its faces with
 * `font.HintingFull`, and a fully hinted face rounds every advance and every
 * kern to a whole pixel — so a Go pen starts on an integer and only ever moves
 * by integers, and *every glyph lands on a pixel boundary*. The browser will
 * happily draw at x = 41.37, and a string drawn that way alternates between
 * matching Go and sitting half a pixel off it, glyph by glyph, which is
 * visible as a shimmer along the line when the two are compared. So each
 * advance and each kern is rounded here too, and the pen stays integral.
 *
 * Because those advances are already whole pixels, the total is as well, and
 * Go's `.Ceil()` before a width comparison is a no-op on it. It is kept anyway:
 * it costs nothing and it is what the original does.
 *
 * What cannot be matched is rasterisation. Go hints the glyph outlines as well
 * as their advances; macOS browsers do not hint at all. Positions and metrics
 * agree exactly; the ink inside each glyph is a little different, and no amount
 * of arithmetic on this side will change that.
 */

/** One glyph and where its pen sits, relative to the start of the line. */
export interface Placed {
  glyph: string
  x: number
}

/** A measurer bound to one font family, cached per size. */
export interface Ruler {
  /**
   * Where every glyph of a line goes, and how wide the line comes to.
   *
   * One function for measuring and for drawing, which is not tidiness — it is
   * the only way the two cannot disagree. The first version had a `width` used
   * for centring and a separate pen loop used for drawing, and the pen loop
   * advanced by the *ceiled* width of each glyph where Go advances by a rounded
   * one. Every glyph gained up to a pixel, the error accumulated left to right,
   * and a five-letter headline sat five pixels right of where it had been
   * centred.
   *
   * Rounding each advance and *accumulating* the rounded values is what Go
   * does, and it is measurably right rather than merely plausible: rounding the
   * running total instead — which spreads the error rather than compounding it,
   * and looks like the better idea — moves further from the Go render, not
   * closer. Mean per-pixel difference over the whole frame goes from 1.36 to
   * 1.70. The compounding is the point, because it is Go's compounding.
   */
  layout: (text: string, size: number, tracking: number) => { glyphs: Placed[]; width: number }
  /** The drawn width, ceiled, which is the number Go compares against limits. */
  width: (text: string, size: number, tracking: number) => number
  /** Distance from the top of a line box to the baseline, as Go reports it. */
  ascent: (size: number) => number
  /** Applies the face at a size to a context, for drawing. */
  apply: (ctx: CanvasRenderingContext2D, size: number) => void
}

/**
 * A ruler over a loaded font family.
 *
 * The measuring context is its own offscreen canvas rather than the one being
 * drawn on: fitting walks a dozen sizes, and every walk would otherwise leave
 * the drawing context's font set to whatever the loop last tried.
 */
export function ruler(family: string): Ruler {
  const measuring = document.createElement('canvas').getContext('2d')
  if (!measuring) throw new Error('no 2d context')

  const font = (size: number) => `${size}px "${family}"`
  const advances = new Map<string, number>()

  // Per glyph and per pair, because a caption of twenty characters measured
  // pairwise is forty measurements, and every caption is measured at up to
  // fifteen sizes while the grid settles on one.
  const advance = (text: string, size: number): number => {
    const key = `${size} ${text}`
    const hit = advances.get(key)
    if (hit !== undefined) return hit
    measuring.font = font(size)
    const value = measuring.measureText(text).width
    advances.set(key, value)
    return value
  }

  const layout = (text: string, size: number, tracking: number) => {
    const runes = [...text]
    const glyphs: Placed[] = []
    let pen = 0
    for (let i = 0; i < runes.length; i += 1) {
      const glyph = runes[i]!
      if (i > 0) {
        const previous = runes[i - 1]!
        // What measuring the pair together keeps and measuring them apart
        // loses: exactly `face.Kern(prev, r)`.
        const kern =
          advance(previous + glyph, size) - advance(previous, size) - advance(glyph, size)
        pen += Math.round(kern) + tracking
      }
      glyphs.push({ glyph, x: pen })
      pen += Math.round(advance(glyph, size))
    }
    return { glyphs, width: pen }
  }

  return {
    layout,
    // Go compares `measure(...).Ceil()` against its limit; so does this, and
    // only here -- never on an individual glyph.
    width: (text, size, tracking) => Math.ceil(layout(text, size, tracking).width),
    ascent: (size) => {
      measuring.font = font(size)
      const metrics = measuring.measureText('H')
      // `fontBoundingBoxAscent` and Go's `face.Metrics().Ascent` both come off
      // the font's own tables, so they agree; Go ceils it, so this does too.
      return Math.ceil(metrics.fontBoundingBoxAscent)
    },
    apply: (ctx, size) => {
      ctx.font = font(size)
    },
  }
}

/** `trackingFor`: the extra space between headline glyphs. */
export function trackingFor(size: number, style: Style): number {
  return Math.max(Math.floor(size / style.headlineTracking), 1)
}

/** A fitted headline: the lines, the size they fit at, and where they sit. */
export interface Headline {
  lines: string[]
  size: number
  tracking: number
  top: number
  lineHeight: number
}

/** What the headline occupies. Go's `headlineLayout.height()`. */
export function headlineHeight(h: Headline | null): number {
  if (!h || h.lines.length === 0) return 0
  return h.lines.length * h.lineHeight
}

/**
 * `layOutHeadline`: as large as it goes on one line, wrapping only when even
 * the floor will not hold it.
 *
 * Every size is tried at one line before any is tried at two, so
 * small-and-single beats large-and-wrapped: wrapping costs the grid a third of
 * its height. One that fits neither bound is drawn at the floor and allowed to
 * overflow, because a clipped word beats a video that cannot produce a
 * thumbnail.
 */
export function layOutHeadline(
  rule: Ruler,
  headline: string,
  maxHeight: number,
  style: Style,
): Headline | null {
  const words = headline.toUpperCase().split(/\s+/).filter(Boolean)
  if (words.length === 0) return null
  const maxWidth = frameWidth - 2 * style.headlineSideMargin

  for (let lines = 1; lines <= style.headlineMaxLines; lines += 1) {
    for (
      let size = style.headlineFontMax;
      size >= style.headlineFontMin;
      size -= headlineFontStep
    ) {
      const tracking = trackingFor(size, style)
      const wrapped = wrap(rule, words, maxWidth, size, tracking)
      if (wrapped.length > lines) continue
      const candidate: Headline = {
        lines: wrapped,
        size,
        tracking,
        top: style.headlineTopMargin,
        lineHeight: size + style.headlineLineGap,
      }
      if (headlineHeight(candidate) <= maxHeight) return candidate
    }
  }

  const tracking = trackingFor(style.headlineFontMin, style)
  const lines = wrap(rule, words, maxWidth, style.headlineFontMin, tracking)
  return {
    lines: lines.slice(0, style.headlineMaxLines),
    size: style.headlineFontMin,
    tracking,
    top: style.headlineTopMargin,
    lineHeight: style.headlineFontMin + style.headlineLineGap,
  }
}

/** `wrap`: greedily break words into lines that fit. */
function wrap(
  rule: Ruler,
  words: string[],
  maxWidth: number,
  size: number,
  tracking: number,
): string[] {
  const lines: string[] = []
  let current = ''
  for (const word of words) {
    const candidate = current === '' ? word : `${current} ${word}`
    if (rule.width(candidate, size, tracking) <= maxWidth || current === '') {
      current = candidate
      continue
    }
    lines.push(current)
    current = word
  }
  if (current !== '') lines.push(current)
  return lines
}

/**
 * `fitCaptions`: one size for every caption in the grid, the largest at which
 * they all fit.
 *
 * Sizing each caption to its own tile would be easy and would look wrong: ten
 * tiles with ten different type sizes read as ten unrelated pictures, and the
 * grid's whole job is to read as a set.
 */
export function fitCaptions(
  rule: Ruler,
  captions: string[],
  maxWidth: number,
  style: Style,
): number {
  for (let size = style.captionFontMax; size >= style.captionFontMin; size -= captionFontStep) {
    if (captions.every((caption) => rule.width(caption, size, 0) <= maxWidth)) return size
  }
  // At the floor the long ones are cut rather than shrunk further.
  return style.captionFontMin
}

/** `captionText`: normalise, and cut to the tile if it still does not fit. */
export function captionText(rule: Ruler, caption: string, size: number, maxWidth: number): string {
  const text = caption.split(/\s+/).filter(Boolean).join(' ')
  if (text === '') return ''
  if (rule.width(text, size, 0) <= maxWidth) return text
  return truncate(rule, text, size, maxWidth)
}

/** `truncate`: drop characters until what is left fits, with an ellipsis. */
function truncate(rule: Ruler, text: string, size: number, maxWidth: number): string {
  const glyphs = [...text]
  while (glyphs.length > 1) {
    glyphs.pop()
    const candidate = `${glyphs.join('').trimEnd()}...`
    if (rule.width(candidate, size, 0) <= maxWidth) return candidate
  }
  return glyphs.join('')
}
