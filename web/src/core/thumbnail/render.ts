/**
 * Draws a design onto a 2D canvas.
 *
 * One function, used by the live preview and by the export both. That is the
 * whole reason this feature can call itself WYSIWYG: there is no second
 * renderer to keep in step, and the bytes uploaded come from the same calls
 * that painted the screen a moment earlier.
 *
 * Canvas rather than DOM-plus-rasteriser: what is exported is exactly what was
 * drawn, with no library approximating CSS, and hit-testing is cheap because
 * every element already carries its own box.
 */

import type { Design, TextElement, TileElement } from './doc'
import { FRAME_HEIGHT, FRAME_WIDTH } from './doc'
import { STYLE } from './style'

/** Loaded images, by URL. Rendering is synchronous; loading is not. */
export type ImageBank = Map<string, HTMLImageElement>

/* ------------------------------------------------------------ measuring */

/**
 * The drawn width of a string, tracking included.
 *
 * Glyph by glyph, because that is how it is drawn — the browser applies
 * kerning across a whole run and none across single characters, so measuring a
 * run and drawing glyphs would disagree with each other by a few pixels per
 * word. Self-consistency is what matters here; the Go renderer's own metrics
 * are a separate question and only affect the seed.
 */
export function measureTracked(
  ctx: CanvasRenderingContext2D,
  text: string,
  tracking: number,
): number {
  let total = 0
  const chars = [...text]
  for (const ch of chars) total += ctx.measureText(ch).width
  return total + Math.max(chars.length - 1, 0) * tracking
}

function drawTracked(
  ctx: CanvasRenderingContext2D,
  text: string,
  x: number,
  baseline: number,
  tracking: number,
): void {
  let cursor = x
  for (const ch of [...text]) {
    ctx.fillText(ch, cursor, baseline)
    cursor += ctx.measureText(ch).width + tracking
  }
}

function strokeTracked(
  ctx: CanvasRenderingContext2D,
  text: string,
  x: number,
  baseline: number,
  tracking: number,
): void {
  let cursor = x
  for (const ch of [...text]) {
    ctx.strokeText(ch, cursor, baseline)
    cursor += ctx.measureText(ch).width + tracking
  }
}

export function fontSpec(size: number, family: string): string {
  return `${size}px "${family}", sans-serif`
}

/** Greedy wrap into the element's own width. */
function wrapText(
  ctx: CanvasRenderingContext2D,
  text: string,
  width: number,
  tracking: number,
): string[] {
  const words = text.split(/\s+/).filter(Boolean)
  if (words.length === 0) return []
  const lines: string[] = []
  let current = ''
  for (const w of words) {
    const candidate = current === '' ? w : `${current} ${w}`
    if (measureTracked(ctx, candidate, tracking) <= width || current === '') {
      current = candidate
      continue
    }
    lines.push(current)
    current = w
  }
  if (current !== '') lines.push(current)
  return lines
}

/** Cuts a caption to its tile, with an ellipsis to say that it was cut. */
function truncate(ctx: CanvasRenderingContext2D, text: string, width: number): string {
  if (measureTracked(ctx, text, 0) <= width) return text
  const chars = [...text]
  while (chars.length > 1) {
    chars.pop()
    const candidate = `${chars.join('').trimEnd()}...`
    if (measureTracked(ctx, candidate, 0) <= width) return candidate
  }
  return chars.join('')
}

/* -------------------------------------------------------------- keying */

/**
 * Turns an icon's dark field into transparency.
 *
 * The icons arrive as artwork on a solid dark background — that is what an
 * image model returns when asked for line art. Drawn as-is they stamp that
 * field over the tile. The ramp between the two thresholds is what stops the
 * anti-aliased edges becoming a hard jagged cut.
 *
 * Keyed once per image and cached: this is a per-pixel pass and the preview
 * redraws on every pointer move.
 */
const keyedCache = new Map<string, HTMLCanvasElement>()

