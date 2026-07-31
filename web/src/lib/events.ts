import { useEffect, useRef, useState } from 'react'
import { useQueryClient, type QueryClient } from '@tanstack/react-query'

import { qk } from './api'
import type {
  Chapter,
  ChapterDelta,
  SchedulerStatus,
  StreamEvent,
  Task,
  TaskDelta,
  Video,
  VideoDelta,
} from './types'

export type ConnectionState = 'connecting' | 'live' | 'offline'

/**
 * The single multiplexed SSE stream.
 *
 * Events carry deltas, applied to the query cache in place; the UI never
 * refetches to stay current. `EventSource` handles reconnection and replays from
 * `Last-Event-ID` on its own.
 */
export function useEventStream(): ConnectionState {
  const queryClient = useQueryClient()
  const [state, setState] = useState<ConnectionState>('connecting')
  // The handler is stable; only the client identity would ever change it.
  const clientRef = useRef(queryClient)
  clientRef.current = queryClient

  useEffect(() => {
    const source = new EventSource('/events')

    const onBatch = (event: MessageEvent<string>) => {
      try {
        applyEvent(clientRef.current, JSON.parse(event.data) as StreamEvent)
      } catch {
        // A malformed frame must not take the stream down.
      }
    }
    const onResync = () => {
      // The client was away longer than the daemon's replay buffer.
      void clientRef.current.invalidateQueries()
    }

    source.onopen = () => setState('live')
    source.onerror = () => setState((prev) => (prev === 'live' ? 'connecting' : 'offline'))
    source.addEventListener('batch', onBatch as EventListener)
    source.addEventListener('scheduler', onBatch as EventListener)
    source.addEventListener('resync', onResync)

    return () => {
      source.removeEventListener('batch', onBatch as EventListener)
      source.removeEventListener('scheduler', onBatch as EventListener)
      source.removeEventListener('resync', onResync)
      source.close()
    }
  }, [])

  return state
}

function applyEvent(client: QueryClient, event: StreamEvent): void {
  if (event.scheduler) {
    client.setQueryData<SchedulerStatus>(qk.scheduler, (prev) =>
      prev
        ? { ...prev, ...event.scheduler, startedAt: prev.startedAt }
        : ({
            ...event.scheduler,
            awaitingApproval: 0,
            succeeded: 0,
            failed: 0,
            retryPending: 0,
            startedAt: event.at,
          } as SchedulerStatus),
    )
  }

  if (event.tasks?.length) {
    applyTaskDeltas(client, event.tasks)
  }
  if (event.chapters?.length) {
    applyChapterDeltas(client, event.chapters)
  }
  if (event.video) {
    applyVideoDelta(client, event.video)
  }
}

/**
 * Replaces only the changed elements of the cached arrays, so every unchanged
 * task keeps its object identity and a memoised row does not re-render.
 */
function applyTaskDeltas(client: QueryClient, deltas: TaskDelta[]): void {
  const byVideo = new Map<string, TaskDelta[]>()
  for (const delta of deltas) {
    const list = byVideo.get(delta.videoId)
    if (list) list.push(delta)
    else byVideo.set(delta.videoId, [delta])
  }

  for (const [videoId, videoDeltas] of byVideo) {
    client.setQueryData<Task[]>(qk.videoTasks(videoId), (prev) =>
      prev ? mergeTasks(prev, videoDeltas) : prev,
    )
    // A task that has just succeeded has usually written a file, and the asset
    // list is the one thing on the detail pane no delta carries. Refetched only
    // while a pane is actually showing it, and the bus already coalesces per
    // video, so a fifty-chapter render is not fifty list queries a second.
    if (videoDeltas.some((delta) => delta.state === 'succeeded')) {
      void client.invalidateQueries({ queryKey: qk.assets(videoId), refetchType: 'active' })
    }
  }
  client.setQueryData<Task[]>(qk.recentTasks, (prev) => (prev ? mergeTasks(prev, deltas) : prev))
}

function mergeTasks(previous: Task[], deltas: TaskDelta[]): Task[] {
  const index = new Map<string, TaskDelta>()
  for (const delta of deltas) index.set(delta.id, delta)

  let changed = false
  const next = previous.map((task) => {
    const delta = index.get(task.id)
    if (!delta) return task
    index.delete(task.id)
    if (
      task.state === delta.state &&
      task.attempt === delta.attempt &&
      task.stale === delta.stale &&
      (task.error ?? '') === (delta.error ?? '')
    ) {
      return task
    }
    changed = true
    return { ...task, ...delta }
  })

  // Tasks the client has not seen yet (a freshly enqueued DAG) are appended.
  if (index.size > 0) {
    changed = true
    for (const delta of index.values()) {
      next.push({
        maxAttempts: 0,
        depsRemaining: 0,
        ...delta,
      } as Task)
    }
  }
  return changed ? next : previous
}

