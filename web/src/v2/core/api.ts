import type { Channel, Chapter, Setting, Task, Video } from './types'

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
    let detail = response.statusText
    try {
      const problem = (await response.json()) as Problem
      detail = problem.detail ?? problem.title ?? detail
    } catch {
      // A non-JSON error body is not worth failing twice over.
    }
    throw new ApiError(response.status, detail)
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
  cancelVideo: (ref: string) => post<Video>(`/api/videos/${key(ref)}/cancel`),

  listChapters: (ref: string) =>
    request<{ chapters: Chapter[] }>(`/api/videos/${key(ref)}/chapters`).then((r) => r.chapters),
  listTasks: (ref: string) =>
    request<{ tasks: Task[] }>(`/api/videos/${key(ref)}/tasks`).then((r) => r.tasks),

  listSettings: () => request<{ settings: Setting[] }>('/api/settings').then((r) => r.settings),
  updateSetting: (name: string, value: string) =>
    request<Setting>(`/api/settings/${key(name)}`, {
      method: 'PUT',
      body: JSON.stringify({ value }),
    }),

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
  videos: ['v2', 'videos'] as const,
  video: (ref: string) => ['v2', 'video', ref] as const,
  chapters: (videoId: string) => ['v2', 'chapters', videoId] as const,
  tasks: (videoId: string) => ['v2', 'tasks', videoId] as const,
  settings: ['v2', 'settings'] as const,
}

/** The content-addressed URL of an asset; the hash is the cache key. */
export function assetUrl(id: string | undefined): string | undefined {
  return id ? `/assets/${id}` : undefined
}
