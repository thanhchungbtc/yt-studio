/**
 * Everything the UI needs to *describe* an artifact, kept separate from the
 * components that draw one. An asset row alone is a hash, a MIME type and a
 * size; these helpers rejoin it to its chapter and to the prompt or script that
 * produced it, so a preview needs no second request.
 */

import { chapterKey, taskKindRank } from './format'
import type { Asset, Chapter, Task, TaskKind, Video } from './types'

export type MediaType = 'image' | 'video' | 'audio' | 'text' | 'binary'

/** How an artifact should be *rendered*, as opposed to what produced it. */
export function mediaTypeOf(mime: string): MediaType {
  if (mime.startsWith('image/')) return 'image'
  if (mime.startsWith('video/')) return 'video'
  if (mime.startsWith('audio/')) return 'audio'
  if (mime === 'application/json' || mime.startsWith('text/')) return 'text'
  return 'binary'
}

/** A block of provenance shown beside the artifact — the prompt, the script. */
export interface ViewerNote {
  label: string
  body: string
  /** Rendered in the monospace face; true for anything model-authored. */
  mono?: boolean
}

/**
 * One entry in the viewer. Deliberately not an `Asset`: a chapter still is
 * viewable long before the artifacts list has been fetched, and the viewer
 * should not care which of the two it was handed.
 */
export interface ViewerItem {
  id: string
  kind: string
  mime: string
  title: string
  subtitle?: string
  size?: number
  createdAt?: string
  notes?: ViewerNote[]
  /**
   * The chapter this belongs to, or 0 for the artifacts the whole video owns.
   * Carried on the item rather than parsed back out of the subtitle, because the
   * gallery groups by it and a label is not an identifier.
   */
  ordinal?: number
  /**
   * The task that produced this artifact, when it is known. It is what lets the
   * viewer offer "run this step again" on the thing the operator is looking at,
   * rather than making them find the row in a table.
   */
  taskId?: string
  /**
   * Where a still sits: its chapter, and its slot within that chapter. This is
   * the coordinate that survives a redraw — the content address does not — so it
   * is what the viewer edits a prompt against and what it re-resolves the
   * picture by afterwards.
   */
  chapterId?: string
  slot?: number
  /**
   * The prompt this still was drawn from. Undefined where the chapter has no
   * prompt at this slot yet, which reads differently from an empty one: nothing
   * has produced it rather than someone cleared it.
   */
  prompt?: string
  /**
   * The thumbnail grid cell this icon draws. The icon counterpart of `slot`, and
   * separate from it because the two address different things: a cell belongs to
   * the video's grid, a slot to a chapter's stills.
   */
  cell?: number
  /**
   * A slot with no artifact behind it — the image task failed, or has not run.
   * Its id is a stand-in, so nothing may try to fetch, download or hash it. It
   * is shown anyway because its prompt is exactly what the operator wants to
   * change when a still did not come out.
   */
  pending?: boolean
}

/**
 * The stand-in address for a slot the pipeline has not filled. Prefixed rather
 * than left empty so it is a usable React key and is obviously not a hash.
 */
export function pendingStillId(chapterId: string, slot: number): string {
  return `pending:${chapterId}:${slot}`
}

/** The same stand-in for a grid cell no icon has been drawn into. */
export function pendingIconId(videoId: string, cell: number): string {
  return `pending:${videoId}:icon:${cell}`
}

/**
 * Which task kind produces which artifact kind. Both vocabularies are closed
 * sets, and this is the only place they meet.
 */
const KIND_TASKS: Record<string, TaskKind> = {
  blueprint: 'blueprint',
  script: 'script',
  prompt: 'image_prompts',
  audio: 'tts',
  image: 'image',
  clip: 'clip',
  final: 'concat',
  metadata: 'metadata',
  thumbnail_plan: 'thumbnail_plan',
  thumbnail_icon: 'thumbnail_icon',
  thumbnail: 'thumbnail',
}

/**
 * The artifact kind a stage of the pipeline produces — the same table as
 * KIND_TASKS read the other way, for the places that start from a task rather
 * than from a file.
 *
 * Two stages are absent because they leave nothing behind: prime_image_prompts
 * fills a cache the per-chapter reads serve from, and upload writes a receipt
 * onto the video rather than a file.
 */
export function artifactKindFor(kind: TaskKind): string | undefined {
  for (const [artifact, task] of Object.entries(KIND_TASKS)) {
    if (task === kind) return artifact
  }
  return undefined
}

/**
 * Where an artifact kind falls in the pipeline — the rank of the stage that
 * produced it. It is what lets a listing read script, narration, stills, clip
 * rather than alphabetically, which is the order the work actually happened in
 * and the order an operator reviews it.
 */
export function artifactKindRank(kind: string): number {
  const task = KIND_TASKS[kind]
  return task ? taskKindRank(task) : Number.MAX_SAFE_INTEGER
}

