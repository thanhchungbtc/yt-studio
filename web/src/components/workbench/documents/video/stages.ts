import type { Chapter, Task } from '@/core/types'

/**
 * What a table cell is showing, independent of which column it is in.
 *
 * Every asset cell answers the same question — has this happened yet — so they
 * all resolve to the same five values and draw the same five shapes. That
 * uniformity is what makes a column scannable: you learn the vocabulary once and
 * then read the whole grid with it.
 */
export type CellState = 'done' | 'running' | 'queued' | 'pending' | 'failed'

export interface Cell {
  state: CellState
  /** Set when an input changed after this ran: intact, but unverified. */
  stale: boolean
  /** The task behind it, so a failed cell can offer to run it again. */
  task?: Task
}

/** One chapter's four stages, plus a slot-by-slot view of its slides. */
export interface ChapterStages {
  script: Cell
  narration: Cell
  /** Indexed by slot, so a half-drawn chapter says *which* slide is missing. */
  slides: Cell[]
  clip: Cell
}

/**
 * A task's state, mapped to a cell's. `succeeded` is deliberately *not* trusted
 * on its own — the artifact is what the cell draws, and a task can have
 * succeeded for a slot whose asset has since been cleared.
 */
function cellFor(task: Task | undefined, hasAsset: boolean): Cell {
  if (hasAsset) return { state: 'done', stale: task?.stale ?? false, ...(task ? { task } : {}) }
  if (!task) return { state: 'pending', stale: false }
  switch (task.state) {
    case 'failed':
      return { state: 'failed', stale: false, task }
    case 'running':
      return { state: 'running', stale: false, task }
    case 'ready':
    case 'awaiting_approval':
      return { state: 'queued', stale: false, task }
    default:
      return { state: 'pending', stale: false, task }
  }
}

/**
 * Indexes every task by the chapter and slot it belongs to, once per render of
 * the table rather than once per row. Forty rows each filtering a 300-task list
 * is forty passes for one answer.
 */
export function stagesByChapter(
  chapters: Chapter[],
  tasks: Task[],
  slidesPerChapter: number,
): Map<string, ChapterStages> {
  const script = new Map<string, Task>()
  const tts = new Map<string, Task>()
  const clip = new Map<string, Task>()
  const slides = new Map<string, Map<number, Task>>()

  for (const task of tasks) {
    if (!task.chapterId) continue
    switch (task.kind) {
      case 'script':
        script.set(task.chapterId, task)
        break
      case 'tts':
        tts.set(task.chapterId, task)
        break
      case 'clip':
        clip.set(task.chapterId, task)
        break
      case 'slide': {
        const bySlot = slides.get(task.chapterId) ?? new Map<number, Task>()
        bySlot.set(task.index, task)
        slides.set(task.chapterId, bySlot)
        break
      }
      default:
        break
    }
  }

  const result = new Map<string, ChapterStages>()
  for (const chapter of chapters) {
    const bySlot = slides.get(chapter.id)
    // The planned width comes from the video, not from what has been drawn, so
    // the slides cell has its final shape from the moment the blueprint lands
    // and nothing reflows underneath the cursor as images arrive.
    const width = Math.max(
      slidesPerChapter,
      chapter.slidePrompts.length,
      chapter.slideAssetIds.length,
    )

    result.set(chapter.id, {
      script: cellFor(script.get(chapter.id), chapter.script.length > 0),
      narration: cellFor(tts.get(chapter.id), Boolean(chapter.audioAssetId)),
      clip: cellFor(clip.get(chapter.id), Boolean(chapter.clipAssetId)),
      slides: Array.from({ length: width }, (_, slot) =>
        cellFor(bySlot?.get(slot), Boolean(chapter.slideAssetIds[slot])),
      ),
    })
  }
  return result
}

/** Words in a written script. The blueprint's estimate is what it is compared to. */
export function wordsIn(script: string): number {
  const trimmed = script.trim()
  return trimmed ? trimmed.split(/\s+/).length : 0
}

export interface ColumnTotals {
  script: { done: number; total: number }
  narration: { done: number; total: number }
  slides: { done: number; total: number }
  clip: { done: number; total: number }
  estimatedWords: number
}

/**
 * The counts in the column heads.
 *
 * This is the only arithmetic on screen, and it is the part that earns the
 * table: `SLIDES 24/80` in a header says how far one stage has got across the
 * whole video, which no individual row can tell you.
 */
export function columnTotals(chapters: Chapter[], slidesPerChapter: number): ColumnTotals {
  let script = 0
  let narration = 0
  let slides = 0
  let slideSlots = 0
  let clip = 0
  let estimatedWords = 0

  for (const chapter of chapters) {
    if (chapter.script.length > 0) script += 1
    if (chapter.audioAssetId) narration += 1
    if (chapter.clipAssetId) clip += 1
    estimatedWords += chapter.estimatedWords
    slides += chapter.slideAssetIds.filter(Boolean).length
    slideSlots += Math.max(slidesPerChapter, chapter.slidePrompts.length)
  }

  const total = chapters.length
  return {
    script: { done: script, total },
    narration: { done: narration, total },
    slides: { done: slides, total: slideSlots },
    clip: { done: clip, total },
    estimatedWords,
  }
}

/**
 * How wide a slide thumbnail can be, given a fixed column and a slot count.
 *
 * The column keeps one width whatever the video's slide count, so the table
 * never has a ragged right edge — the pictures shrink instead. Below the floor
 * a thumbnail stops being a picture and the cell switches to a state strip.
 */
export const SLIDES_COLUMN = 190
const SLIDE_GAP = 4
const SLIDE_PADDING = 16
const SLIDE_MAX = 78
const SLIDE_MIN = 40

export function slideThumbWidth(count: number): number | null {
  if (count <= 0) return null
  const available = SLIDES_COLUMN - SLIDE_PADDING - (count - 1) * SLIDE_GAP
  const width = Math.min(SLIDE_MAX, Math.floor(available / count))
  return width >= SLIDE_MIN ? width : null
}
