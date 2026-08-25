import type { Chapter, Task, TaskKind } from '../../../core/types'

/**
 * What a cell is showing, independent of which column it is in.
 *
 * Every cell in the grid answers the same question — *has this happened yet* —
 * so they all resolve to the same four values and draw the same four shapes.
 * That uniformity is what earns the table: you learn the vocabulary once and
 * then read the whole thing with it.
 *
 * Four and not five. There used to be `queued` and `pending` — a task that is
 * ready to run, and one still blocked behind its inputs. The distinction is
 * real in the scheduler and invisible to a person watching: both mean *not
 * yet*, and telling two near-identical dots apart is work the table was
 * supposed to save. The header count says how far along it is; the cell only
 * has to say which of four things is true.
 */
export type CellState = 'done' | 'running' | 'waiting' | 'failed'

export interface Cell {
  state: CellState
  /** An input changed after this ran: intact, but unverified. */
  stale: boolean
  /** The task behind it, so a cell can say why it failed. */
  task?: Task
}

/** One chapter's four stages, with the slides slot by slot. */
export interface ChapterStages {
  script: Cell
  narration: Cell
  /** Indexed by slot, so a half-drawn chapter says *which* slide is missing. */
  slides: Cell[]
  clip: Cell
}

/**
 * A task's state, mapped to a cell's.
 *
 * `succeeded` is deliberately not trusted on its own. The artifact is what the
 * cell is about, and a task can have succeeded for a slot whose asset has since
 * been cleared — so the artifact is asked first and the task only explains what
 * is happening in its absence.
 */
function cellFor(task: Task | undefined, hasArtifact: boolean): Cell {
  if (hasArtifact) return { state: 'done', stale: task?.stale ?? false, ...(task ? { task } : {}) }
  if (!task) return { state: 'waiting', stale: false }
  switch (task.state) {
    case 'failed':
      return { state: 'failed', stale: false, task }
    case 'running':
      return { state: 'running', stale: false, task }
    default:
      return { state: 'waiting', stale: false, task }
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
  const narration = new Map<string, Task>()
  const clip = new Map<string, Task>()
  const slides = new Map<string, Map<number, Task>>()

  for (const task of tasks) {
    if (!task.chapterId) continue
    switch (task.kind) {
      case 'script':
        script.set(task.chapterId, task)
        break
      case 'tts':
        narration.set(task.chapterId, task)
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
      narration: cellFor(narration.get(chapter.id), Boolean(chapter.audioAssetId)),
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

/**
 * The stages that belong to the video rather than to any chapter.
 *
 * They are the FINAL strip. Keyed by artifact rather than by task for the same
 * reason a cell is: the question is whether the thing exists.
 */
export function videoStage(tasks: Task[], kinds: TaskKind[], hasArtifact: boolean): Cell {
  // The newest task of the kind wins; a re-run leaves the earlier one behind.
  let latest: Task | undefined
  for (const task of tasks) {
    if (task.chapterId || !kinds.includes(task.kind)) continue
    if (!latest || task.updatedAt > latest.updatedAt) latest = task
  }
  return cellFor(latest, hasArtifact)
}

export interface ColumnTotals {
  script: { done: number; total: number }
  narration: { done: number; total: number }
  slides: { done: number; total: number }
  clip: { done: number; total: number }
  /** Written, and what the blueprint budgeted for. */
  words: number
  estimatedWords: number
  seconds: number
}

/**
 * The counts in the column heads.
 *
 * This is the only arithmetic on screen, and it is the rest of what earns the
 * table: `SLIDES 24/80` says how far one stage has got across the whole video,
 * which no individual row can tell you.
 */
export function columnTotals(chapters: Chapter[], slidesPerChapter: number): ColumnTotals {
  const totals: ColumnTotals = {
    script: { done: 0, total: chapters.length },
    narration: { done: 0, total: chapters.length },
    slides: { done: 0, total: 0 },
    clip: { done: 0, total: chapters.length },
    words: 0,
    estimatedWords: 0,
    seconds: 0,
  }

  for (const chapter of chapters) {
    if (chapter.script.length > 0) totals.script.done += 1
    if (chapter.audioAssetId) totals.narration.done += 1
    if (chapter.clipAssetId) totals.clip.done += 1
    totals.slides.total += Math.max(
      slidesPerChapter,
      chapter.slidePrompts.length,
      chapter.slideAssetIds.length,
    )
    totals.slides.done += chapter.slideAssetIds.filter(Boolean).length
    totals.words += wordsIn(chapter.script)
    totals.estimatedWords += chapter.estimatedWords
    totals.seconds += chapter.durationSeconds
  }
  return totals
}
