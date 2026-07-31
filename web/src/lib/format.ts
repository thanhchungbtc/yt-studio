import type { PoolName, Task, TaskKind, TaskState, VideoState } from './types'

/**
 * The pipeline in the order the DAG runs it. The scheduler has no stage
 * barriers, so this is a reading order rather than a schedule — but it is the
 * order an operator thinks in, and every list that groups by kind sorts by it.
 */
export const TASK_KIND_ORDER: TaskKind[] = [
  'blueprint',
  'prime_image_prompts',
  'image_prompts',
  'script',
  'tts',
  'image',
  'clip',
  'concat',
  'metadata',
  'thumbnail_plan',
  'thumbnail_icon',
  'thumbnail',
  'upload',
]

const KIND_RANK = new Map(TASK_KIND_ORDER.map((kind, i) => [kind, i]))

export function taskKindRank(kind: TaskKind): number {
  return KIND_RANK.get(kind) ?? TASK_KIND_ORDER.length
}

export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '0s'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

export function formatClock(seconds: number): string {
  const total = Math.max(0, Math.floor(seconds))
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  const pad = (n: number) => String(n).padStart(2, '0')
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let value = bytes / 1024
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value < 10 ? value.toFixed(1) : Math.round(value)} ${units[unit]}`
}

export function formatRelative(iso: string | undefined): string {
  if (!iso) return '—'
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return '—'
  const delta = Math.round((Date.now() - then) / 1000)
  if (delta < 5) return 'just now'
  if (delta < 60) return `${delta}s ago`
  if (delta < 3600) return `${Math.floor(delta / 60)}m ago`
  if (delta < 86400) return `${Math.floor(delta / 3600)}h ago`
  return `${Math.floor(delta / 86400)}d ago`
}

export function formatAbsolute(iso: string | undefined): string {
  if (!iso) return '—'
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

const TASK_LABELS: Record<TaskKind, string> = {
  blueprint: 'Blueprint',
  prime_image_prompts: 'Prime prompts',
  image_prompts: 'Prompts',
  script: 'Script',
  tts: 'Narration',
  image: 'Still',
  clip: 'Clip',
  concat: 'Concat',
  metadata: 'Metadata',
  thumbnail_plan: 'Thumbnail plan',
  thumbnail_icon: 'Thumbnail icon',
  thumbnail: 'Thumbnail',
  upload: 'Upload',
}

export function taskLabel(kind: TaskKind): string {
  return TASK_LABELS[kind] ?? kind
}

const POOL_LABELS: Record<PoolName, string> = {
  llm: 'LLM',
  tts: 'TTS',
  image: 'Image',
  compose: 'Compose',
  cache: 'Cache',
  upload: 'Upload',
}

export function poolLabel(pool: PoolName): string {
  return POOL_LABELS[pool] ?? pool
}

const VIDEO_STATE_LABELS: Record<VideoState, string> = {
  draft: 'Draft',
  running: 'Running',
  awaiting_approval: 'Awaiting approval',
  blocked: 'Blocked',
  completed: 'Completed',
  failed: 'Failed',
  cancelled: 'Cancelled',
}

export function videoStateLabel(state: VideoState): string {
  return VIDEO_STATE_LABELS[state] ?? state
}

const TASK_STATE_LABELS: Record<TaskState, string> = {
  blocked: 'Blocked',
  ready: 'Ready',
  running: 'Running',
  awaiting_approval: 'Gated',
  succeeded: 'Done',
  failed: 'Failed',
  cancelled: 'Cancelled',
}

export function taskStateLabel(state: TaskState): string {
  return TASK_STATE_LABELS[state] ?? state
}

/** Human label for a chapter within its video, e.g. DSS-14#7. */
export function chapterKey(ref: string, ordinal: number): string {
  return `${ref}#${ordinal}`
}

export function percent(done: number, total: number): number {
  if (total <= 0) return 0
  return Math.min(100, Math.round((done / total) * 100))
}

/**
 * How long a task took, in seconds — or how long it has been going, when it is
 * still running. `now` is passed in rather than read here so a table of two
 * hundred rows shares one clock and re-renders on one tick.
 */
export function taskSeconds(task: Task, now: number): number | undefined {
  if (!task.startedAt) return undefined
  const started = new Date(task.startedAt).getTime()
  if (Number.isNaN(started)) return undefined
  const ended = task.finishedAt ? new Date(task.finishedAt).getTime() : now
  if (Number.isNaN(ended)) return undefined
  return Math.max(0, (ended - started) / 1000)
}

/** A duration at column width: 940ms, 12s, 4m 20s, 1h 06m. */
export function formatCompactDuration(seconds: number | undefined): string {
  if (seconds === undefined) return '—'
  if (seconds < 1) return `${Math.round(seconds * 1000)}ms`
  if (seconds < 60) return `${seconds < 10 ? seconds.toFixed(1) : Math.round(seconds)}s`
  const m = Math.floor(seconds / 60)
  const s = Math.round(seconds % 60)
  if (m < 60) return `${m}m ${String(s).padStart(2, '0')}s`
  return `${Math.floor(m / 60)}h ${String(m % 60).padStart(2, '0')}m`
}
