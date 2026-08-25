/**
 * The slice of the API v2 actually reads.
 *
 * V2 is self-contained by rule, so it carries its own view of the wire format
 * rather than importing v1's. It is a narrowing, not a fork: every field here
 * is a field the server already sends, and anything v2 does not render is left
 * out until a screen needs it.
 */

export type VideoState =
  'draft' | 'running' | 'awaiting_approval' | 'blocked' | 'completed' | 'failed' | 'cancelled'

export type TaskState =
  'blocked' | 'ready' | 'running' | 'awaiting_approval' | 'succeeded' | 'failed' | 'cancelled'

export type TaskKind =
  | 'blueprint'
  | 'prime_slide_prompts'
  | 'slide_prompts'
  | 'script'
  | 'tts'
  | 'slide'
  | 'clip'
  | 'concat'
  | 'metadata'
  | 'thumbnail_plan'
  | 'thumbnail_icon'
  | 'thumbnail'
  | 'upload'

/** The two moments the pipeline stops and waits for a person. */
export type GateKind = 'blueprint' | 'upload'

export interface Channel {
  id: string
  slug: string
  name: string
  description: string
  credentials: 'missing' | 'valid' | 'expired'
  updatedAt: string
}

export interface TaskCounts {
  total: number
  succeeded: number
  failed: number
  running: number
  ready: number
  blocked: number
  awaitingApproval: number
  /** Cuts across the counts above: a stale task is usually a succeeded one. */
  stale: number
  cancelled: number
}

export interface Task {
  id: string
  videoId: string
  chapterId?: string
  kind: TaskKind
  /** The chapter's position; 0 for the tasks that belong to the video itself. */
  ordinal: number
  /** The slot within a kind — which slide, which thumbnail cell. */
  index: number
  state: TaskState
  gate?: GateKind | ''
  attempt: number
  /** An input changed after this ran; the artifact is intact but unverified. */
  stale: boolean
  error?: string
  updatedAt: string
}

export interface Chapter {
  id: string
  videoId: string
  ordinal: number
  title: string
  summary: string
  script: string
  slidePrompts: string[]
  audioAssetId?: string
  slideAssetIds: string[]
  clipAssetId?: string
  durationSeconds: number
  estimatedWords: number
  updatedAt: string
}

export interface Metadata {
  title: string
  description: string
  tags: string[]
}

export interface UploadRecord {
  url: string
  dryRun: boolean
  uploadedAt: string
}

export interface Video {
  id: string
  channelId: string
  /** The stable human-facing key — `DSS-2` — used in every route. */
  ref: string
  title: string
  topic: string
  state: VideoState
  chapterCount: number
  slidesPerChapter: number
  targetDurationMinutes: number
  finalAssetId?: string
  /** The thumbnail that will actually publish: the override if there is one. */
  effectiveThumbnailAssetId?: string
  metadata?: Metadata
  upload?: UploadRecord
  error?: string
  counts: TaskCounts
  updatedAt: string
}

/* ------------------------------------------------------------------ events */

/**
 * The frames on the wire.
 *
 * A delta is a *patch*, not a record: it carries the handful of fields that can
 * change while something is running and nothing else. That is why applying one
 * is a merge rather than a replace, and why a few of them end in a refetch —
 * see the notes in `events.ts` for which, and why.
 */

export interface TaskDelta {
  id: string
  videoId: string
  chapterId?: string
  kind: TaskKind
  ordinal: number
  index: number
  state: TaskState
  gate?: GateKind | ''
  attempt: number
  stale: boolean
  error?: string
  updatedAt: string
}

export interface VideoDelta {
  id: string
  state: VideoState
  done: number
  total: number
  failed: number
  running: number
  error?: string
  updatedAt: string
}

export interface ChapterDelta {
  id: string
  videoId: string
  ordinal: number
  title: string
  /** The script *body* is not on the wire; this says one has arrived. */
  hasScript: boolean
  audioAssetId?: string
  slideAssetIds?: string[]
  clipAssetId?: string
  updatedAt: string
}

export interface PoolStat {
  pool: string
  limit: number
  inFlight: number
  queued: number
}

/**
 * How busy the machine is, as a whole.
 *
 * Nothing in v2 renders this yet. It is typed anyway because the stream already
 * delivers it and `subscribeStream` already hands it to observers — a frame
 * whose type denies a field it carries is a lie waiting for the first person
 * who tries to read it.
 */
export interface SchedulerDelta {
  pools: PoolStat[]
  ready: number
  running: number
  blocked: number
  videos: number
  uptimeSeconds: number
  updatedAt: string
}

export interface StreamEvent {
  id: number
  kind: 'batch' | 'scheduler'
  videoId?: string
  at: string
  tasks?: TaskDelta[]
  video?: VideoDelta
  chapters?: ChapterDelta[]
  scheduler?: SchedulerDelta
}
