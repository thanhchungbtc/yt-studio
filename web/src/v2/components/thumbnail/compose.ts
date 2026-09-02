import { captionRowHeight, fontURL, frameHeight, frameWidth, rgba, type Style } from './style'
import {
  captionText,
  fitCaptions,
  headlineHeight,
  layOutHeadline,
  ruler,
  type Headline,
  type Ruler,
} from './text'

/**
 * The thumbnail, composed in the browser exactly as the Go renderer composes
 * it in the server.
 *
 * A port of `adapters/provider/thumbnail/{layout,paint,thumbnail}.go`, function
 * for function. The point is not that a browser can draw a picture; it is that
 * the picture it draws is the one the pipeline would have produced, so an
 * operator changing a caption is looking at the thing that publishes rather
 * than at an impression of it.
 *
 * Which is why the plate and the icon keying are per-pixel loops over
 * `ImageData` rather than `roundRect` and `globalAlpha`. Two reasons, and both
 * are visible:
 *
 *   - Go's `coverage()` is a signed-distance field sampled at `0.5 - d`. The
 *     browser's path antialiasing is a different function, and at a 10px radius
 *     on a 3px border the difference shows on every one of twelve tiles.
 *   - `paintPlate` strokes the border and fills the *translucent* plate in one
 *     pass, deliberately, so the border does not tint the fill. Canvas cannot
 *     express that as two draws in either order.
 *
 * What is not identical, and cannot be: resampling and text rasterisation. Go
 * scales with Catmull-Rom where the browser uses its own filter, and Go hints
 * glyphs fully where macOS browsers do not. The geometry, the metrics and every
 * colour agree; the interpolated and rasterised pixels are close, not equal.
 * `apply_thumbnail_override.go` is built on that being acceptable: the bytes the
 * operator saw are the bytes that publish, with no second renderer to keep in
 * step.
 */

/* ------------------------------------------------------------------- grid */

/**
 * `grid`: the resolved geometry of one thumbnail's tiles.
 *
 * The style rides along on it. Every one of the geometry helpers below reads
 * two or three tunables, and threading the whole style through each of them
 * separately was six signatures saying the same thing -- where the grid was
 * *computed from* that style in the first place and can hardly be used with a
 * different one.
 */
interface Grid {
  tileSize: number
  rows: number
  cols: number
  /** How many tiles each row holds; the remainder is centred, not left short. */
  counts: number[]
  rowX: number[]
  rowY: number[]
  style: Style
}

/** `blockHeight`: how tall rows of tiles plus their captions come to. */
function blockHeight(rows: number, tile: number, style: Style): number {
  return (
    rows * (tile + style.tileToCaptionGap + captionRowHeight(style)) +
    (rows - 1) * style.tileSpacing
  )
}

/**
 * `layOutGrid`: size the tiles from the frame width.
 *
 * Width first, so the tiles always span the frame and it is the headline that
 * gives way. Sizing from leftover height leaves small tiles stranded between
 * wide empty gutters.
 */
function layOutGrid(cells: number, style: Style): Grid {
  let rows = style.rows
  if (rows < 1) rows = 1
  if (rows > cells) rows = cells
  const cols = Math.floor((cells + rows - 1) / rows)

  let tile = Math.floor(
    (frameWidth - 2 * style.gridSideMargin - (cols - 1) * style.tileSpacing) / cols,
  )
  // The one case where the tiles give way instead: a grid so tall that the
  // headline would not get its floor.
  while (
    tile > 1 &&
    frameHeight -
      blockHeight(rows, tile, style) -
      style.gridBottomMargin -
      style.headlineToGridGap <
      style.headlineTopMargin + style.headlineFontMin
  ) {
    tile -= 2
  }
  if (tile < 1) tile = 1

  const grid: Grid = { tileSize: tile, rows, cols, counts: [], rowX: [], rowY: [], style }
  let remaining = cells
  for (let r = 0; r < rows; r += 1) {
    const n = Math.min(cols, remaining)
    remaining -= n
    grid.counts.push(n)
    const rowWidth = n * tile + (n - 1) * style.tileSpacing
    grid.rowX.push(Math.floor((frameWidth - rowWidth) / 2))
    grid.rowY.push(0)
  }
  place(grid, 0)
  return grid
}

/** `headlineBudget`: how much height is left above the grid. */
function headlineBudget(grid: Grid): number {
  const { style } = grid
  return (
    frameHeight -
    style.gridBottomMargin -
    blockHeight(grid.rows, grid.tileSize, style) -
    style.headlineToGridGap -
    style.headlineTopMargin
  )
}