function keyedIcon(img: HTMLImageElement, below: number, above: number): HTMLCanvasElement | null {
  const cacheKey = `${img.src}|${below}|${above}`
  const cached = keyedCache.get(cacheKey)
  if (cached) return cached

  const w = img.naturalWidth
  const h = img.naturalHeight
  if (w === 0 || h === 0) return null

  const off = document.createElement('canvas')
  off.width = w
  off.height = h
  const ctx = off.getContext('2d', { willReadFrequently: true })
  if (!ctx) return null
  ctx.drawImage(img, 0, 0)

  const data = ctx.getImageData(0, 0, w, h)
  const px = data.data
  const span = Math.max(above - below, 1)
  for (let i = 0; i < px.length; i += 4) {
    const r = px[i] ?? 0
    const g = px[i + 1] ?? 0
    const b = px[i + 2] ?? 0
    // Rec. 601 luma: the eye reads green as most of an image's brightness, and
    // a flat average would key blue artwork differently from red.
    const lum = (299 * r + 587 * g + 114 * b) / 1000
    let alpha = 255
    if (lum <= below) alpha = 0
    else if (lum < above) alpha = ((lum - below) * 255) / span
    px[i + 3] = alpha
  }
  ctx.putImageData(data, 0, 0)

  // Bounded so a long session on many videos cannot grow this without limit.
  if (keyedCache.size > 64) keyedCache.clear()
  keyedCache.set(cacheKey, off)
  return off
}

/* --------------------------------------------------------------- shapes */

function roundedPath(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  w: number,
  h: number,
  radius: number,
): void {
  const r = Math.max(0, Math.min(radius, Math.min(w, h) / 2))
  ctx.beginPath()
  ctx.moveTo(x + r, y)
  ctx.lineTo(x + w - r, y)
  ctx.arcTo(x + w, y, x + w, y + r, r)
  ctx.lineTo(x + w, y + h - r)
  ctx.arcTo(x + w, y + h, x + w - r, y + h, r)
  ctx.lineTo(x + r, y + h)
  ctx.arcTo(x, y + h, x, y + h - r, r)
  ctx.lineTo(x, y + r)
  ctx.arcTo(x, y, x + r, y, r)
  ctx.closePath()
}

/* ------------------------------------------------------------ elements */

function drawTextElement(ctx: CanvasRenderingContext2D, el: TextElement, fontFamily: string): void {
  const text = el.uppercase ? el.text.toUpperCase() : el.text
  if (text.trim() === '') return

  ctx.save()
  ctx.font = fontSpec(el.fontSize, fontFamily)
  ctx.textBaseline = 'alphabetic'
  ctx.fillStyle = el.color

  const lines = wrapText(ctx, text, el.w, el.tracking)
  const lineHeight = el.fontSize + el.lineGap

  if (el.shadowBlur > 0 || el.shadowY !== 0) {
    ctx.shadowColor = el.shadowColor
    ctx.shadowBlur = el.shadowBlur
    ctx.shadowOffsetY = el.shadowY
  }

  lines.forEach((line, i) => {
    const width = measureTracked(ctx, line, el.tracking)
    let x = el.x
    if (el.align === 'center') x = el.x + (el.w - width) / 2
    else if (el.align === 'right') x = el.x + el.w - width
    // The baseline sits above the bottom of the line box, which keeps
    // descenders inside the block rather than in the gap below it.
    const baseline = el.y + i * lineHeight + el.fontSize
    if (el.strokeWidth > 0) {
      ctx.save()
      ctx.strokeStyle = el.strokeColor
      ctx.lineWidth = el.strokeWidth
      ctx.lineJoin = 'round'
      strokeTracked(ctx, line, x, baseline, el.tracking)
      ctx.restore()
    }
    drawTracked(ctx, line, x, baseline, el.tracking)
  })
  ctx.restore()
}

function drawTileElement(
  ctx: CanvasRenderingContext2D,
  el: TileElement,
  fontFamily: string,
  images: ImageBank,
  assetUrl: (id: string) => string,
): void {
  ctx.save()

  // The plate and its border in one pass: the border is opaque and the plate is
  // not, so filling the whole box first would tint the border.
  const radius = Math.max(0, Math.min(el.radius, Math.min(el.w, el.h) / 2))
  roundedPath(ctx, el.x, el.y, el.w, el.h, radius)
  ctx.fillStyle = el.fillColor
  ctx.fill()
  if (el.borderWidth > 0) {
    roundedPath(
      ctx,
      el.x + el.borderWidth / 2,
      el.y + el.borderWidth / 2,
      el.w - el.borderWidth,
      el.h - el.borderWidth,
      Math.max(radius - el.borderWidth / 2, 0),
    )
    ctx.strokeStyle = el.borderColor
    ctx.lineWidth = el.borderWidth
    ctx.stroke()
  }

  const inset = el.borderWidth + el.padding
  const inner = {
    x: el.x + inset,
    y: el.y + inset,
    w: el.w - 2 * inset,
    h: el.h - 2 * inset,
  }
  if (inner.w > 0 && inner.h > 0 && el.assetId) {
    const img = images.get(assetUrl(el.assetId))
    if (img?.complete && img.naturalWidth > 0) {
      const keyed = keyedIcon(img, el.keyBelow, el.keyAbove)
      if (keyed) {
        ctx.save()
        // Masked to the same rounding, or a square icon pokes out through the
        // tile's corners at any radius wider than its padding.
        roundedPath(ctx, inner.x, inner.y, inner.w, inner.h, Math.max(radius - inset, 0))
        ctx.clip()
        ctx.drawImage(keyed, inner.x, inner.y, inner.w, inner.h)
        ctx.restore()
      }
    }
  }
  ctx.restore()

  if (el.caption.trim() === '') return
  ctx.save()
  ctx.font = fontSpec(el.captionSize, fontFamily)
  ctx.textBaseline = 'top'
  ctx.fillStyle = el.captionColor
  const caption = truncate(ctx, el.caption.split(/\s+/).filter(Boolean).join(' '), el.w)
  const width = measureTracked(ctx, caption, 0)
  drawTracked(ctx, caption, el.x + (el.w - width) / 2, el.y + el.h + STYLE.tileToCaptionGap, 0)
  ctx.restore()
}

