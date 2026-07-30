/**
 * The shapes the API returns.
 *
 * These mirror the Go DTOs one-for-one. `npm run gen:api` regenerates
 * `schema.d.ts` from the daemon's OpenAPI document, and `npm run typecheck`
 * asserts these stay assignable to it — a drifted client type is a build
 * failure rather than a runtime surprise.
 */

export type VideoState =
  'draft' | 'running' | 'awaiting_approval' | 'blocked' | 'completed' | 'failed' | 'cancelled'

export type TaskState =
  'blocked' | 'ready' | 'running' | 'awaiting_approval' | 'succeeded' | 'failed' | 'cancelled'

export type TaskKind =
  | 'blueprint'
  | 'prime_image_prompts'
  | 'image_prompts'
  | 'script'
  | 'tts'
  | 'image'
  | 'clip'
  | 'concat'
  | 'metadata'
  | 'upload'

export type PoolName = 'llm' | 'tts' | 'image' | 'compose' | 'cache' | 'upload'

export type GateKind = 'blueprint' | 'upload'

export interface Style {
  tone: string
  voice: string
  imageStyle: string
  language: string
  wordsPerChapter: number
}

export interface Channel {
  id: string
  slug: string
  name: string
  description: string
  style: Style
  credentials: 'missing' | 'valid' | 'expired'
  videoSeq: number
  createdAt: string
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
  /**
   * Cuts across the counts above rather than partitioning with them: a stale
   * task is usually also a succeeded one.
   */
  stale: number
  cancelled: number
}

export interface Metadata {
  title: string
  description: string
  tags: string[]
  categoryId: string
  privacy: string
}

export interface UploadRecord {
  remoteVideoId: string
  url: string
  dryRun: boolean
  uploadedAt: string
}

export interface Video {
  id: string
  channelId: string
  ref: string
  title: string
  topic: string
  state: VideoState
  chapterCount: number
  imagesPerChapter: number
  blueprintAssetId?: string
  finalAssetId?: string
  metadata?: Metadata
  upload?: UploadRecord
  error?: string
  counts: TaskCounts
  createdAt: string
  updatedAt: string
  startedAt?: string
  completedAt?: string
}

export interface Chapter {
  id: string
  videoId: string
  ordinal: number
  title: string
  summary: string
  script: string
  imagePrompts: string[]
  audioAssetId?: string
  imageAssetIds: string[]
  clipAssetId?: string
  durationSeconds: number
  updatedAt: string
}

export interface Task {
  id: string
  videoId: string
  chapterId?: string
  kind: TaskKind
  ordinal: number
  index: number
  state: TaskState
  pool: PoolName
  gate?: GateKind | ''
  attempt: number
  maxAttempts: number
  depsRemaining: number
  /** An input changed after this ran; the artifact is intact but unverified. */
  stale: boolean
  error?: string
  updatedAt: string
  startedAt?: string
  finishedAt?: string
  notBefore?: string
}

export interface PoolStat {
  pool: PoolName
  limit: number
  inFlight: number
  queued: number
}

export interface SchedulerStatus {
  pools: PoolStat[]
  ready: number
  running: number
  blocked: number
  awaitingApproval: number
  succeeded: number
  failed: number
  retryPending: number
  videos: number
  uptimeSeconds: number
  startedAt: string
}

export interface Setting {
  key: string
  value: string
  type: 'int' | 'bool' | 'string'
  group: string
  description: string
  min: number
  max: number
  /** The only accepted values; empty when the setting is free-form. */
  options: string[]
  updatedAt: string
}

export interface Asset {
  id: string
  videoId: string
  chapterId?: string
  kind: string
  size: number
  mime: string
  url: string
  createdAt: string
}

/* ------------------------------------------------------------------ events */

export interface TaskDelta {
  id: string
  videoId: string
  chapterId?: string
  kind: TaskKind
  ordinal: number
  index: number
  state: TaskState
  pool: PoolName
  gate?: GateKind | ''
  attempt: number
  stale: boolean
  error?: string
  updatedAt: string
}

export interface VideoDelta {
  id: string
  ref: string
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
  hasScript: boolean
  audioAssetId?: string
  imageAssetIds?: string[]
  clipAssetId?: string
  updatedAt: string
}

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

/**
 * What a re-run is about to do. Returned by a dry run so the operator can see
 * the blast radius before committing, and by the real thing so the UI can say
 * what just happened.
 */
export interface RerunPlan {
  dryRun: boolean
  rerun: Task[]
  stale: Task[]
}