/**
 * `place`: centre the block in what the headline left.
 *
 * Rather than pinning it to the bottom, where a grid of small tiles would leave
 * a band of empty background that reads as a mistake.
 */
function place(grid: Grid, headlineBottom: number): void {
  const { style } = grid
  const bandTop = Math.max(headlineBottom + style.headlineToGridGap, style.headlineTopMargin)
  const bandBottom = frameHeight - style.gridBottomMargin
  const block = blockHeight(grid.rows, grid.tileSize, style)

  const top = bandTop + Math.floor(Math.max(bandBottom - bandTop - block, 0) / 2)
  for (let r = 0; r < grid.rows; r += 1) {
    grid.rowY[r] =
      top +
      r * (grid.tileSize + style.tileToCaptionGap + captionRowHeight(style) + style.tileSpacing)
  }
}

/** `rowOf`: which row and column the i-th cell lands in. */
function rowOf(grid: Grid, i: number): { row: number; col: number } {
  for (let r = 0; r < grid.rows; r += 1) {
    if (i < (grid.counts[r] ?? 0)) return { row: r, col: i }
    i -= grid.counts[r] ?? 0
  }
  return { row: grid.rows - 1, col: 0 }
}

/** `tile`: the box the i-th icon is drawn in. */
function tileBox(grid: Grid, i: number): Box {
  const { row, col } = rowOf(grid, i)
  const x = (grid.rowX[row] ?? 0) + col * (grid.tileSize + grid.style.tileSpacing)
  const y = grid.rowY[row] ?? 0
  return { x, y, w: grid.tileSize, h: grid.tileSize }
}

/** `captionTop`: the top of the box the i-th caption is drawn in. */
function captionTop(grid: Grid, i: number): number {
  const { row } = rowOf(grid, i)
  return (grid.rowY[row] ?? 0) + grid.tileSize + grid.style.tileToCaptionGap
}

interface Box {
  x: number
  y: number
  w: number
  h: number
}

/* ------------------------------------------------------------------ paint */

/**
 * `coverage`: how much of the pixel at (x, y) falls inside a rounded
 * rectangle, from 0 to 1.
 *
 * The signed distance to the edge, read across the half pixel either side of
 * it, which is what makes the corners an arc and not a staircase.
 */
function coverage(x: number, y: number, box: Box, radius: number): number {
  const px = x + 0.5
  const py = y + 0.5
  const cx = box.x + box.w / 2
  const cy = box.y + box.h / 2

  // Distance from the centre, measured to the box inset by its own corner
  // radius: inside that inset rectangle the corners play no part.
  const qx = Math.abs(px - cx) - (box.w / 2 - radius)
  const qy = Math.abs(py - cy) - (box.h / 2 - radius)
  const d = Math.hypot(Math.max(qx, 0), Math.max(qy, 0)) + Math.min(Math.max(qx, qy), 0) - radius

  return Math.min(Math.max(0.5 - d, 0), 1)
}

/** `inset`: a box shrunk on every side, as `image.Rectangle.Inset`. */
function inset(box: Box, by: number): Box {
  return { x: box.x + by, y: box.y + by, w: box.w - 2 * by, h: box.h - 2 * by }
}

/**
 * `blend`: lay a colour over one pixel at a coverage, honouring its own alpha.
 *
 * The canvas is opaque throughout, so there is no destination alpha to carry.
 * Written straight into the frame's `ImageData` rather than through the 2D
 * context, which is the only way to reproduce Go's arithmetic exactly.
 */
function blend(
  pixels: Uint8ClampedArray,
  offset: number,
  colour: { r: number; g: number; b: number; a: number },
  cov: number,
): void {
  const a = (cov * colour.a) / 255
  if (a <= 0) return
  pixels[offset] = colour.r * a + (pixels[offset] ?? 0) * (1 - a)
  pixels[offset + 1] = colour.g * a + (pixels[offset + 1] ?? 0) * (1 - a)
  pixels[offset + 2] = colour.b * a + (pixels[offset + 2] ?? 0) * (1 - a)
  pixels[offset + 3] = 255
}

/**
 * `paintPlate`: fill the tile and stroke its border in one pass.
 *
 * One pass because the border is opaque and the plate is not, so filling first
 * would tint the plate.
 */
