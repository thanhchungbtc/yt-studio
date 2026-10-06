import { useEffect } from 'react'
import { Events } from '@wailsio/runtime'
import { create } from 'zustand'

import * as ConsoleService from '@bindings/services/consoleservice'

/**
 * What the machine is doing, right now: every model exchange as it streams, and
 * a line apiece for every task that has no text to stream.
 *
 * Separate from the pipeline events in `events.ts`: those carry *state*, merged
 * last-wins, which is right for a task that moved and wrong for text, where
 * merging two frames loses the first one's words. This is an append-only log.
 *
 * It has its own lifetime, too. The console is the only thing that reads it, so
 * the backend sends frames only while the console is shown — and it retains
 * recent exchanges, so reopening replays what was missed rather than starting
 * blank. A panel nobody is looking at costs nothing.
 */

/** One frame, as the backend sends it. Text is a delta; runs are append-only. */
interface LLMFrame {
  run: number
  videoId: string
  label: string
  model: string
  text?: string
  done?: boolean
  error?: string
  truncated?: boolean
  startedAt: string
  ms?: number
}

/** One exchange, as the console shows it. */
export interface LLMRun {
  run: number
  videoId: string
  label: string
  model: string
  text: string
  done: boolean
  error?: string
  truncated: boolean
  startedAt: string
  ms?: number
}

/**
 * What the client keeps, mirroring what the backend retains.
 *
 * Both caps matter and they cap different things. A browser left open across a
 * fifty-chapter render would otherwise accumulate every run of it, and a model
 * that has started repeating itself would otherwise grow one of them without
 * limit. Neither is a reason for a log window to become the largest thing in
 * the tab.
 *
 * The run cap matches `maxRuns` in the backend's log deliberately. A client that
 * kept fewer would drop runs the backlog had just replayed to it, so reopening
 * the panel would show less than it did a moment before it was closed — and one
 * that kept more would be holding runs nothing will ever send it again.
 */
const MAX_RUNS = 128
const MAX_RUN_CHARS = 64 * 1024

interface LLMState {
  runs: LLMRun[]
  connected: boolean
}

const useStore = create<LLMState>(() => ({ runs: [], connected: false }))

/** The exchanges to show, oldest first. */
export function useLLMRuns(): LLMRun[] {
  return useStore((s) => s.runs)
}

/** Whether the console is receiving. */
export function useLLMConnected(): boolean {
  return useStore((s) => s.connected)
}

/**
 * Empties the console.
 *
 * The client's copy only. The server retains its own backlog and replays it to
 * the next subscriber, so reopening the panel brings the runs back — which is
 * what a log does.
 */
export function clearRuns(): void {
  drop()
  useStore.setState({ runs: [] })
}

/**
 * Applies one frame.
 *
 * A run it has not seen is a new exchange; one it has appends. That is the same
 * operation for a live frame and for a backlog frame carrying everything so far,
 * which is what lets a console opened halfway through a generation be served by
 * the code that serves one that was open from the start.
 */
function apply(runs: LLMRun[], frame: LLMFrame): LLMRun[] {
  const index = runs.findIndex((r) => r.run === frame.run)
  const previous = index === -1 ? undefined : runs[index]

  const next: LLMRun = {
    run: frame.run,
    videoId: frame.videoId,
    label: frame.label,
    model: frame.model,
    text: clamp((previous?.text ?? '') + (frame.text ?? '')),
    done: frame.done ?? previous?.done ?? false,
    error: frame.error ?? previous?.error,
    truncated: frame.truncated ?? previous?.truncated ?? false,
    startedAt: frame.startedAt,
    ms: frame.ms ?? previous?.ms,
  }

  if (previous) {
    runs[index] = next
    return runs
  }
  runs.push(next)
  return runs.length > MAX_RUNS ? runs.slice(-MAX_RUNS) : runs
}

// One store write per paint; the timeout covers hidden windows, where rAF stops.
let queue: LLMFrame[] = []
let scheduled = 0
let fallback = 0

function flush(): void {
  cancelAnimationFrame(scheduled)
  window.clearTimeout(fallback)
  scheduled = 0
  fallback = 0
  const frames = queue
  queue = []
  if (frames.length === 0) return
  useStore.setState((state) => {
    let runs = state.runs.slice()
    for (const frame of frames) runs = apply(runs, frame)
    return { runs }
  })
}

function enqueue(frame: LLMFrame): void {
  queue.push(frame)
  if (scheduled || fallback) return
  scheduled = requestAnimationFrame(flush)
  fallback = window.setTimeout(flush, 100)
}

function drop(): void {
  cancelAnimationFrame(scheduled)
  window.clearTimeout(fallback)
  scheduled = 0
  fallback = 0
  queue = []
}

/** Keeps the tail, which is the half being watched. */
function clamp(text: string): string {
  return text.length <= MAX_RUN_CHARS ? text : text.slice(text.length - MAX_RUN_CHARS)
}

/** One opening's frames; its first batch is the backlog. */
interface ConsoleBatch {
  session: number
  reset?: boolean
  frames: LLMFrame[] | null
}

// Increases across reloads: the backend ignores older sessions.
let sessions = Date.now() * 1000

/** Streams for as long as the caller is mounted. Mounted by the console alone. */
export function useLLMStream(): void {
  useEffect(() => {
    const session = ++sessions
    const off = Events.On('console:frames', (event) => {
      const batch = event.data as ConsoleBatch
      if (batch.session !== session) return
      if (batch.reset) {
        drop()
        useStore.setState({ runs: [], connected: true })
      }
      for (const frame of batch.frames ?? []) enqueue(frame)
    })
    void ConsoleService.Open(session)

    return () => {
      off()
      void ConsoleService.Close(session)
      drop()
      useStore.setState({ runs: [], connected: false })
    }
  }, [])
}
