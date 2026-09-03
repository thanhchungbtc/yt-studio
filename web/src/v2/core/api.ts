import type { Channel, ChannelAuth, Chapter, Setting, Task, Video } from './types'

/**
 * V2's client.
 *
 * Deliberately its own, and deliberately small: it grows a method when a screen
 * needs one and not before. It talks to the same HTTP the CLI does — the UI has
 * no privileged access.
 */

/** ApiError carries the server's RFC 7807 problem detail through to the UI. */
export class ApiError extends Error {
  readonly status: number

  constructor(status: number, detail: string) {
    super(detail || `request failed with status ${status}`)
    this.name = 'ApiError'
    this.status = status
  }
}

interface Problem {
  title?: string
  detail?: string
  errors?: ProblemError[]
}

/** One field's rejection, as huma reports it: `location` is a JSON path. */
interface ProblemError {
  message?: string
  location?: string
}

/**
 * The message to show for a failed response.
 *
 * `detail` alone is not enough. When huma rejects a request against the schema
 * it never reaches the handler, so the detail is the bare string "validation
 * failed" for every cause there is — a title too long and a topic too long read
 * identically, and neither says which field. What field it was lives in
 * `errors`, so that is folded in here rather than dropped: "validation failed:
 * topic expected length <= 5000" is a message an operator can act on alone.
 */
async function problemDetail(response: Response): Promise<string> {
  try {
    const problem = (await response.json()) as Problem
    const detail = problem.detail ?? problem.title ?? response.statusText
    const fields = (problem.errors ?? [])
      .map((e) => {
        // Locations arrive prefixed with the part of the request they were found
        // in. The operator is looking at a form, not at an HTTP message — and a
        // whole-body complaint ("channel is required") names its own field in
        // the message, so there is nothing left to prefix it with.
        const where = e.location?.replace(/^body\.?/, '')
        return [where, e.message].filter(Boolean).join(' ')
      })
      .filter(Boolean)
    return fields.length > 0 ? `${detail}: ${fields.join('; ')}` : detail
  } catch {
    // A non-JSON error body is not worth failing twice over.
    return response.statusText
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      Accept: 'application/json',
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
      ...init?.headers,
    },
  })
  if (!response.ok) {
    throw new ApiError(response.status, await problemDetail(response))
  }
  if (response.status === 204) return undefined as T
  return (await response.json()) as T
}

function post<T>(path: string, body?: unknown, idempotencyKey?: string): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    ...(body !== undefined ? { body: JSON.stringify(body) } : {}),
    ...(idempotencyKey ? { headers: { 'Idempotency-Key': idempotencyKey } } : {}),
  })
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
}

/** What a chapter of the blueprint plans; the whole plan, every time. */
export interface ChapterPlan {
  title: string
  summary: string
  /** The spoken-word budget. 0 is unset, not zero words. */
  estimatedWords: number
}

const key = encodeURIComponent