function paintPlate(frame: ImageData, box: Box, radius: number, style: Style): void {
  const inner = inset(box, style.tileBorderWidth)
  const innerRadius = Math.max(radius - style.tileBorderWidth, 0)
  // Resolved once for the whole tile rather than per pixel. Both carry an
  // alpha; `blend` has always honoured it, and Go fixes the border's at 255.
  const border = rgba(style.tileBorderColor, style.tileBorderAlpha)
  const fill = rgba(style.tileFillColor, style.tileFillAlpha)

  for (let y = box.y; y < box.y + box.h; y += 1) {
    for (let x = box.x; x < box.x + box.w; x += 1) {
      const outside = coverage(x, y, box, radius)
      if (outside <= 0) continue
      const within = coverage(x, y, inner, innerRadius)
      const offset = (y * frame.width + x) * 4
      const ring = outside - within
      if (ring > 0) blend(frame.data, offset, border, ring)
      if (within > 0) blend(frame.data, offset, fill, within)
    }
  }
}

/**
 * `keyOutBackground`: turn an icon's dark field into transparency, in place.
 *
 * The icons arrive as artwork on a solid dark field, which is what an image
 * model returns when asked for line art. Drawn as-is they would stamp that
 * field over the tile.
 *
 * Go re-premultiplies the colours against the new alpha because `image/draw`
 * works in premultiplied alpha. `ImageData` does not: the browser premultiplies
 * on its own when compositing, so doing it here as well would multiply every
 * icon by its alpha twice and leave the whole grid visibly dark. Alpha only.
 */
function keyOutBackground(icon: ImageData, style: Style): void {
  const pixels = icon.data
  for (let i = 0; i < pixels.length; i += 4) {
    const r = pixels[i] ?? 0
    const g = pixels[i + 1] ?? 0
    const b = pixels[i + 2] ?? 0
    // Rec. 601 luma: the eye reads green as most of an image's brightness, and
    // a flat average would key blue artwork differently from red.
    const lum = Math.floor((299 * r + 587 * g + 114 * b) / 1000)

    let alpha: number
    if (lum <= style.iconTransparentBelow) alpha = 0
    else if (lum >= style.iconOpaqueAbove) alpha = 255
    else
      alpha = Math.floor(
        ((lum - style.iconTransparentBelow) * 255) /
          (style.iconOpaqueAbove - style.iconTransparentBelow),
      )

    pixels[i + 3] = alpha
  }
}

/**
 * `cover`: scale an image to fill a box and centre-crop the overflow.
 *
 * The arithmetic is Go's; the resampling is the browser's. `xdraw.CatmullRom`
 * and `drawImage` will not agree pixel for pixel, which on a photograph and on
 * line art is the difference nobody can see and the one worth naming anyway.
 */
function cover(ctx: CanvasRenderingContext2D, src: CanvasImageSource, w: number, h: number): void {
  const dx = 'naturalWidth' in src ? src.naturalWidth : (src as HTMLCanvasElement).width
  const dy = 'naturalHeight' in src ? src.naturalHeight : (src as HTMLCanvasElement).height
  if (dx === 0 || dy === 0) return

  // The larger of the two ratios is the one that covers.
  const scale = Math.max(w / dx, h / dy)
  const sw = Math.floor(dx * scale)
  const sh = Math.floor(dy * scale)
  const offsetX = Math.floor((sw - w) / 2)
  const offsetY = Math.floor((sh - h) / 2)

  ctx.imageSmoothingEnabled = true
  ctx.imageSmoothingQuality = 'high'
  ctx.drawImage(src, -offsetX, -offsetY, sw, sh)
}

/* ---------------------------------------------------------------- compose */

/** One cell: the caption under the tile, and the artwork in it. */
export interface Cell {
  caption: string
  icon: HTMLImageElement | null
}

/**
 * The shape of the grid, without drawing it.
 *
 * Exported because the builder lays its caption fields out in the same shape:
 * six across and two down, exactly where the tiles are. A field's position is
 * then the answer to "which tile is this?", which counting to nine in a
 * vertical list never is.
 */
export function gridShape(cells: number, style: Style): { rows: number; cols: number } {
  const grid = layOutGrid(cells, style)
  return { rows: grid.rows, cols: grid.cols }
}

/**
 * What the fitting settled on, handed back after a draw.
 *
 * Every number here is a decision the layout made rather than one anybody
 * typed, and each is the answer to a question the picture provokes: why did the
 * headline suddenly get smaller, why is every caption the same size as the
 * longest one. Reporting them costs nothing -- they were computed anyway.
 */
