/**
 * The thumbnail editor's document.
 *
 * This is the whole contract of the feature. The browser draws the thumbnail,
 * the operator edits it, and on apply the canvas is rasterised and uploaded —
 * so the image is finished before it reaches the server, and nothing on that
 * side ever renders this. The server stores the document verbatim and hands it
 * back; only this file knows its shape.
 *
 * Which is why `readDesign` exists and is the only way in: a document is
 * whatever a previous version of this code wrote, and it has to be checked
 * rather than trusted.
 */

/** Bumped when a change here cannot be read by the tolerant loader below. */
export const DESIGN_VERSION = 1

/** YouTube's frame. The canvas is exactly this, and the export is too. */
export const FRAME_WIDTH = 1280
export const FRAME_HEIGHT = 720

export type ElementKind = 'text' | 'tile' | 'image'

interface ElementBase {
  id: string
  kind: ElementKind
  x: number
  y: number
  w: number
  h: number
  /** Locked elements draw but cannot be picked up, so a backdrop stays put. */
  locked?: boolean
  hidden?: boolean
}

export interface TextElement extends ElementBase {
  kind: 'text'
  text: string
  fontSize: number
  color: string
  align: 'left' | 'center' | 'right'
  /** Letter-spacing in pixels. The Go renderer states this as a divisor of the
   *  type size; pixels are what a drag on a slider means. */
  tracking: number
  lineGap: number
  uppercase: boolean
  strokeColor: string
  strokeWidth: number
  shadowColor: string
  shadowBlur: number
  shadowY: number
}

export interface TileElement extends ElementBase {
  kind: 'tile'
  /** The generated icon for this cell; absent while it is still being drawn. */
  assetId?: string
  caption: string
  captionSize: number
  captionColor: string
  fillColor: string
  borderColor: string
  borderWidth: number
  radius: number
  padding: number
  /**
   * The icons arrive as artwork on a solid dark field — that is what an image
   * model returns when asked for line art. Drawn as-is they stamp that field
   * over the tile, so darkness becomes transparency instead. The ramp between
   * the two is what stops anti-aliased edges turning into a jagged cut.
   */
  keyBelow: number
  keyAbove: number
}

export interface ImageElement extends ElementBase {
  kind: 'image'
  assetId: string
  radius: number
  opacity: number
}

export type DesignElement = TextElement | TileElement | ImageElement

export interface Design {
  version: number
  /** Typeface filename under the resources fonts directory. */
  font: string
  /** How much of the backdrop survives, 0..255. Lower is darker: white type
   *  over an undimmed photograph is unreadable. */
  scrim: number
  background: string
  elements: DesignElement[]
}

/* ------------------------------------------------------------ reading in */

function num(v: unknown, fallback: number): number {
  return typeof v === 'number' && Number.isFinite(v) ? v : fallback
}

function str(v: unknown, fallback: string): string {
  return typeof v === 'string' ? v : fallback
}

function bool(v: unknown, fallback: boolean): boolean {
  return typeof v === 'boolean' ? v : fallback
}

function rec(v: unknown): Record<string, unknown> | undefined {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
    ? (v as Record<string, unknown>)
    : undefined
}

function readBase(raw: Record<string, unknown>, index: number): ElementBase {
  return {
    id: str(raw.id, `el-${index}`),
    kind: 'text',
    x: num(raw.x, 0),
    y: num(raw.y, 0),
    w: num(raw.w, 100),
    h: num(raw.h, 100),
    locked: bool(raw.locked, false),
    hidden: bool(raw.hidden, false),
  }
}

function readElement(raw: unknown, index: number): DesignElement | undefined {
  const r = rec(raw)
  if (!r) return undefined
  const base = readBase(r, index)
  const assetId = typeof r.assetId === 'string' && r.assetId !== '' ? r.assetId : undefined

  switch (r.kind) {
    case 'text':
      return {
        ...base,
        kind: 'text',
        text: str(r.text, ''),
        fontSize: num(r.fontSize, 72),
        color: str(r.color, '#f6f6f4'),
        align: r.align === 'left' || r.align === 'right' ? r.align : 'center',
        tracking: num(r.tracking, 0),
        lineGap: num(r.lineGap, 6),
        uppercase: bool(r.uppercase, true),
        strokeColor: str(r.strokeColor, '#000000'),
        strokeWidth: num(r.strokeWidth, 0),
        shadowColor: str(r.shadowColor, 'rgba(0,0,0,0.55)'),
        shadowBlur: num(r.shadowBlur, 0),
        shadowY: num(r.shadowY, 0),
      }
    case 'tile':
      return {
        ...base,
        kind: 'tile',
        assetId,
        caption: str(r.caption, ''),
        captionSize: num(r.captionSize, 20),
        captionColor: str(r.captionColor, '#e2e2de'),
        fillColor: str(r.fillColor, 'rgba(6,6,8,0.92)'),
        borderColor: str(r.borderColor, '#e4e4e0'),
        borderWidth: num(r.borderWidth, 3),
        radius: num(r.radius, 10),
        padding: num(r.padding, 8),
        keyBelow: num(r.keyBelow, 48),
        keyAbove: num(r.keyAbove, 105),
      }
    case 'image':
      // An image element with no asset has nothing to draw and no way to get
      // one, so it is dropped rather than kept as an empty box.
      if (!assetId) return undefined
      return {
        ...base,
        kind: 'image',
        assetId,
        radius: num(r.radius, 0),
        opacity: num(r.opacity, 1),
      }
    default:
      return undefined
  }
}

/**
 * Reads a stored document, filling anything missing with a default.
 *
 * Deliberately tolerant: the server never validated this beyond "it is JSON",
 * and a document written by an older build must still open. Anything
 * unrecognisable yields `undefined`, and the caller seeds a fresh one — the
 * operator loses a layout, which is recoverable, rather than facing a screen
 * that will not load, which is not.
 */
export function readDesign(raw: unknown): Design | undefined {
  const r = rec(raw)
  if (!r) return undefined
  if (!Array.isArray(r.elements)) return undefined
  const elements = r.elements
    .map((el, i) => readElement(el, i))
    .filter((el): el is DesignElement => el !== undefined)
  return {
    version: num(r.version, DESIGN_VERSION),
    font: str(r.font, 'CabinSketch-Bold.ttf'),
    scrim: num(r.scrim, 220),
    background: str(r.background, 'background.jpg'),
    elements,
  }
}

/* ------------------------------------------------------------- utilities */

export function isText(el: DesignElement): el is TextElement {
  return el.kind === 'text'
}

export function isTile(el: DesignElement): el is TileElement {
  return el.kind === 'tile'
}

/** Replaces one element by id, returning a new document. */
export function withElement(design: Design, id: string, patch: Partial<DesignElement>): Design {
  return {
    ...design,
    elements: design.elements.map((el) =>
      el.id === id ? ({ ...el, ...patch } as DesignElement) : el,
    ),
  }
}

/** The topmost element containing the point, skipping locked and hidden ones. */
export function hitTest(design: Design, x: number, y: number): DesignElement | undefined {
  for (let i = design.elements.length - 1; i >= 0; i--) {
    const el = design.elements[i]
    if (!el || el.locked || el.hidden) continue
    if (x >= el.x && x <= el.x + el.w && y >= el.y && y <= el.y + el.h) return el
  }
  return undefined
}
