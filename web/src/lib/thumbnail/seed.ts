/**
 * Seeds a document from the builtin renderer's layout.
 *
 * The grid is a constraint solver in Go: tiles are sized from the frame width
 * first and the headline is fitted into whatever height is left, which is what
 * makes the result hold up at four cells or twenty. Reproducing it here means
 * the editor opens on the thumbnail the pipeline would have produced, and the
 * operator starts by adjusting something real.
 *
 * From that point the document is free — elements carry absolute positions and
 * nothing re-solves them. That is the trade the browser editor makes: give up
 * the automatic layout, get direct manipulation, and be exact about what
 * publishes because the browser's own output is the artifact.
 */

import type { Design, DesignElement, TextElement, TileElement } from './doc'
import { DESIGN_VERSION, FRAME_HEIGHT, FRAME_WIDTH } from './doc'
import { STYLE, trackingFor } from './style'

/** Measures a string's drawn width. Supplied by the caller so this file stays
 *  free of the canvas, and so the same fitting runs against the real face. */
export type Measure = (text: string, fontSize: number, tracking: number) => number

export interface SeedInput {
  headline: string
  /** One per grid cell, in reading order. */
  cells: { caption: string; assetId?: string }[]
  rows: number
  font: string
}

/* ----------------------------------------------------------------- grid */

interface Grid {
  tileSize: number
  rows: number
  /** How many tiles each row holds; an uneven grid centres the remainder. */
  counts: number[]
  rowX: number[]
  rowY: number[]
}

function blockHeight(rows: number, tile: number): number {
  return (
    rows * (tile + STYLE.tileToCaptionGap + STYLE.captionRowHeight) + (rows - 1) * STYLE.tileSpacing
  )
}

/**
 * Sizes the tiles from the frame width; where the block sits is decided once
 * the headline is fitted. Width first, so the tiles always span the frame and
 * it is the headline that gives way — sizing from leftover height leaves small
 * tiles stranded between wide empty gutters.
 */
function layOutGrid(cells: number, rows: number): Grid {
  rows = Math.max(1, Math.min(rows, cells))
  const cols = Math.ceil(cells / rows)

  let tile = Math.floor(
    (FRAME_WIDTH - 2 * STYLE.gridSideMargin - (cols - 1) * STYLE.tileSpacing) / cols,
  )
  // The one case where the tiles give way instead: a grid so tall the headline
  // would not get its floor.
  while (
    tile > 1 &&
    FRAME_HEIGHT - blockHeight(rows, tile) - STYLE.gridBottomMargin - STYLE.headlineToGridGap <
      STYLE.headlineTopMargin + STYLE.headlineFontMin
  ) {
    tile -= 2
  }
  tile = Math.max(tile, 1)

  const counts: number[] = []
  const rowX: number[] = []
  let remaining = cells
  for (let r = 0; r < rows; r++) {
    const n = Math.min(cols, remaining)
    remaining -= n
    counts.push(n)
    const rowWidth = n * tile + (n - 1) * STYLE.tileSpacing
    rowX.push(Math.floor((FRAME_WIDTH - rowWidth) / 2))
  }
  const grid: Grid = { tileSize: tile, rows, counts, rowX, rowY: [] }
  placeGrid(grid, 0)
  return grid
}

/**
 * Centres the block in what the headline left rather than pinning it to the
 * bottom, where a grid of small tiles would leave a band of empty background
 * that reads as a mistake.
 */
function placeGrid(grid: Grid, headlineBottom: number): void {
  const bandTop = Math.max(headlineBottom + STYLE.headlineToGridGap, STYLE.headlineTopMargin)
  const bandBottom = FRAME_HEIGHT - STYLE.gridBottomMargin
  const block = blockHeight(grid.rows, grid.tileSize)

  const top = bandTop + Math.max(bandBottom - bandTop - block, 0) / 2
  grid.rowY = []
  for (let r = 0; r < grid.rows; r++) {
    grid.rowY.push(
      Math.round(
        top +
          r * (grid.tileSize + STYLE.tileToCaptionGap + STYLE.captionRowHeight + STYLE.tileSpacing),
      ),
    )
  }
}

function headlineBudget(grid: Grid): number {
  return (
    FRAME_HEIGHT -
    STYLE.gridBottomMargin -
    blockHeight(grid.rows, grid.tileSize) -
    STYLE.headlineToGridGap -
    STYLE.headlineTopMargin
  )
}

function tileRect(grid: Grid, i: number): { x: number; y: number } {
  let row = grid.rows - 1
  let col = 0
  let n = i
  for (let r = 0; r < grid.rows; r++) {
    const count = grid.counts[r] ?? 0
    if (n < count) {
      row = r
      col = n
      break
    }
    n -= count
  }
  return {
    x: (grid.rowX[row] ?? 0) + col * (grid.tileSize + STYLE.tileSpacing),
    y: grid.rowY[row] ?? 0,
  }
}