export interface Report {
  headlineSize: number
  headlineLines: number
  captionSize: number
  tileSize: number
}

export interface Composition {
  headline: string
  cells: Cell[]
  /** Every tunable of the look. `defaultStyle` is what the renderer uses. */
  style: Style
  /** The typeface family, already loaded. See `loadFont`. */
  family: string
  background: HTMLImageElement
}

/**
 * `Render`: draw exactly one thumbnail into a 1280x720 canvas.
 *
 * The canvas is the output. There is no separate export path that could
 * disagree with what is on screen, which is the whole reason the preview is a
 * real canvas at the real size rather than a DOM layout that has to be
 * rasterised by something else afterwards.
 */
export function compose(canvas: HTMLCanvasElement, input: Composition): Report | null {
  canvas.width = frameWidth
  canvas.height = frameHeight
  const ctx = canvas.getContext('2d')
  if (!ctx) return null

  // The backdrop, covered and then scrimmed so white type over it is legible
  // without losing the photograph's texture entirely.
  const style = input.style
  cover(ctx, input.background, frameWidth, frameHeight)
  const frame = ctx.getImageData(0, 0, frameWidth, frameHeight)
  for (let i = 0; i < frame.data.length; i += 4) {
    frame.data[i] = Math.floor(((frame.data[i] ?? 0) * style.backgroundBrightness) / 255)
    frame.data[i + 1] = Math.floor(((frame.data[i + 1] ?? 0) * style.backgroundBrightness) / 255)
    frame.data[i + 2] = Math.floor(((frame.data[i + 2] ?? 0) * style.backgroundBrightness) / 255)
    frame.data[i + 3] = 255
  }

  const rule = ruler(input.family)

  // The grid takes the width it needs and the headline is fitted into what is
  // left, which is why the tiles run edge to edge.
  const grid = layOutGrid(input.cells.length, style)
  const headline = layOutHeadline(rule, input.headline, headlineBudget(grid), style)
  place(grid, style.headlineTopMargin + headlineHeight(headline))

  // Every plate before any text, because the plates are written through
  // `ImageData` and the text through the 2D context: one `putImageData` between
  // them, or the plates would erase the glyphs they were drawn after.
  if (input.cells.length > 0) {
    const radius = Math.min(style.tileCornerRadius, Math.floor(grid.tileSize / 2))
    for (let i = 0; i < input.cells.length; i += 1) {
      paintPlate(frame, tileBox(grid, i), radius, style)
    }
  }
  ctx.putImageData(frame, 0, 0)

  drawHeadline(ctx, rule, headline, style)
  const captionSize = drawGrid(ctx, rule, grid, input.cells)

  return {
    headlineSize: headline?.size ?? 0,
    headlineLines: headline?.lines.length ?? 0,
    captionSize,
    tileSize: grid.tileSize,
  }
}

/** `drawHeadline`: centre each line of the fitted headline. */
function drawHeadline(
  ctx: CanvasRenderingContext2D,
  rule: Ruler,
  h: Headline | null,
  style: Style,
): void {
  if (!h || h.lines.length === 0) return
  ctx.fillStyle = style.headlineColor
  ctx.textBaseline = 'alphabetic'
  for (let i = 0; i < h.lines.length; i += 1) {
    const line = h.lines[i]!
    const width = rule.width(line, h.size, h.tracking)
    const x = Math.floor((frameWidth - width) / 2)
    // The baseline sits above the bottom of the line box, which is what keeps
    // descenders inside the block rather than in the gap below it.
    const baseline = h.top + i * h.lineHeight + h.size
    drawTracked(ctx, rule, line, x, baseline, h.size, h.tracking)
  }
}

