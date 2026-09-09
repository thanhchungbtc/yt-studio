import type { Chapter, GateKind, Task, TaskKind, Video } from '../../../core/types'

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
 * Whether a task has finished with the run it is on.
 *
 * Not the same question as whether it worked. `failed` is settled — the task has
 * stopped and is waiting for a person. `blocked` and `ready` are not: they are a
 * task queued to run, which after a re-run is a task queued to run *again*.
 */
function settled(state: Task['state']): boolean {
  return (
    state === 'succeeded' ||
    state === 'failed' ||
    state === 'cancelled' ||
    state === 'awaiting_approval'
  )
}

/**
 * A task's state, mapped to a cell's.
 *
 * Work in flight is asked about first, and that is what makes Regenerate
 * visible. A re-run leaves the old artifact where it is while it produces the
 * new one — not cascading is the whole point of it — so a cell that asked the
 * artifact first would stay green and settled with a task running underneath,
 * and pressing Regenerate would look like it had done nothing. An artifact plus
 * an unfinished task can only mean one thing: it is being made again.
 *
 * Past that, `succeeded` is deliberately not trusted on its own. The artifact is
 * what the cell is about, and a task can have succeeded for a slot whose asset
 * has since been cleared — so the artifact is asked next and the task only
 * explains what is happening in its absence.
 */
function cellFor(task: Task | undefined, hasArtifact: boolean): Cell {
  if (task && !settled(task.state)) {
    return { state: task.state === 'running' ? 'running' : 'waiting', stale: false, task }
  }
  if (hasArtifact) return { state: 'done', stale: task?.stale ?? false, ...(task ? { task } : {}) }
  if (!task) return { state: 'waiting', stale: false }
  return { state: task.state === 'failed' ? 'failed' : 'waiting', stale: false, task }
}

/** The one verb a dot carries, when it carries one. */
export type CellAction = 'rerun' | 'retry'

/**
 * What clicking a dot offers, or nothing when it offers nothing.
 *
 * Only the two settled outcomes are actionable, and that rule is also what keeps
 * the grid learnable: a dot you can click is a dot with a result you might
 * disagree with. Waiting and running have no result yet, and a menu whose one
 * item is greyed out is worse than no menu.
 *
 * The two are different operations rather than one operation with two labels.
 * A failed task cascades, because nothing under it holds an artifact worth
 * keeping; a done one must not, which is the whole reason to have this at all.
 */
export function cellAction(cell: Cell): CellAction | null {
  if (!cell.task) return null
  // Expansion is one-way, so the scheduler refuses a second roll of the outline
  // unless the first one failed. Better to offer nothing than an item that
  // exists in order to produce an error.
  if (cell.task.kind === 'blueprint' && cell.task.state !== 'failed') return null
  if (cell.state === 'failed') return 'retry'
  if (cell.state === 'done') return 'rerun'
  return null
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

/**
 * The narration speed the blueprint budgets with.
 *
 * Mirrors `entity.DefaultWordsPerMinute`. Duplicated rather than fetched
 * because it is a constant of the *plan*, not a setting — and the cost of it
 * drifting is a projected runtime that reads a little long or a little short,
 * which is a projection either way. Nothing downstream depends on it.
 */
export const NARRATION_WPM = 130

/** How long a word count is expected to take to read aloud. */
export function projectedSeconds(words: number): number {
  return (words / NARRATION_WPM) * 60
}

/**
 * How long a chapter runs: measured where there is audio, projected from the
 * script where there is not.
 *
 * The measurement wins whenever it exists, and the fallback is explicitly a
 * projection rather than a stored number pretending to be one. Reading the two
 * off a single column is what put a word count behind a seek bar — on a real
 * video the projection ran a third long, which is fine for a plan and useless
 * for an offset.
 */
export function chapterSeconds(chapter: Chapter): number {
  return chapter.audioDurationSeconds > 0
    ? chapter.audioDurationSeconds
    : projectedSeconds(wordsIn(chapter.script))
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
    totals.seconds += chapterSeconds(chapter)
  }
  return totals
}

/* ---------------------------------------------------------------- pipeline */

/**
 * The ten stages a video passes through, as a person names them.
 *
 * Thirteen `TaskKind`s collapse to ten rows. `prime_slide_prompts` and
 * `slide_prompts` are one stage because the split is the scheduler's business,
 * and the three thumbnail kinds are one for the same reason — a person waiting
 * for a thumbnail is waiting for a thumbnail, not for a plan, an icon and a
 * composite in sequence.
 */
export type StageId =
  | 'blueprint'
  | 'slide_prompts'
  | 'script'
  | 'narration'
  | 'slides'
  | 'clips'
  | 'cut'
  | 'metadata'
  | 'thumbnail'
  | 'upload'

/**
 * The kinds each stage is made of, named once.
 *
 * The rows below need this to draw a mark, and anything acting on a stage as a
 * whole needs the same list to find the tasks under it. Two copies of "which
 * kinds are the thumbnail" is one copy too many: the collapse of thirteen kinds
 * into ten rows is the fact this table exists to state.
 */
export const STAGE_KINDS: Record<StageId, TaskKind[]> = {
  blueprint: ['blueprint'],
  slide_prompts: ['prime_slide_prompts', 'slide_prompts'],
  script: ['script'],
  narration: ['tts'],
  slides: ['slide'],
  clips: ['clip'],
  cut: ['concat'],
  metadata: ['metadata'],
  thumbnail: ['thumbnail', 'thumbnail_plan', 'thumbnail_icon'],
  upload: ['upload'],
}