function applyChapterDeltas(client: QueryClient, deltas: ChapterDelta[]): void {
  const byVideo = new Map<string, ChapterDelta[]>()
  for (const delta of deltas) {
    const list = byVideo.get(delta.videoId)
    if (list) list.push(delta)
    else byVideo.set(delta.videoId, [delta])
  }

  for (const [videoId, videoDeltas] of byVideo) {
    // A chapter the cache has never seen is one the blueprint has just created.
    // The delta carries none of what a chapter card shows — no summary, no
    // script, no prompts — so appending a husk would be worse than refetching:
    // the operator would be reviewing an outline rendered as blanks.
    const cached = client.getQueryData<Chapter[]>(qk.chapters(videoId))
    const known = new Set(cached?.map((c) => c.id))
    const unseen = cached !== undefined && videoDeltas.some((d) => !known.has(d.id))

    client.setQueryData<Chapter[]>(qk.chapters(videoId), (prev) => {
      if (!prev) return prev
      const index = new Map(videoDeltas.map((d) => [d.id, d]))
      let changed = false
      const next = prev.map((chapter) => {
        const delta = index.get(chapter.id)
        if (!delta) return chapter
        const merged: Chapter = {
          ...chapter,
          audioAssetId: delta.audioAssetId ?? chapter.audioAssetId,
          imageAssetIds: delta.imageAssetIds ?? chapter.imageAssetIds,
          clipAssetId: delta.clipAssetId ?? chapter.clipAssetId,
          updatedAt: delta.updatedAt,
        }
        if (
          merged.audioAssetId === chapter.audioAssetId &&
          merged.clipAssetId === chapter.clipAssetId &&
          sameIds(merged.imageAssetIds, chapter.imageAssetIds)
        ) {
          // A script arrived, which the delta does not carry; refetch lazily.
          if (delta.hasScript && !chapter.script) {
            changed = true
            return merged
          }
          return chapter
        }
        changed = true
        return merged
      })
      return changed ? next : prev
    })
    // The script body is deliberately not on the wire; fetch it when it lands.
    if (unseen || videoDeltas.some((d) => d.hasScript)) {
      void client.invalidateQueries({ queryKey: qk.chapters(videoId), refetchType: 'active' })
    }
  }
}

function sameIds(a: string[] | undefined, b: string[] | undefined): boolean {
  if (a === b) return true
  if (!a || !b || a.length !== b.length) return false
  return a.every((value, i) => value === b[i])
}

/**
 * Matches the cached video *body*, not the key it was fetched under.
 *
 * The detail pane opens a video by whichever key the URL carried — its ref, or
 * its id — while a delta only ever carries the id. Keying off the cached video's
 * own id patches the entry either way; guessing at the key silently misses it,
 * and a header that never moves while the sidebar does is the shape that bug
 * takes. The length check keeps this off ['video', key, 'tasks'|'chapters'],
 * whose bodies are arrays.
 */
function videoEntries(id: string) {
  return (query: { queryKey: readonly unknown[]; state: { data: unknown } }): boolean =>
    query.queryKey[0] === 'video' &&
    query.queryKey.length === 2 &&
    (query.state.data as Video | undefined)?.id === id
}

function applyVideoDelta(client: QueryClient, delta: VideoDelta): void {
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

  client.setQueriesData<Video>({ predicate: videoEntries(delta.id) }, patch)

  // The list view shows progress inline, so patch it rather than refetching.
  client.setQueriesData<{ videos: Video[]; total: number }>({ queryKey: ['videos'] }, (prev) => {
    if (!prev) return prev
    let changed = false
    const videos = prev.videos.map((video) => {
      if (video.id !== delta.id) return video
      const next = patch(video)
      if (next && next !== video) changed = true
      return next ?? video
    })
    return changed ? { ...prev, videos } : prev
  })

  // A finished or newly gated video changes what the detail page can offer: the
  // final render, the publish metadata, the upload receipt. None of those ride
  // on the delta, so this is a refetch rather than a patch.
  if (delta.state === 'awaiting_approval' || delta.state === 'completed') {
    void client.invalidateQueries({ predicate: videoEntries(delta.id), refetchType: 'active' })
  }
}