/** `drawGrid`: place every cell and paint its icon and caption. */
function drawGrid(ctx: CanvasRenderingContext2D, rule: Ruler, grid: Grid, cells: Cell[]): number {
  // A headline on its own is a thin thumbnail, not a broken one.
  if (cells.length === 0) return 0

  const { style } = grid
  const radius = Math.min(style.tileCornerRadius, Math.floor(grid.tileSize / 2))
  const captions = cells.map((cell) => cell.caption.split(/\s+/).filter(Boolean).join(' '))
  // One size for every caption: a dozen tiles at a dozen type sizes read as a
  // dozen unrelated pictures.
  const captionSize = fitCaptions(rule, captions, grid.tileSize, style)
  const ascent = rule.ascent(captionSize)

  for (let i = 0; i < cells.length; i += 1) {
    const cell = cells[i]!
    const box = tileBox(grid, i)
    if (cell.icon) drawIcon(ctx, box, radius, cell.icon, style)

    const text = captionText(rule, cell.caption, captionSize, box.w)
    if (text === '') continue
    const width = rule.width(text, captionSize, 0)
    ctx.fillStyle = style.captionColor
    ctx.textBaseline = 'alphabetic'
    drawTracked(
      ctx,
      rule,
      text,
      box.x + Math.floor((box.w - width) / 2),
      captionTop(grid, i) + ascent,
      captionSize,
      0,
    )
  }

  return captionSize
}

/**
 * `drawTile`'s icon half: the artwork over the plate, with its own background
 * keyed away and masked to the tile's rounding.
 *
 * Into a buffer rather than straight onto the frame, because the keying needs
 * the icon's own pixels before they are composited. Masked to the same rounding
 * or a square icon pokes out through the tile's corners at any radius wider
 * than its padding.
 */
function drawIcon(
  ctx: CanvasRenderingContext2D,
  box: Box,
  radius: number,
  icon: HTMLImageElement,
  style: Style,
): void {
  const by = style.tileBorderWidth + style.tileIconPadding
  const inner = inset(box, by)
  if (inner.w <= 0 || inner.h <= 0) return

  const buffer = document.createElement('canvas')
  buffer.width = inner.w
  buffer.height = inner.h
  const scratch = buffer.getContext('2d')
  if (!scratch) return
  scratch.imageSmoothingEnabled = true
  scratch.imageSmoothingQuality = 'high'
  scratch.drawImage(icon, 0, 0, inner.w, inner.h)

  const pixels = scratch.getImageData(0, 0, inner.w, inner.h)
  keyOutBackground(pixels, style)

  // The rounded mask, applied into the buffer's alpha rather than as a clip
  // path on the frame: `coverage` is the same function the plate used, so the
  // icon's corners and the plate's corners are cut by identical arcs.
  const mask = Math.max(radius - by, 0)
  const shape: Box = { x: 0, y: 0, w: inner.w, h: inner.h }
  for (let y = 0; y < inner.h; y += 1) {
    for (let x = 0; x < inner.w; x += 1) {
      const offset = (y * inner.w + x) * 4
      const alpha = pixels.data[offset + 3] ?? 0
      pixels.data[offset + 3] = alpha * coverage(x, y, shape, mask)
    }
  }
  scratch.putImageData(pixels, 0, 0)
  ctx.drawImage(buffer, inner.x, inner.y)
}

/**
 * `drawString`: one line at a baseline, glyph by glyph so tracking can be
 * applied between them.
 *
 * Not `letterSpacing` on the context: that is not in every engine this has to
 * run in, and where it is it rounds differently from adding an integer number
 * of pixels per gap, which is what Go does.
 *
 * The pen offsets come from `rule.layout`, the same call that measured the line
 * for centring. Computing them here instead is what put the text a few pixels
 * off: see the note on `Ruler`.
 */
function drawTracked(
  ctx: CanvasRenderingContext2D,
  rule: Ruler,
  text: string,
  x: number,
  baseline: number,
  size: number,
  tracking: number,
): void {
  rule.apply(ctx, size)
  for (const placed of rule.layout(text, size, tracking).glyphs) {
    ctx.fillText(placed.glyph, x + placed.x, baseline)
  }
}

/* -------------------------------------------------------------- resources */

/**
 * Load the typeface the renderer is configured with, and wait for it.
 *
 * The waiting is the point. Every fitting loop measures text, so a first paint
 * that ran before the face arrived would measure a fallback, settle on the
 * wrong size, and then draw correct-looking type at it. The bug looks like a
 * design decision, which is the worst kind.
 *
 * Registered under a name of its own rather than the family in the file, so a
 * system font of the same name cannot answer instead.
 */
export async function loadFont(file: string): Promise<string> {
  const family = `yts-thumb-${file.replace(/[^a-zA-Z0-9]/g, '-')}`
  const face = new FontFace(family, `url(${fontURL(file)})`)
  await face.load()
  document.fonts.add(face)
  return family
}

/** An image, once the bytes are in. */
export function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve(image)
    image.onerror = () => reject(new Error(`could not load ${src}`))
    image.src = src
  })
}
