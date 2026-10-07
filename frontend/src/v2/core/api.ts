import type { QueryClient } from '@tanstack/react-query'

import * as ChannelService from '@bindings/services/channelservice'
import * as ChapterService from '@bindings/services/chapterservice'
import * as SettingsService from '@bindings/services/settingsservice'
import * as TaskService from '@bindings/services/taskservice'
import * as ThumbnailService from '@bindings/services/thumbnailservice'
import * as VideoService from '@bindings/services/videoservice'

import type { Channel, ChannelAuth, Chapter, Metadata, Setting, Task, Video } from './types'

// The Go services, called through Wails; every failure is an ApiError.

/** What kind of failure a call reported (see services/errors.go). */
export type ErrorKind =
  'cancelled' | 'not_found' | 'invalid' | 'conflict' | 'unavailable' | 'internal'

/** ApiError carries the backend's message through to the UI. */
export class ApiError extends Error {
  readonly kind: ErrorKind

  constructor(kind: ErrorKind, message: string) {
    super(message || 'Something went wrong')
    this.name = 'ApiError'
    this.kind = kind
  }
}

function toApiError(error: unknown): ApiError {
  if (error instanceof ApiError) return error
  if (error instanceof Error) {
    const cause = error.cause as { kind?: ErrorKind } | undefined
    return new ApiError(cause?.kind ?? 'internal', error.message)
  }
  return new ApiError('internal', String(error))
}

async function call<T>(promise: PromiseLike<unknown>): Promise<T> {
  try {
    return (await promise) as T
  } catch (error) {
    throw toApiError(error)
  }
}

/** A blob's bytes as base64, which is how a Go []byte crosses the bridge. */
async function base64(blob: Blob): Promise<string> {
  const bytes = new Uint8Array(await blob.arrayBuffer())
  let binary = ''
  for (let i = 0; i < bytes.length; i += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000))
  }
  return btoa(binary)
}

/** What the server needs to lay out a video's DAG. */
export interface NewVideo {
  channel: string
  title: string
  topic?: string
  chapterCount?: number
  targetDurationMinutes?: number
  slidesPerChapter?: number
  thumbnailCells?: number
  /** Enqueue the DAG straight away, rather than leaving it a draft. */
  start?: boolean
  /**
   * An outline written elsewhere, sent instead of leaving the model to write
   * one. Typed as unknown because this layer does not read it: it is the
   * operator's JSON, forwarded whole, and the server is what has an opinion
   * about its shape.
   */
  blueprint?: unknown
}

/**
 * A video's brief, whole.
 *
 * The channel is absent because it is not editable, and `start` because editing
 * is not a lifecycle verb. Everything else is required: the dialog holds all six
 * and sends all six, so there is nothing for the server to merge.
 */
export interface VideoBrief {
  title: string
  topic: string
  chapterCount: number
  slidesPerChapter: number
  thumbnailCells: number
  targetDurationMinutes: number
}

/**
 * What a re-run did: the tasks that ran again, and the ones it left flagged.
 *
 * Nothing renders this yet — the grid catches up over the event stream — but it
 * is what the endpoint answers with, and typing it as `void` would be a lie the
 * next caller has to discover.
 */
export interface RerunPlan {
  dryRun: boolean
  rerun: Task[]
  stale: Task[]
}

/** What a chapter of the blueprint plans; the whole plan, every time. */
export interface ChapterPlan {
  title: string
  summary: string
  /** The spoken-word budget. 0 is unset, not zero words. */
  estimatedWords: number
}