/* ------------------------------------------------------------- headline */

interface FittedHeadline {
  lines: string[]
  size: number
  lineHeight: number
}

function wrap(words: string[], size: number, tracking: number, measure: Measure): string[] {
  const maxWidth = FRAME_WIDTH - 2 * STYLE.headlineSideMargin
  const lines: string[] = []
  let current = ''
  for (const w of words) {
    const candidate = current === '' ? w : `${current} ${w}`
    if (measure(candidate, size, tracking) <= maxWidth || current === '') {
      current = candidate
      continue
    }
    lines.push(current)
    current = w
  }
  if (current !== '') lines.push(current)
  return lines
}

/**
 * Fits the hook into the band the grid left above it: as large as it goes on
 * one line, wrapping only when even the floor will not hold it.
 *
 * Every size is tried at one line before any is tried at two, so
 * small-and-single beats large-and-wrapped — wrapping costs the grid a third of
 * its height.
 */
function fitHeadline(headline: string, maxHeight: number, measure: Measure): FittedHeadline {
  const words = headline.toUpperCase().split(/\s+/).filter(Boolean)
  if (words.length === 0) return { lines: [], size: STYLE.headlineFontMin, lineHeight: 0 }

  for (let maxLines = 1; maxLines <= STYLE.headlineMaxLines; maxLines++) {
    for (
      let size = STYLE.headlineFontMax;
      size >= STYLE.headlineFontMin;
      size -= STYLE.headlineFontStep
    ) {
      const lines = wrap(words, size, trackingFor(size), measure)
      if (lines.length > maxLines) continue
      const lineHeight = size + STYLE.headlineLineGap
      if (lines.length * lineHeight <= maxHeight) return { lines, size, lineHeight }
    }
  }
  // Neither bound can be met: drawn at the floor and allowed to overflow. A
  // clipped word beats a video that cannot produce a thumbnail.
  const size = STYLE.headlineFontMin
  const lines = wrap(words, size, trackingFor(size), measure).slice(0, STYLE.headlineMaxLines)
  return { lines, size, lineHeight: size + STYLE.headlineLineGap }
}

/* ----------------------------------------------------------------- seed */

/** Picks one size for the whole grid: the largest at which every caption fits.
 *  Sizing each caption to its own tile reads as a dozen unrelated pictures. */
function fitCaptions(captions: string[], tileSize: number, measure: Measure): number {
  for (let size = STYLE.captionFontMax; size >= STYLE.captionFontMin; size--) {
    if (captions.every((c) => measure(c, size, 0) <= tileSize)) return size
  }
  return STYLE.captionFontMin
}

export function seedDesign(input: SeedInput, measure: Measure): Design {
  const cells = input.cells
  const grid = layOutGrid(Math.max(cells.length, 1), input.rows)
  const headline = fitHeadline(input.headline, headlineBudget(grid), measure)
  placeGrid(grid, STYLE.headlineTopMargin + headline.lines.length * headline.lineHeight)

  const captionSize = fitCaptions(
    cells.map((c) => c.caption),
    grid.tileSize,
    measure,
  )

  const elements: DesignElement[] = []

  const headlineElement: TextElement = {
    id: 'headline',
    kind: 'text',
    x: STYLE.headlineSideMargin,
    y: STYLE.headlineTopMargin,
    w: FRAME_WIDTH - 2 * STYLE.headlineSideMargin,
    h: Math.max(headline.lines.length, 1) * headline.lineHeight,
    text: input.headline,
    fontSize: headline.size,
    color: STYLE.headlineColor,
    align: 'center',
    tracking: trackingFor(headline.size),
    lineGap: STYLE.headlineLineGap,
    uppercase: true,
    strokeColor: '#000000',
    strokeWidth: 0,
    shadowColor: 'rgba(0, 0, 0, 0.55)',
    shadowBlur: 0,
    shadowY: 0,
  }
  elements.push(headlineElement)

  cells.forEach((cell, i) => {
    const at = tileRect(grid, i)
    const tile: TileElement = {
      id: `cell-${i}`,
      kind: 'tile',
      x: at.x,
      y: at.y,
      w: grid.tileSize,
      h: grid.tileSize,
      assetId: cell.assetId,
      caption: cell.caption,
      captionSize,
      captionColor: STYLE.captionColor,
      fillColor: STYLE.tileFillColor,
      borderColor: STYLE.tileBorderColor,
      borderWidth: STYLE.tileBorderWidth,
      radius: STYLE.tileCornerRadius,
      padding: STYLE.tileIconPadding,
      keyBelow: STYLE.iconKeyBelow,
      keyAbove: STYLE.iconKeyAbove,
    }
    elements.push(tile)
  })

  return {
    version: DESIGN_VERSION,
    font: input.font,
    scrim: STYLE.scrim,
    background: 'background.jpg',
    elements,
  }
}
