import { useEffect, useRef, useState } from 'react'
import { useQueryClient, type QueryClient } from '@tanstack/react-query'
import { create } from 'zustand'

import { qk } from './api'
import type {
  Chapter,
  ChapterDelta,
  SchedulerDelta,
  StreamEvent,
  Task,
  TaskDelta,
  Video,
  VideoDelta,
} from './types'

/**
 * The stream that keeps the window true.
 *
 * One `EventSource` on `/events` for the whole application — the server calls
 * it "the single multiplexed SSE stream" and means it. Every frame is a batch
 * of deltas for whichever video moved, and they are applied to the query cache
 * **in place**. Nothing on screen polls, and nothing refetches merely to stay
 * current.
 *
 * Five things make that work, and they are the reason this file is worth
 * reading before adding a screen to v2:
 *
 *  1. `EventSource` owns reconnection. It retries on its own and resumes from
 *     `Last-Event-ID`, so the server replays what was missed. There is no
 *     backoff to write and no "am I connected" state to thread anywhere.
 *
 *  2. When the client was away longer than the server's replay buffer, the
 *     server says `resync` rather than lying. That is the *only* moment the
 *     whole cache is dropped.
 *
 *  3. A delta is a patch, not a record. Merging is per-element and returns the
 *     previous array unchanged when nothing moved, so an unchanged row keeps
 *     its object identity and a memoised component does not re-render. A
 *     fifty-chapter table under a running pipeline is the case that matters.
 *
 *  4. What a delta does not carry, it says has arrived. A script body is not on
 *     the wire; `hasScript` is. So a few frames end in a narrow `invalidate`
 *     rather than a patch — and those refetches are scoped to `active` queries,
 *     so a video nobody is looking at costs nothing.
 *
 *  5. A malformed frame is dropped, not thrown. The stream outlives any one
 *     bad message.
 */

export type ConnectionState = 'connecting' | 'live' | 'offline'

/**
 * A read-only tap, for views that want to *watch* the stream rather than be
 * kept current by it — a log, an activity view.
 *
 * Listeners run before the cache is patched and cannot alter the frame: the
 * stream's job is to keep the cache true, and an observer that could interfere
 * with that would make the cache a function of who happened to be watching.
 */
type StreamListener = (event: StreamEvent) => void

const listeners = new Set<StreamListener>()

export function subscribeStream(listener: StreamListener): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

/**
 * What the machine is doing, right now.
 *
 * Deliberately not in the query cache. There is no endpoint behind it that
 * anything fetches — the server sends a scheduler frame the moment the stream
 * opens, and another whenever the pools move — so a cache entry would be a
 * fetch that never happens wrapped around a push that always does.
 */
const useSchedulerStore = create<{ snapshot: SchedulerDelta | null }>(() => ({ snapshot: null }))

export function useScheduler(): SchedulerDelta | null {
  return useSchedulerStore((s) => s.snapshot)
}

/**
 * True when nothing a meter draws has moved.
 *
 * The scheduler recomputes on every task transition, which under load is many
 * times a second. Without this the status bar would re-render on frames that
 * would draw it identically.
 */
function sameSnapshot(a: SchedulerDelta | null, b: SchedulerDelta): boolean {
  if (!a || a.pools.length !== b.pools.length) return false
  return a.pools.every((pool, i) => {
    const next = b.pools[i]
    return (
      next !== undefined &&
      pool.pool === next.pool &&
      pool.limit === next.limit &&
      pool.inFlight === next.inFlight &&
      pool.queued === next.queued
    )
  })
}

/** Mounted once, at the root of the workbench. Never twice. */
export function useEventStream(): ConnectionState {
  const queryClient = useQueryClient()
  const [state, setState] = useState<ConnectionState>('connecting')

  // The effect runs once; only the client's identity could ever invalidate it,
  // and it does not change. A ref keeps the handler stable without saying so.
  const client = useRef(queryClient)
  client.current = queryClient

  useEffect(() => {
    const source = new EventSource('/events')

    const onFrame = (event: MessageEvent<string>) => {
      let frame: StreamEvent
      try {
        frame = JSON.parse(event.data) as StreamEvent
      } catch {
        return
      }
      for (const listener of listeners) {
        // Guarded one at a time: a spectator that throws must not stop the
        // cache being patched.
        try {
          listener(frame)
        } catch {
          /* ignored */
        }
      }
      apply(client.current, frame)
    }

    const onResync = () => {
      void client.current.invalidateQueries({ queryKey: ['v2'] })
    }

    source.onopen = () => setState('live')
    // A first failure is a reconnect in progress; a failure while already
    // reconnecting is worth calling offline.
    source.onerror = () => setState((prev) => (prev === 'live' ? 'connecting' : 'offline'))
    source.addEventListener('batch', onFrame as EventListener)
    source.addEventListener('scheduler', onFrame as EventListener)
    source.addEventListener('resync', onResync)

    return () => {
      source.removeEventListener('batch', onFrame as EventListener)
      source.removeEventListener('scheduler', onFrame as EventListener)
      source.removeEventListener('resync', onResync)
      source.close()
    }
  }, [])

  return state
}

function apply(client: QueryClient, frame: StreamEvent): void {
  if (frame.scheduler && !sameSnapshot(useSchedulerStore.getState().snapshot, frame.scheduler)) {
    useSchedulerStore.setState({ snapshot: frame.scheduler })
  }
  if (frame.tasks?.length) applyTasks(client, frame.tasks)
  if (frame.chapters?.length) applyChapters(client, frame.chapters)
  if (frame.video) applyVideo(client, frame.video)
}

