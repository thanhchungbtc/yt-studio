/**
 * The slice of the API this UI actually reads.
 *
 * It is a narrowing, not a fork: every field here is a field the server already
 * sends, and anything no screen renders is left out until one needs it.
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
  /**
   * How far a long task has got, 0-100.
   *
   * Only meaningful while `state` is `running`, and only present at all for the
   * tasks whose backend can measure themselves — today the ffmpeg concat and
   * the upload, which are the two long enough that "running" is not an answer.
   * It is never persisted, so it arrives by delta and never from the task list;
   * and because a delta that carries no percent does not clear the last one,
   * the state is what says whether to read it.
   */
  percent?: number
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
  /** The plan the pipeline was built from, as the LLM returned it. */
  blueprintAssetId?: string
  finalAssetId?: string
  /** The thumbnail that will actually publish: the override if there is one. */
  effectiveThumbnailAssetId?: string
  metadata?: Metadata
  upload?: UploadRecord
  error?: string
  counts: TaskCounts
  /**
   * When it was made, and the only timestamp on a video that never moves.
   *
   * `updatedAt` advances on every task delta, so a list ordered by it reorders
   * itself under the pointer while anything is running. This is what the library
   * sorts by instead.
   */
  createdAt: string
  updatedAt: string
}

/** One value the operator can change, and everything needed to draw it. */
export interface Setting {
  key: string
  value: string
  type: 'int' | 'bool' | 'string' | 'float'
  group: string
  description: string
  min: number
  max: number
  /** A closed set: the value must be one of these. */
  options: string[]
  /**
   * Known-good values that are not the only ones allowed. A model name is the
   * case: the list is helpful and out of date the week it ships.
   */
  suggestions: { value: string; label: string }[]
  /**
   * The backend this belongs to. Non-empty means the setting only matters while
   * that backend is the one selected — thirteen narration settings for a server
   * you are not using is noise, not configurability.
   */
  backend: string
  /** Never echoed back by the server; `configured` is how you know it is set. */
  secret: boolean
  configured: boolean
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
  /** Progress, when the task is one that can measure itself. See `Task`. */
  percent?: number
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