/* -------------------------------------------------------------- render */

export interface RenderOptions {
  design: Design
  /**
   * The CSS family the typeface is registered under, which is *not*
   * `design.font` — that is the filename on disk. Passing the filename to
   * `ctx.font` names no family, so the canvas silently falls back to sans and
   * the operator lays out a headline in a face they will never publish.
   * Resolved by the caller, which is what loads the font in the first place.
   */
  fontFamily: string
  images: ImageBank
  assetUrl: (id: string) => string
  resourceUrl: (name: string) => string
}

/**
 * Paints the whole frame. Synchronous and total: anything not yet loaded is
 * skipped rather than awaited, and the caller redraws when an image lands.
 */
export function renderDesign(ctx: CanvasRenderingContext2D, opts: RenderOptions): void {
  const { design, images } = opts
  ctx.clearRect(0, 0, FRAME_WIDTH, FRAME_HEIGHT)

  // The backdrop, scaled to cover and centre-cropped so any aspect fills the
  // frame without distortion, then scrimmed: white type over an undimmed
  // photograph is unreadable.
  const bg = images.get(opts.resourceUrl(design.background))
  if (bg?.complete && bg.naturalWidth > 0) {
    const scale = Math.max(FRAME_WIDTH / bg.naturalWidth, FRAME_HEIGHT / bg.naturalHeight)
    const w = bg.naturalWidth * scale
    const h = bg.naturalHeight * scale
    ctx.drawImage(bg, (FRAME_WIDTH - w) / 2, (FRAME_HEIGHT - h) / 2, w, h)
  } else {
    ctx.fillStyle = '#101014'
    ctx.fillRect(0, 0, FRAME_WIDTH, FRAME_HEIGHT)
  }
  if (design.scrim < 255) {
    ctx.save()
    // Multiply reproduces the Go scrim, which scales each channel toward black
    // rather than washing a flat grey over the photograph.
    ctx.globalCompositeOperation = 'multiply'
    const level = Math.round(Math.max(0, Math.min(design.scrim, 255)))
    ctx.fillStyle = `rgb(${level}, ${level}, ${level})`
    ctx.fillRect(0, 0, FRAME_WIDTH, FRAME_HEIGHT)
    ctx.restore()
  }

  for (const el of design.elements) {
    if (el.hidden) continue
    switch (el.kind) {
      case 'text':
        drawTextElement(ctx, el, opts.fontFamily)
        break
      case 'tile':
        drawTileElement(ctx, el, opts.fontFamily, images, opts.assetUrl)
        break
      case 'image': {
        const img = images.get(opts.assetUrl(el.assetId))
        if (!img?.complete || img.naturalWidth === 0) break
        ctx.save()
        ctx.globalAlpha = el.opacity
        roundedPath(ctx, el.x, el.y, el.w, el.h, el.radius)
        ctx.clip()
        ctx.drawImage(img, el.x, el.y, el.w, el.h)
        ctx.restore()
        break
      }
      default:
        break
    }
  }
}

/** Every image URL a design needs, so the caller can preload before drawing. */
export function imageUrls(
  design: Design,
  assetUrl: (id: string) => string,
  resourceUrl: (name: string) => string,
): string[] {
  const urls = new Set<string>([resourceUrl(design.background)])
  for (const el of design.elements) {
    if (el.kind === 'tile' && el.assetId) urls.add(assetUrl(el.assetId))
    if (el.kind === 'image') urls.add(assetUrl(el.assetId))
  }
  return [...urls]
}