/**
 * Finds the task that produced an artifact, by the coordinates the DAG
 * addresses it with: kind, chapter ordinal, and the index within that chapter.
 *
 * Chapter-level artifacts carry an ordinal; video-level ones are -1. An index
 * of -1 matches whatever the task carries, which is what lets a caller that
 * does not know the slot still find the single task of its kind.
 */
export function producingTask(
  tasks: Task[],
  kind: string,
  ordinal: number,
  index: number,
): Task | undefined {
  const taskKind = KIND_TASKS[kind]
  if (!taskKind) return undefined
  return tasks.find(
    (t) => t.kind === taskKind && t.ordinal === ordinal && (index < 0 || t.index === index),
  )
}

/** The same lookup where only the id is wanted. */
export function producingTaskId(
  tasks: Task[],
  kind: string,
  ordinal: number,
  index: number,
): string | undefined {
  return producingTask(tasks, kind, ordinal, index)?.id
}

const KIND_TITLES: Record<string, string> = {
  blueprint: 'Blueprint',
  final: 'Final render',
  metadata: 'Publish metadata',
  thumbnail: 'Thumbnail',
  thumbnail_plan: 'Thumbnail plan',
  thumbnail_icon: 'Thumbnail icon',
  script: 'Script',
  prompt: 'Prompts',
  audio: 'Narration',
  clip: 'Clip',
  image: 'Still',
}

export function kindTitle(kind: string): string {
  return KIND_TITLES[kind] ?? kind
}

/** The first eight hex characters — enough to tell two artifacts apart. */
export function shortId(id: string): string {
  return id.slice(0, 8)
}

/*
  Mirrors entity.AssetKind's Ext and MIME. A viewer item can be built from a
  chapter alone — long before the artifacts list has been fetched — and those
  two tables are what let it say what the artifact is without asking.
*/

const KIND_EXT: Record<string, string> = {
  blueprint: '.json',
  metadata: '.json',
  thumbnail_plan: '.json',
  thumbnail_icon: '.png',
  script: '.txt',
  prompt: '.txt',
  audio: '.wav',
  image: '.png',
  thumbnail: '.png',
  clip: '.mp4',
  final: '.mp4',
}

const KIND_MIME: Record<string, string> = {
  blueprint: 'application/json',
  metadata: 'application/json',
  thumbnail_plan: 'application/json',
  thumbnail_icon: 'image/png',
  script: 'text/plain; charset=utf-8',
  prompt: 'text/plain; charset=utf-8',
  audio: 'audio/wav',
  image: 'image/png',
  thumbnail: 'image/png',
  clip: 'video/mp4',
  final: 'video/mp4',
}

export function kindMime(kind: string): string {
  return KIND_MIME[kind] ?? 'application/octet-stream'
}

/**
 * A filename an operator can find again on disk. The content address is kept
 * as the suffix so a downloaded file can still be traced back to the artifact
 * it came from.
 */
export function downloadName(item: ViewerItem): string {
  const slug =
    `${item.subtitle ?? ''} ${item.title}`
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '')
      .slice(0, 60) || item.kind
  return `${slug}-${shortId(item.id)}${KIND_EXT[item.kind] ?? '.bin'}`
}

/**
 * What a cell says, shown beside what it pictures. Read-only: the caption is the
 * plan's, and rewriting one tile's words without the others is how a grid stops
 * reading as one thing.
 */
function captionNote(video: Video | undefined, cell: number): ViewerNote[] {
  const caption = video?.thumbnailPlan[cell]?.caption
  return caption ? [{ label: 'Caption', body: caption }] : []
}

function notesFor(kind: string, chapter: Chapter | undefined): ViewerNote[] {
  if (!chapter) return []
  switch (kind) {
    // A still's prompt is not a note: it is editable, and generating from it is
    // how a still is redrawn. The viewer renders it as its own section.
    case 'image':
      return []
    case 'audio':
    case 'clip':
      return chapter.script ? [{ label: 'Narration script', body: chapter.script, mono: true }] : []
    default:
      return chapter.summary ? [{ label: 'Chapter summary', body: chapter.summary }] : []
  }
}

/**
 * Everything one chapter has produced, in the order it was produced: stills in
 * slot order, then the narration, then the rendered clip. Walking the viewer
 * with the arrow keys therefore walks a chapter end to end.
 */
