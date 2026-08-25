import type { Channel, Video } from './types'

/**
 * V2's client.
 *
 * Deliberately small and deliberately its own: the workbench reads two
 * collections, so the client is two calls plus the error type they can fail
 * with. It talks to the same HTTP the CLI does — the UI has no privileged
 * access — and grows a method only when a screen needs one.
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

async function request<T>(path: string): Promise<T> {
  const response = await fetch(path, { headers: { Accept: 'application/json' } })
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
  return (await response.json()) as T
}

export const api = {
  listChannels: () => request<{ channels: Channel[] }>('/api/channels').then((r) => r.channels),
  listVideos: () => request<{ videos: Video[] }>('/api/videos?limit=200').then((r) => r.videos),
}

/** Query keys, in one place so a mutation can invalidate what a view reads. */
export const qk = {
  channels: ['v2', 'channels'] as const,
  videos: ['v2', 'videos'] as const,
}