export const api = {
  listChannels: () => request<{ channels: Channel[] }>('/api/channels').then((r) => r.channels),

  listVideos: () => request<{ videos: Video[] }>('/api/videos?limit=200').then((r) => r.videos),
  /**
   * The key makes a double submit a no-op rather than a second video. It is
   * minted once per dialog session, so the retry of a request that timed out
   * on the way back still resolves to the video it already created.
   */
  createVideo: (body: NewVideo, idempotencyKey: string) =>
    post<Video>('/api/videos', body, idempotencyKey),
  getVideo: (ref: string) => request<Video>(`/api/videos/${key(ref)}`),
  /**
   * The one verb that gets a stopped video moving again.
   *
   * The server does two different things behind it — enqueue a fresh blueprint
   * for a video with no tasks, or requeue whatever an existing DAG stopped on —
   * which is why the button that calls it says either Start or Resume.
   */
  startVideo: (ref: string) => post<Video>(`/api/videos/${key(ref)}/start`),
  cancelVideo: (ref: string) => post<Video>(`/api/videos/${key(ref)}/cancel`),
  /**
   * Removes the video, its chapters, its task graph and the files only it was
   * using. There is no undo and no trash: the server unlinks what nothing else
   * references, so this is the one call in here that destroys work.
   */
  deleteVideo: (ref: string) => request<void>(`/api/videos/${key(ref)}`, { method: 'DELETE' }),

  listChapters: (ref: string) =>
    request<{ chapters: Chapter[] }>(`/api/videos/${key(ref)}/chapters`).then((r) => r.chapters),
  listTasks: (ref: string) =>
    request<{ tasks: Task[] }>(`/api/videos/${key(ref)}/tasks`).then((r) => r.tasks),

  /**
   * An edit to the blueprint, and only that.
   *
   * Keyed by chapter id, not by video ref, because a chapter is what is being
   * changed — and the id is derived (`<video>:ch:<ordinal>`), so the row already
   * knows it.
   *
   * The server re-runs nothing. A chapter whose script has not been written yet
   * will be written from this; one that already has a script keeps it until the
   * operator decides otherwise. The returned chapter is the whole row, so the
   * cache is patched from the response rather than refetched.
   */
  updateChapterPlan: (id: string, plan: ChapterPlan) =>
    request<Chapter>(`/api/chapters/${key(id)}/plan`, {
      method: 'PUT',
      body: JSON.stringify(plan),
    }),

  /**
   * An asset's bytes, as text.
   *
   * Not `request`: an asset is not an API resource with a problem-detail error
   * shape, it is a file at a content address. What comes back is whatever was
   * stored, which for the blueprint is the JSON the model returned.
   */
  assetText: async (id: string) => {
    const response = await fetch(`/assets/${key(id)}`)
    if (!response.ok) throw new ApiError(response.status, response.statusText)
    return response.text()
  },

  listSettings: () => request<{ settings: Setting[] }>('/api/settings').then((r) => r.settings),
  updateSetting: (name: string, value: string) =>
    request<Setting>(`/api/settings/${key(name)}`, {
      method: 'PUT',
      body: JSON.stringify({ value }),
    }),

  /**
   * The thumbnail builder's working document.
   *
   * Opaque to the server, which is the point: the builder owns the shape, so
   * this is the only definition of it. Saved alongside the override rather than
   * as you type, because a half-finished draft is not what should reopen.
   */
  saveThumbnailDesign: (ref: string, design: unknown) =>
    request<Video>(`/api/videos/${key(ref)}/thumbnail/design`, {
      method: 'PUT',
      body: JSON.stringify({ design }),
    }),

  /**
   * The image the builder drew, as the one that publishes.
   *
   * Not `request`: the body is a PNG, so there is no JSON envelope to send and
   * the content type has to say what it actually is. The rendered thumbnail is
   * left where it is on its own field, so re-running the thumbnail task cannot
   * discard this and reverting is always possible.
   */
  applyThumbnailOverride: async (ref: string, png: Blob) => {
    const response = await fetch(`/api/videos/${key(ref)}/thumbnail/override`, {
      // POST, matching the operation in `delivery/http/thumbnails.go`. PUT is
      // what the sibling design route uses and what this looked like it should
      // be; the server answers a PUT here with a 500, not a 405, so the mistake
      // arrives looking like a bug in the image rather than in the verb.
      method: 'POST',
      headers: { 'Content-Type': 'image/png', Accept: 'application/json' },
      body: png,
    })
    if (!response.ok) {
      throw new ApiError(response.status, await problemDetail(response))
    }
    return (await response.json()) as Video
  },

  /** Back to the renderer's own image. The design document is kept. */
  clearThumbnailOverride: (ref: string) =>
    request<Video>(`/api/videos/${key(ref)}/thumbnail/override`, { method: 'DELETE' }),

  /**
   * What a channel can publish with. Cheap — the server reads two files — and
   * reconciling: calling it is what makes the channel's row agree with a token
   * that arrived while nothing was looking.
   */
  channelAuth: (channel: string) => request<ChannelAuth>(`/api/channels/${key(channel)}/youtube`),
  /** Generated per call: it is only useful while somebody is looking at it. */
  channelAuthUrl: (channel: string) =>
    request<{ url: string }>(`/api/channels/${key(channel)}/youtube/auth-url`).then((r) => r.url),
  /** Takes the whole redirect URL as well as a bare code. */
  authorizeChannel: (channel: string, code: string) =>
    post<ChannelAuth>(`/api/channels/${key(channel)}/youtube/authorize`, { code }),
  /** Drops the grant. The OAuth client stays, so re-authorizing needs no file. */
  forgetChannelAuth: (channel: string) =>
    request<ChannelAuth>(`/api/channels/${key(channel)}/youtube`, { method: 'DELETE' }),

  approveGate: (ref: string, gate: string) =>
    post<Task>(`/api/videos/${key(ref)}/approve`, { gate }),
  rejectGate: (ref: string, gate: string, reason: string) =>
    post<Task>(`/api/videos/${key(ref)}/reject`, { gate, reason }),
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