/** Groups deltas by the video they belong to, so each cache entry is written once. */
function byVideo<T extends { videoId: string }>(deltas: T[]): Map<string, T[]> {
  const grouped = new Map<string, T[]>()
  for (const delta of deltas) {
    const list = grouped.get(delta.videoId)
    if (list) list.push(delta)
    else grouped.set(delta.videoId, [delta])
  }
  return grouped
}

function applyTasks(client: QueryClient, deltas: TaskDelta[]): void {
  for (const [videoId, group] of byVideo(deltas)) {
    client.setQueryData<Task[]>(qk.tasks(videoId), (prev) =>
      prev ? mergeTasks(prev, group) : prev,
    )
  }
}

function mergeTasks(previous: Task[], deltas: TaskDelta[]): Task[] {
  const pending = new Map(deltas.map((delta) => [delta.id, delta]))

  let changed = false
  const next = previous.map((task) => {
    const delta = pending.get(task.id)
    if (!delta) return task
    pending.delete(task.id)
    // Nothing changed that anything renders, so the identity is kept and the
    // array is not rebuilt. `percent` has to be in this list: a progress frame
    // is a delta whose state, attempt, staleness and error are all identical to
    // what is already cached, and without it every one of them would be
    // discarded here as a no-op.
    if (
      task.state === delta.state &&
      task.attempt === delta.attempt &&
      task.stale === delta.stale &&
      (task.percent ?? 0) === (delta.percent ?? 0) &&
      (task.error ?? '') === (delta.error ?? '')
    ) {
      return task
    }
    changed = true
    return { ...task, ...delta }
  })

  // Tasks the client has not seen — a freshly enqueued DAG — are appended
  // rather than refetched: a delta carries every field a Task has.
  if (pending.size > 0) {
    changed = true
    next.push(...pending.values())
  }
  return changed ? next : previous
}

function applyChapters(client: QueryClient, deltas: ChapterDelta[]): void {
  for (const [videoId, group] of byVideo(deltas)) {
    const cached = client.getQueryData<Chapter[]>(qk.chapters(videoId))
    if (!cached) continue

    // A chapter the cache has never seen is one the blueprint has just written.
    // Its delta carries none of what a row shows — no summary, no script, no
    // prompts — so appending a husk would be worse than refetching: an outline
    // rendered as blanks.
    const known = new Set(cached.map((chapter) => chapter.id))
    const unseen = group.some((delta) => !known.has(delta.id))

    client.setQueryData<Chapter[]>(qk.chapters(videoId), (prev) => {
      if (!prev) return prev
      const index = new Map(group.map((delta) => [delta.id, delta]))
      let changed = false
      const next = prev.map((chapter) => {
        const delta = index.get(chapter.id)
        if (!delta) return chapter
        const merged: Chapter = {
          ...chapter,
          title: delta.title || chapter.title,
          audioAssetId: delta.audioAssetId ?? chapter.audioAssetId,
          slideAssetIds: delta.slideAssetIds ?? chapter.slideAssetIds,
          clipAssetId: delta.clipAssetId ?? chapter.clipAssetId,
          updatedAt: delta.updatedAt,
        }
        if (
          merged.title === chapter.title &&
          merged.audioAssetId === chapter.audioAssetId &&
          merged.clipAssetId === chapter.clipAssetId &&
          sameIds(merged.slideAssetIds, chapter.slideAssetIds)
        ) {
          return chapter
        }
        changed = true
        return merged
      })
      return changed ? next : prev
    })

    // The script body is deliberately not on the wire. `hasScript` says one
    // landed; this is what goes and gets it.
    if (unseen || group.some((delta) => delta.hasScript)) {
      void client.invalidateQueries({ queryKey: qk.chapters(videoId), refetchType: 'active' })
    }
  }
}

function sameIds(a: string[] | undefined, b: string[] | undefined): boolean {
  if (a === b) return true
  if (!a || !b || a.length !== b.length) return false
  return a.every((value, i) => value === b[i])
}

function applyVideo(client: QueryClient, delta: VideoDelta): void {
  const patch = (video: Video | undefined): Video | undefined => {
    if (!video) return video
    if (
      video.state === delta.state &&
      video.counts.succeeded === delta.done &&
      video.counts.failed === delta.failed &&
      video.counts.running === delta.running
    ) {
      return video
    }
    return {
      ...video,
      state: delta.state,
      error: delta.error ?? video.error,
      counts: {
        ...video.counts,
        total: delta.total || video.counts.total,
        succeeded: delta.done,
        failed: delta.failed,
        running: delta.running,
      },
      updatedAt: delta.updatedAt,
    }
  }

  // The list is keyed by nothing in particular and holds every video, so it is
  // both the thing to patch and the index from id to ref — which is how the
  // open document, keyed by the ref its tab was restored with, gets found.
  let ref: string | undefined
  client.setQueryData<Video[]>(qk.videos, (prev) => {
    if (!prev) return prev
    let changed = false
    const next = prev.map((video) => {
      if (video.id !== delta.id) return video
      ref = video.ref
      const patched = patch(video)
      if (patched && patched !== video) changed = true
      return patched ?? video
    })
    return changed ? next : prev
  })

  if (ref) client.setQueryData<Video>(qk.video(ref), patch)

  // Reaching a gate, or finishing, changes what the document can offer: the
  // final cut, the upload receipt, the counts behind the gate strip. None of
  // that rides on the delta, so this one is a refetch.
  if (ref && (delta.state === 'awaiting_approval' || delta.state === 'completed')) {
    void client.invalidateQueries({ queryKey: qk.video(ref), refetchType: 'active' })
  }
}
