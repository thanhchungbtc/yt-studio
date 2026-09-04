import { useEffect, useLayoutEffect, useRef, useState } from 'react'

import { useLLMConnected, useLLMRuns, type LLMRun } from '../../core/llm'

/**
 * What the models are saying, as they say it.
 *
 * The console shows raw text and parses nothing. A blueprint comes back as JSON
 * against a schema, and half of a JSON document is not a document — so the half
 * that has arrived is shown as what it is, characters, and the parsed result
 * still lands only at the end through the ordinary API. This view has no
 * opinion about the pipeline and the pipeline does not know it exists.
 *
 * Exchanges are blocks in the order they began rather than lines interleaved by
 * arrival. Two run at once here at most — the LLM pool's limit — and a block
 * per exchange keeps each one readable, where interleaving two token streams
 * would make both unreadable to save a little vertical space.
 */
export function Console() {
  const runs = useLLMRuns()
  const connected = useLLMConnected()

  if (runs.length === 0) {
    return (
      <Empty>{connected ? 'Nothing has been generated yet.' : 'Console is not connected.'}</Empty>
    )
  }
  return (
    <Scroller>
      {runs.map((run) => (
        <Exchange key={run.run} run={run} />
      ))}
    </Scroller>
  )
}

function Empty({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-full items-center justify-center px-3 text-[11px] text-tertiary">
      {children}
    </div>
  )
}

/**
 * The scrolling body, which follows the output *unless* the reader has moved.
 *
 * A log that always jumps to the bottom cannot be read while it is being
 * written, and one that never does has to be chased. So the rule is the one
 * every terminal uses: pinned to the end until you scroll away from it, pinned
 * again the moment you come back. Nothing is announced and there is no button —
 * the scrollbar is the control.
 */
function Scroller({ children }: { children: React.ReactNode }) {
  const ref = useRef<HTMLDivElement>(null)
  const [pinned, setPinned] = useState(true)

  // Layout rather than effect: this runs after the new text has been measured
  // and before the frame is painted, so the view never shows the old bottom for
  // a frame and then jumps.
  useLayoutEffect(() => {
    const node = ref.current
    if (node && pinned) node.scrollTop = node.scrollHeight
  })

  useEffect(() => {
    const node = ref.current
    if (!node) return
    const onScroll = () => {
      // A few pixels of slack: a scroll position is fractional on a retina
      // display, and an exact comparison unpins the view at the bottom.
      const distance = node.scrollHeight - node.scrollTop - node.clientHeight
      setPinned(distance < 8)
    }
    node.addEventListener('scroll', onScroll, { passive: true })
    return () => node.removeEventListener('scroll', onScroll)
  }, [])

  return (
    <div ref={ref} className="h-full overflow-y-auto px-3 pb-2">
      {children}
    </div>
  )
}

/** One exchange: what it was, and what came out of it. */
function Exchange({ run }: { run: LLMRun }) {
  return (
    <div className="pt-2">
      <div className="flex items-baseline gap-2 text-[11px] text-tertiary">
        <span className="font-semibold text-secondary">{run.label}</span>
        <span className="min-w-0 truncate">{run.model}</span>
        <span className="ml-auto shrink-0 tabular-nums">
          <Status run={run} />
        </span>
      </div>
      {run.truncated ? (
        <div className="pt-0.5 text-[11px] text-tertiary italic">
          earlier output dropped to bound the log
        </div>
      ) : null}
      {/* `pre-wrap` rather than `pre`: a model emits very long lines and a
          horizontal scrollbar on a log is a way of hiding text. `break-all`
          because what wraps here is frequently JSON, which has no spaces to
          wrap at. */}
      <pre className="font-mono text-[11px] leading-[1.45] break-all whitespace-pre-wrap text-secondary">
        {run.text}
        {run.done ? null : <Caret />}
      </pre>
      {run.error ? (
        <div className="pt-0.5 font-mono text-[11px] text-[color:var(--failed)]">{run.error}</div>
      ) : null}
    </div>
  )
}

/**
 * How the exchange is going, in the one place a duration belongs.
 *
 * A running exchange shows elapsed time counted here rather than sent — the
 * server has no reason to emit a frame a second so that a number can tick, and
 * a clock is the one thing a client can keep on its own.
 */
function Status({ run }: { run: LLMRun }) {
  const elapsed = useElapsed(run.startedAt, run.done)
  if (run.error) return <span className="text-[color:var(--failed)]">failed</span>
  if (run.done) return <>{seconds(run.ms ?? 0)}</>
  return <>{seconds(elapsed)}</>
}

function seconds(ms: number): string {
  return `${(ms / 1000).toFixed(1)}s`
}

/** Milliseconds since a start, ticking while it is still running. */
function useElapsed(startedAt: string, done: boolean): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (done) return
    const timer = window.setInterval(() => setNow(Date.now()), 100)
    return () => window.clearInterval(timer)
  }, [done])
  const started = Date.parse(startedAt)
  if (Number.isNaN(started)) return 0
  return Math.max(0, now - started)
}

/**
 * The block caret at the end of a running exchange.
 *
 * The one piece of decoration here, and it earns its place: it is what says a
 * model that has gone quiet is still connected. Without it a stalled generation
 * and a finished one look the same.
 */
function Caret() {
  return (
    <span className="ml-px inline-block h-[1em] w-[0.5em] translate-y-[0.15em] animate-pulse bg-current align-baseline" />
  )
}