export function chapterStillItems(
  chapter: Chapter,
  videoRef: string,
  tasks: Task[] = [],
): ViewerItem[] {
  const subtitle = `${chapterKey(videoRef, chapter.ordinal)} · ${chapter.title}`

  // Empty slots are kept rather than skipped. A still that failed or has not run
  // is the one whose prompt most wants changing, and dropping it here would put
  // that prompt out of reach of the only surface that edits it.
  const items: ViewerItem[] = chapter.imageAssetIds.map((id, slot) => ({
    id: id || pendingStillId(chapter.id, slot),
    kind: 'image',
    mime: kindMime('image'),
    title: `Still ${slot + 1}`,
    subtitle,
    notes: notesFor('image', chapter),
    taskId: producingTaskId(tasks, 'image', chapter.ordinal, slot),
    chapterId: chapter.id,
    slot,
    prompt: chapter.imagePrompts[slot],
    pending: !id,
  }))

  if (chapter.audioAssetId) {
    items.push({
      id: chapter.audioAssetId,
      kind: 'audio',
      mime: kindMime('audio'),
      title: kindTitle('audio'),
      subtitle,
      notes: notesFor('audio', chapter),
      taskId: producingTaskId(tasks, 'audio', chapter.ordinal, -1),
    })
  }
  if (chapter.clipAssetId) {
    items.push({
      id: chapter.clipAssetId,
      kind: 'clip',
      mime: kindMime('clip'),
      title: kindTitle('clip'),
      subtitle,
      notes: notesFor('clip', chapter),
      taskId: producingTaskId(tasks, 'clip', chapter.ordinal, -1),
    })
  }

  return items
}

/**
 * The thumbnail grid, cell by cell, in reading order.
 *
 * Built from the video row rather than from the asset list, for the same reason
 * a chapter's stills are: a cell that failed or has not run has no artifact, and
 * that is exactly the cell whose prompt wants changing.
 */
export function thumbnailCellItems(video: Video, tasks: Task[] = []): ViewerItem[] {
  const cells = Math.max(video.thumbnailPlan.length, video.thumbnailIconIds.length)
  return Array.from({ length: cells }, (_, cell) => {
    const id = video.thumbnailIconIds[cell] ?? ''
    return {
      id: id || pendingIconId(video.id, cell),
      kind: 'thumbnail_icon',
      mime: kindMime('thumbnail_icon'),
      title: `${kindTitle('thumbnail_icon')} ${cell + 1}`,
      subtitle: video.ref,
      notes: captionNote(video, cell),
      taskId: producingTaskId(tasks, 'thumbnail_icon', -1, cell),
      cell,
      prompt: video.thumbnailPlan[cell]?.prompt,
      pending: !id,
    }
  })
}

/**
 * Every artifact of a video, described. Sorted the way an operator reads the
 * video: what belongs to the whole thing first, then chapter by chapter.
 */
export function videoAssetItems(
  assets: Asset[],
  chapters: Chapter[],
  videoRef: string,
  tasks: Task[] = [],
  video?: Video,
): ViewerItem[] {
  const byId = new Map(chapters.map((chapter) => [chapter.id, chapter]))

  const items = assets.map((asset) => {
    const chapter = asset.chapterId ? byId.get(asset.chapterId) : undefined
    const slot = chapter ? chapter.imageAssetIds.indexOf(asset.id) : -1
    // An icon belongs to a grid cell rather than to a chapter, and the video row
    // is the only place that mapping exists. Without it every icon looks alike:
    // same title, same sort key, and — because producingTaskId matches any index
    // when given -1 — the same producing task, so re-running the fifth icon from
    // the gallery would re-run the first.
    const cell =
      asset.kind === 'thumbnail_icon' ? (video?.thumbnailIconIds.indexOf(asset.id) ?? -1) : -1
    const index = cell >= 0 ? cell : slot
    const label = kindTitle(asset.kind)
    const numbered = (asset.kind === 'image' && slot >= 0) || cell >= 0

    return {
      item: {
        id: asset.id,
        kind: asset.kind,
        mime: asset.mime,
        size: asset.size,
        createdAt: asset.createdAt,
        title: numbered ? `${label} ${index + 1}` : label,
        subtitle: chapter
          ? `${chapterKey(videoRef, chapter.ordinal)} · ${chapter.title}`
          : videoRef,
        ordinal: chapter?.ordinal ?? 0,
        notes: cell >= 0 ? captionNote(video, cell) : notesFor(asset.kind, chapter),
        taskId: producingTaskId(tasks, asset.kind, chapter?.ordinal ?? -1, index),
        // Carried so a still opened from the gallery edits its prompt exactly as
        // one opened from its chapter does.
        ...(asset.kind === 'image' && chapter && slot >= 0
          ? { chapterId: chapter.id, slot, prompt: chapter.imagePrompts[slot] }
          : {}),
        ...(cell >= 0 ? { cell, prompt: video?.thumbnailPlan[cell]?.prompt } : {}),
      } satisfies ViewerItem,
      ordinal: chapter?.ordinal ?? 0,
      // Grid order for icons, slot order for stills: both read the way the
      // artifact is laid out rather than the order the pool finished them in.
      slot: index < 0 ? 0 : index,
    }
  })

  items.sort(
    (a, b) =>
      a.ordinal - b.ordinal ||
      a.item.kind.localeCompare(b.item.kind) ||
      a.slot - b.slot ||
      (a.item.createdAt ?? '').localeCompare(b.item.createdAt ?? ''),
  )

  return items.map((entry) => entry.item)
}
