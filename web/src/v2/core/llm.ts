import { useEffect } from 'react'
import { create } from 'zustand'

/**
 * What the language models are producing, right now.
 *
 * A second `EventSource`, on `/llm`, deliberately separate from the one in
 * `events.ts`. That stream carries *state*: frames are coalesced per video and
 * merged last-wins, which is right for a task that moved and wrong for text,
 * where merging two frames loses the first one's words. This carries an
 * append-only log, so it gets its own connection.
 *
 * It also gets its own lifetime. The console is the only thing that reads it,
 * so the connection opens when the console is shown and closes when it is
 * hidden — and the server retains recent exchanges, so reopening replays what
 * was missed rather than starting blank. A panel nobody is looking at costs
 * nothing at all, which is the whole reason this is not folded into the stream
 * that must always be connected.
 *
 * Not in the query cache, for the reason the scheduler snapshot is not: there
 * is no endpoint behind it that anything fetches, so a cache entry would be a
 * fetch that never happens wrapped around a push that always does.
 */

/** One frame, as the server sends it. Text is a delta; runs are append-only. */
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
 * What the client keeps, mirroring what the server retains.
 *
 * Both caps matter and they cap different things. A browser left open across a
 * fifty-chapter render would otherwise accumulate every exchange of it, and a
 * model that has started repeating itself would otherwise grow one of them
 * without limit. Neither is a reason for a log window to become the largest
 * thing in the tab.
 */
const MAX_RUNS = 32
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
 * Applies one frame.
 *
 * A run it has not seen is a new exchange; one it has appends. That is the same
 * operation for a live frame and for a backlog frame carrying everything so far,
 * which is what lets a console opened halfway through a generation be served by
 * the code that serves one that was open from the start.
 */
function apply(frame: LLMFrame): void {
  useStore.setState((state) => {
    const index = state.runs.findIndex((r) => r.run === frame.run)
    const previous = index === -1 ? undefined : state.runs[index]

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
      const runs = state.runs.slice()
      runs[index] = next
      return { runs }
    }
    // Appended, then trimmed from the front: the server sends them in the order
    // they began, and that is the order they are read in.
    return { runs: [...state.runs, next].slice(-MAX_RUNS) }
  })
}

/** Keeps the tail, which is the half being watched. */
function clamp(text: string): string {
  return text.length <= MAX_RUN_CHARS ? text : text.slice(text.length - MAX_RUN_CHARS)
}

/**
 * Connects for as long as the caller is mounted.
 *
 * Mounted by the console and by nothing else. Two consoles would mean two
 * connections writing the same store, which is wasteful rather than wrong — but
 * there is only ever one panel, so it does not arise.
 */
export function useLLMStream(): void {
  useEffect(() => {
    const source = new EventSource('/llm')

    const onFrame = (event: MessageEvent<string>) => {
      try {
        apply(JSON.parse(event.data) as LLMFrame)
      } catch {
        // A malformed frame is dropped, not thrown. The stream outlives any one
        // bad message.
      }
    }

    // Cleared on *every* open, not once on mount. `EventSource` reconnects on
    // its own without this effect running again, and every connection is served
    // the backlog from the beginning — appending that onto runs already applied
    // would print the last few minutes twice. The browser fires this before it
    // delivers a byte of the body, so the reset always lands first.
    source.onopen = () => useStore.setState({ runs: [], connected: true })
    source.onerror = () => useStore.setState({ connected: false })
    source.addEventListener('llm', onFrame as EventListener)

    return () => {
      source.removeEventListener('llm', onFrame as EventListener)
      source.close()
      useStore.setState({ runs: [], connected: false })
    }
  }, [])
}
