import type {
  Asset,
  Channel,
  Chapter,
  Preset,
  RerunPlan,
  SchedulerStatus,
  Setting,
  Task,
  Video,
} from './types'

/** ApiError carries the server's RFC 7807 problem detail through to the UI. */
export class ApiError extends Error {
  readonly status: number
  readonly detail: string

  constructor(status: number, detail: string) {
    super(detail || `request failed with status ${status}`)
    this.name = 'ApiError'
    this.status = status
    this.detail = detail
  }
}

interface Problem {
  title?: string
  detail?: string
  errors?: { message?: string; location?: string }[]
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
      if (problem.errors?.length) {
        const first = problem.errors[0]
        if (first?.message) detail = `${detail}: ${first.message}`
      }
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

/**
 * Every action the UI can take is available here and, by construction, on the
 * same API the CLI would use. The UI has no privileged access.
 */
export const api = {
  health: () => request<{ status: string; version: string; sseClients: number }>('/api/health'),

  listChannels: () => request<{ channels: Channel[] }>('/api/channels').then((r) => r.channels),
  getChannel: (key: string) => request<Channel>(`/api/channels/${encodeURIComponent(key)}`),
  createChannel: (body: {
    slug?: string
    name: string
    description?: string
    style: Partial<Channel['style']>
  }) => post<Channel>('/api/channels', body),
  updateChannel: (
    key: string,
    body: {
      name?: string
      description?: string
      style: Partial<Channel['style']>
      credentials?: string
    },
  ) =>
    request<Channel>(`/api/channels/${encodeURIComponent(key)}`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),
  deleteChannel: (key: string) =>
    request<void>(`/api/channels/${encodeURIComponent(key)}`, { method: 'DELETE' }),

  listVideos: (params: { channelId?: string; state?: string; limit?: number } = {}) => {
    const search = new URLSearchParams()
    if (params.channelId) search.set('channelId', params.channelId)
    if (params.state) search.set('state', params.state)
    search.set('limit', String(params.limit ?? 200))
    return request<{ videos: Video[]; total: number }>(`/api/videos?${search.toString()}`)
  },
  getVideo: (key: string) => request<Video>(`/api/videos/${encodeURIComponent(key)}`),
  createVideo: (
    body: {
      channel: string
      title: string
      topic?: string
      chapterCount?: number
      targetDurationMinutes?: number
      slidesPerChapter?: number
      thumbnailCells?: number
      start?: boolean
    },
    idempotencyKey: string,
  ) => post<Video>('/api/videos', body, idempotencyKey),
  startVideo: (key: string) => post<Video>(`/api/videos/${encodeURIComponent(key)}/start`),
  cancelVideo: (key: string) => post<Video>(`/api/videos/${encodeURIComponent(key)}/cancel`),
  deleteVideo: (key: string) =>
    request<void>(`/api/videos/${encodeURIComponent(key)}`, { method: 'DELETE' }),
  approveGate: (key: string, gate: string) =>
    post<Task>(`/api/videos/${encodeURIComponent(key)}/approve`, { gate }),
  rejectGate: (key: string, gate: string, reason: string) =>
    post<Task>(`/api/videos/${encodeURIComponent(key)}/reject`, { gate, reason }),

  listChapters: (key: string) =>
    request<{ chapters: Chapter[] }>(`/api/videos/${encodeURIComponent(key)}/chapters`).then(
      (r) => r.chapters,
    ),
  updateChapterScript: (chapterId: string, script: string) =>
    request<Chapter>(`/api/chapters/${encodeURIComponent(chapterId)}/script`, {
      method: 'PUT',
      body: JSON.stringify({ script }),
    }),
  /**
   * Saves the prompt at this slot and redraws that one slide from it. Saving
   * and generating are one call because they are one decision: there is no way
   * to store a prompt the slide on screen was not drawn from.
   */
  regenerateSlide: (chapterId: string, index: number, prompt: string) =>
    post<Chapter>(`/api/chapters/${encodeURIComponent(chapterId)}/slides/${index}/generate`, {
      prompt,
    }),
  /**
   * The same one-step loop on the thumbnail grid: saves the cell's prompt and
   * redraws that icon. The caption and the shared style clause are untouched.
   */
  regenerateThumbnailIcon: (key: string, index: number, prompt: string) =>
    post<Video>(`/api/videos/${encodeURIComponent(key)}/thumbnail/cells/${index}/generate`, {
      prompt,
    }),
  retryChapter: (key: string, ordinal: number) =>
    post<void>(`/api/videos/${encodeURIComponent(key)}/chapters/${ordinal}/retry`),

  /**
   * Re-runs tasks that already succeeded. Everything downstream is flagged
   * stale rather than re-run, so artifacts the operator may have reviewed are
   * not discarded without a decision. `dryRun` reports the blast radius and
   * changes nothing.
   */
  rerunTasks: (key: string, taskIds: string[], dryRun = false) =>
    post<RerunPlan>(`/api/videos/${encodeURIComponent(key)}/rerun`, { taskIds, dryRun }),
  /** Re-runs stale tasks. An empty list means all of them. */
  runStale: (key: string, taskIds: string[] = []) =>
    post<{ count: number }>(`/api/videos/${encodeURIComponent(key)}/stale/run`, { taskIds }),
  /** Keeps stale artifacts as they are, clearing the flag without re-running. */
  acceptStale: (key: string, taskIds: string[] = []) =>
    post<{ count: number }>(`/api/videos/${encodeURIComponent(key)}/stale/accept`, { taskIds }),

  listVideoTasks: (key: string) =>
    request<{ tasks: Task[] }>(`/api/videos/${encodeURIComponent(key)}/tasks`).then((r) => r.tasks),
  listRecentTasks: (limit = 300) =>
    request<{ tasks: Task[] }>(`/api/tasks?limit=${limit}`).then((r) => r.tasks),
  retryTask: (id: string) => post<Task>(`/api/tasks/${encodeURIComponent(id)}/retry`),

  listAssets: (key: string) =>
    request<{ assets: Asset[] }>(`/api/videos/${encodeURIComponent(key)}/assets`).then(
      (r) => r.assets,
    ),

  schedulerStatus: () => request<SchedulerStatus>('/api/scheduler'),

  listSettings: () => request<{ settings: Setting[] }>('/api/settings').then((r) => r.settings),
  updateSetting: (key: string, value: string) =>
    request<Setting>(`/api/settings/${encodeURIComponent(key)}`, {
      method: 'PUT',
      body: JSON.stringify({ value }),
    }),

  listPresets: () => request<{ presets: Preset[] }>('/api/settings/presets').then((r) => r.presets),
  /** Resolves to the rows that actually moved; an empty list means it was already in force. */
  applyPreset: (name: string) =>
    post<{ settings: Setting[] }>(`/api/settings/presets/${encodeURIComponent(name)}/apply`).then(
      (r) => r.settings,
    ),
}

/** Query keys, centralised so an event delta can invalidate precisely. */
export const qk = {
  channels: ['channels'] as const,
  channel: (key: string) => ['channels', key] as const,
  videos: (params?: { channelId?: string; state?: string }) => ['videos', params ?? {}] as const,
  video: (key: string) => ['video', key] as const,
  chapters: (key: string) => ['video', key, 'chapters'] as const,
  videoTasks: (key: string) => ['video', key, 'tasks'] as const,
  assets: (key: string) => ['video', key, 'assets'] as const,
  recentTasks: ['tasks', 'recent'] as const,
  scheduler: ['scheduler'] as const,
  settings: ['settings'] as const,
  presets: ['settings', 'presets'] as const,
}

/** The content-addressed URL of an asset; the hash is the cache key. */
export function assetUrl(id: string | undefined): string | undefined {
  return id ? `/assets/${id}` : undefined
}