export interface PipelineStage {
  id: StageId
  label: string
  /** The same four shapes the grid uses, aggregated over the whole stage. */
  cell: Cell
  /**
   * Present only on the stages that repeat per chapter or per slide. A single
   * stage carries nothing, because the mark has already said `done` and
   * printing `1/1` beside a filled disc is saying it twice.
   */
  count?: { done: number; total: number }
  /** Set when this stage is the one holding a gate open. */
  gate?: GateKind
}

/**
 * A stage's mark, from the tasks under it.
 *
 * Artifact-first, exactly as a cell is: when every slot has produced something
 * the stage is done, whatever the tasks say. A task that failed and was retried
 * into success leaves a `failed` row behind it, and a stage that reported red
 * over a complete set of artifacts would be reporting its own history rather
 * than the state of the video.
 */
function aggregate(tasks: Task[], kinds: TaskKind[], done: number, total: number): Cell {
  const mine = tasks.filter((task) => kinds.includes(task.kind))
  if (total > 0 && done >= total) {
    return { state: 'done', stale: mine.some((task) => task.stale) }
  }
  const failed = mine.find((task) => task.state === 'failed')
  if (failed) return { state: 'failed', stale: false, task: failed }
  const running = mine.find((task) => task.state === 'running')
  if (running) return { state: 'running', stale: false, task: running }
  return { state: 'waiting', stale: false }
}

/**
 * Which gate a paused task is holding, when the field is empty.
 *
 * `gate` is what the server says and is trusted first. The fallback exists
 * because only two kinds ever wait for a person, so a task that has stopped at
 * `awaiting_approval` without saying which gate it is has still told us.
 */
function gateKindOf(task: Task): GateKind | undefined {
  if (task.gate === 'blueprint' || task.gate === 'upload') return task.gate
  if (task.kind === 'blueprint') return 'blueprint'
  if (task.kind === 'upload') return 'upload'
  return undefined
}

/**
 * The pipeline, top to bottom, aggregated across the whole video.
 *
 * This is the grid rotated. The table reads chapters down and stages across, so
 * *a stage's* progress is a column you have to count; here the stages are the
 * rows and the counting is done. The two answer different questions — how is
 * chapter 2 doing, and how far along is the video — from the same tasks.
 *
 * Denominators come from the video, never from the rows that happen to exist
 * yet. `chapterCount` is known the moment a video is created, so the ten rows
 * have their final shape before the blueprint has written a single chapter; if
 * the totals were counted off `chapters` the panel would read `0/0` at the one
 * moment it is being watched hardest, and understate the work as it filled in.
 */
export function pipelineStages(video: Video, chapters: Chapter[], tasks: Task[]): PipelineStage[] {
  const totals = columnTotals(chapters, video.slidesPerChapter)

  // `max` rather than the video alone: a blueprint is allowed to come back with
  // more chapters than were asked for, and the count has to be the truth.
  const perChapter = Math.max(video.chapterCount, chapters.length)
  const slideTotal = Math.max(video.chapterCount * video.slidesPerChapter, totals.slides.total)
  const promptsDone = chapters.filter((chapter) => chapter.slidePrompts.length > 0).length

  const paused = tasks.find((task) => task.state === 'awaiting_approval')
  const gate = paused ? gateKindOf(paused) : undefined

  const single = (kinds: TaskKind[], hasArtifact: boolean) => videoStage(tasks, kinds, hasArtifact)

  const stages: PipelineStage[] = [
    {
      id: 'blueprint',
      label: 'Blueprint',
      cell: single(STAGE_KINDS.blueprint, Boolean(video.blueprintAssetId)),
    },
    {
      id: 'slide_prompts',
      label: 'Slide prompts',
      cell: aggregate(tasks, STAGE_KINDS.slide_prompts, promptsDone, perChapter),
      count: { done: promptsDone, total: perChapter },
    },
    {
      id: 'script',
      label: 'Script',
      cell: aggregate(tasks, STAGE_KINDS.script, totals.script.done, perChapter),
      count: { done: totals.script.done, total: perChapter },
    },
    {
      id: 'narration',
      label: 'Narration',
      cell: aggregate(tasks, STAGE_KINDS.narration, totals.narration.done, perChapter),
      count: { done: totals.narration.done, total: perChapter },
    },
    {
      id: 'slides',
      label: 'Slides',
      cell: aggregate(tasks, STAGE_KINDS.slides, totals.slides.done, slideTotal),
      count: { done: totals.slides.done, total: slideTotal },
    },
    {
      id: 'clips',
      label: 'Clips',
      cell: aggregate(tasks, STAGE_KINDS.clips, totals.clip.done, perChapter),
      count: { done: totals.clip.done, total: perChapter },
    },
    { id: 'cut', label: 'Cut', cell: single(STAGE_KINDS.cut, Boolean(video.finalAssetId)) },
    {
      id: 'metadata',
      label: 'Metadata',
      cell: single(STAGE_KINDS.metadata, Boolean(video.metadata?.title)),
    },
    {
      id: 'thumbnail',
      label: 'Thumbnail',
      cell: single(STAGE_KINDS.thumbnail, Boolean(video.effectiveThumbnailAssetId)),
    },
    { id: 'upload', label: 'Upload', cell: single(STAGE_KINDS.upload, Boolean(video.upload)) },
  ]

  if (!gate) return stages
  return stages.map((stage) => (stage.id === gate ? { ...stage, gate } : stage))
}