export const api = {
  listChannels: () => call<Channel[]>(ChannelService.List()),

  listVideos: () => call<Video[]>(VideoService.List()),
  createVideo: (body: NewVideo) => call<Video>(VideoService.Create(body)),
  getVideo: (ref: string) => call<Video>(VideoService.Get(ref)),
  /**
   * Corrects the brief a video was created from. Nothing re-runs: these fields
   * are read as each task runs, so an edit reaches whatever has not happened
   * yet. Answers with the whole video, so callers patch the cache from it.
   */
  updateVideo: (ref: string, brief: VideoBrief) => call<Video>(VideoService.Update(ref, brief)),
  /**
   * The one verb that gets a stopped video moving again: a draft's blueprint is
   * enqueued, or whatever an existing DAG stopped on is requeued — which is why
   * the button says either Start or Resume.
   */
  startVideo: (ref: string) => call<Video>(VideoService.Start(ref)),
  cancelVideo: (ref: string) => call<Video>(VideoService.Cancel(ref)),
  /**
   * Publishes an already-published video again, as a second YouTube video. The
   * one already up stays up, untracked; the answer carries its URL because
   * nothing here will know it afterwards.
   */
  republishVideo: (ref: string) =>
    call<{ previousUrl: string; previousVideoId: string }>(VideoService.Republish(ref)),
  /** Removes the video and the files only it uses. There is no undo. */
  deleteVideo: (ref: string) => call<void>(VideoService.Delete(ref)),

  listChapters: (ref: string) => call<Chapter[]>(ChapterService.List(ref)),
  listTasks: (ref: string) => call<Task[]>(TaskService.List(ref)),

  /** An edit to the blueprint's plan for one chapter. Nothing re-runs. */
  updateChapterPlan: (id: string, plan: ChapterPlan) =>
    call<Chapter>(ChapterService.UpdatePlan(id, plan)),
  /**
   * Replaces a chapter's narration. What the old text produced is flagged
   * stale rather than re-run, so the decision to redo it stays with the editor.
   */
  updateChapterScript: (id: string, script: string) =>
    call<Chapter>(ChapterService.UpdateScript(id, script)),
  /**
   * A new prompt for one slide, and the redraw it implies: there is no way to
   * save a prompt without generating from it, so the stored text and the
   * picture always describe each other.
   */
  regenerateSlide: (chapterId: string, index: number, prompt: string) =>
    call<Chapter>(ChapterService.RegenerateSlide(chapterId, index, prompt)),
  /** Redraw one thumbnail cell from an edited prompt. */
  regenerateThumbnailIcon: (ref: string, index: number, prompt: string) =>
    call<Video>(ThumbnailService.RegenerateIcon(ref, index, prompt)),

  /** An asset's bytes, as text (for the blueprint, the JSON the model wrote). */
  assetText: async (id: string) => {
    const response = await fetch(`/assets/${encodeURIComponent(id)}`)
    if (!response.ok) {
      throw new ApiError(response.status === 404 ? 'not_found' : 'internal', response.statusText)
    }
    return response.text()
  },

  listSettings: () => call<Setting[]>(SettingsService.List()),
  updateSetting: (name: string, value: string) =>
    call<Setting>(SettingsService.Update(name, value)),

  /** The thumbnail builder's working document, opaque to the backend. */
  saveThumbnailDesign: (ref: string, design: unknown) =>
    call<Video>(ThumbnailService.SaveDesign(ref, design)),
  /**
   * The image the builder drew, as the one that publishes. The rendered
   * thumbnail is kept, so reverting is always possible.
   */
  applyThumbnailOverride: async (ref: string, png: Blob) =>
    call<Video>(ThumbnailService.ApplyOverride(ref, await base64(png))),
  /** Back to the renderer's own image. The design document is kept. */
  clearThumbnailOverride: (ref: string) => call<Video>(ThumbnailService.ClearOverride(ref)),

  /**
   * What a channel can publish with. Cheap, and reconciling: calling it is what
   * makes the channel's row agree with a token that arrived while nothing was
   * looking.
   */
  channelAuth: (channel: string) => call<ChannelAuth>(ChannelService.Auth(channel)),
  /** Generated per call: it is only useful while somebody is looking at it. */
  channelAuthUrl: (channel: string) => call<string>(ChannelService.AuthURL(channel)),
  /** Takes the whole redirect URL as well as a bare code. */
  authorizeChannel: (channel: string, code: string) =>
    call<ChannelAuth>(ChannelService.Authorize(channel, code)),
  /** Drops the grant. The OAuth client stays, so re-authorizing needs no file. */
  forgetChannelAuth: (channel: string) => call<ChannelAuth>(ChannelService.ForgetAuth(channel)),

  /**
   * Runs tasks that already succeeded — and only those. Everything downstream
   * keeps its artifact and is flagged stale instead. Pipeline events bring the
   * grid up to date, so there is nothing to invalidate here.
   */
  rerunTasks: (ref: string, taskIds: string[]) => call<RerunPlan>(TaskService.Rerun(ref, taskIds)),
  /** A failed task and everything under it, which is blocked rather than done. */
  retryTask: (id: string) => call<Task>(TaskService.Retry(id)),
  /**
   * Clears the stale flag without running anything: "I looked, and it is still
   * fine". The only way to settle a flagged artifact that costs no generation.
   */
  acceptStale: (ref: string, taskIds: string[]) =>
    call<number>(TaskService.AcceptStale(ref, taskIds)).then((count) => ({ count })),

  /**
   * The YouTube listing, replaced whole. Nothing re-runs: the upload reads this
   * row when it runs, so a corrected title is simply what publishes.
   */
  saveMetadata: (ref: string, metadata: Metadata) =>
    call<Video>(VideoService.SaveMetadata(ref, metadata)),
  /**
   * Send a published video's listing to YouTube again. Separate from saving on
   * purpose: a local edit should never silently rewrite a live video.
   */
  pushMetadata: (ref: string) => call<Video>(VideoService.PushMetadata(ref)),
  /** Send a published video's thumbnail (whichever would publish) again. */
  pushThumbnail: (ref: string) => call<Video>(ThumbnailService.Push(ref)),

  approveGate: (ref: string, gate: string) => call<Task>(VideoService.Approve(ref, gate)),
  rejectGate: (ref: string, gate: string, reason: string) =>
    call<Task>(VideoService.Reject(ref, gate, reason)),
}

/**
 * Query keys.
 *
 * Two rules, and the event stream is why both exist.
 *
 * Everything lives under one `v2` root, so a resync can drop v2's whole cache
 * without touching anything else.
 *
 * And everything *inside* a video is keyed by the video's **id**, not by the
 * `ref` a document was opened with. A delta only ever carries the id, so keying
 * by ref would mean guessing at a key on every frame — the shape of a bug where
 * the table never moves while the sidebar does. The one entry that has to be
 * keyed by ref is the video itself, because that is the only key a restored tab
 * has; `events.ts` resolves that one through the list.
 */
export const qk = {
  channels: ['v2', 'channels'] as const,
  /**
   * Keyed by the channel key the caller had, not by id: the upload strip knows
   * a video's channel slug and nothing more, and resolving it first would put a
   * round trip in front of the question.
   */
  channelAuth: (channel: string) => ['v2', 'channel-auth', channel] as const,
  videos: ['v2', 'videos'] as const,
  video: (ref: string) => ['v2', 'video', ref] as const,
  chapters: (videoId: string) => ['v2', 'chapters', videoId] as const,
  tasks: (videoId: string) => ['v2', 'tasks', videoId] as const,
  settings: ['v2', 'settings'] as const,
  /** Content-addressed, so the key is the version and it never goes stale. */
  asset: (id: string) => ['v2', 'asset', id] as const,
}

/** The content-addressed URL of an asset; the hash is the cache key. */
export function assetUrl(id: string | undefined): string | undefined {
  return id ? `/assets/${id}` : undefined
}

export function prefetchVideo(client: QueryClient, video: Pick<Video, 'id' | 'ref'>): void {
  void client.prefetchQuery({
    queryKey: qk.video(video.ref),
    queryFn: () => api.getVideo(video.ref),
  })
  void client.prefetchQuery({
    queryKey: qk.chapters(video.id),
    queryFn: () => api.listChapters(video.ref),
  })
  void client.prefetchQuery({
    queryKey: qk.tasks(video.id),
    queryFn: () => api.listTasks(video.ref),
  })
}
