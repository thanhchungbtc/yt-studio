import { Search, Trash2, X } from 'lucide-react'
import { memo, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'

import { clearRuns, useLLMConnected, useLLMRuns, type LLMRun } from '../../core/llm'
import { cn } from '../../core/utils'

/**
 * What the machine is saying, as it says it.
 *
 * The console shows raw text and parses nothing. A blueprint comes back as JSON
 * against a schema, and half of a JSON document is not a document — so the half
 * that has arrived is shown as what it is, characters, and the parsed result
 * still lands only at the end through the ordinary API. This view has no
 * opinion about the pipeline and the pipeline does not know it exists.
 *
 * Runs are blocks in the order they began rather than lines interleaved by
 * arrival. Interleaving two token streams would make both unreadable to save a
 * little vertical space, and a block also gives the runs with no stream — a
 * narration, a clip, an upload — the one shape that says a thing started, took
 * this long, and ended this way.
 *
 * Which is why nothing here distinguishes the two. A model exchange and a task
 * are the same four fields and the same four states, so they are drawn by the
 * same component, and what tells them apart is the second column of the header:
 * an exchange names the model it went to, a task says `task`.
 */
export type Filter = 'all' | 'failed' | 'running'

const FILTERS: readonly { value: Filter; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'failed', label: 'Failed' },
  { value: 'running', label: 'Running' },
]

function matchesFilter(run: LLMRun, filter: Filter): boolean {
  if (filter === 'failed') return run.error !== undefined
  if (filter === 'running') return !run.done
  return true
}

/** Where the needle appears in a run, which is also whether it appears at all. */
function hits(run: LLMRun, needle: string): number {
  if (needle === '') return 0
  const hay = `${run.label} ${run.model} ${run.text} ${run.error ?? ''}`.toLowerCase()
  let found = 0
  for (let at = hay.indexOf(needle); at !== -1; at = hay.indexOf(needle, at + needle.length)) {
    found++
  }
  return found
}

export function Console() {
  const runs = useLLMRuns()
  const connected = useLLMConnected()
  const [filter, setFilter] = useState<Filter>('all')
  const [query, setQuery] = useState('')
  const [at, setAt] = useState(0)

  const needle = query.trim().toLowerCase()
  const shown = useMemo(() => {
    const byFilter = runs.filter((run) => matchesFilter(run, filter))
    return needle === '' ? byFilter : byFilter.filter((run) => hits(run, needle) > 0)
  }, [runs, filter, needle])

  // Where the cursor is, in runs rather than in characters: a match is a block
  // you read, and stepping to the next one means bringing that block into view.
  const total = shown.length
  const cursor = total === 0 ? 0 : Math.min(at, total - 1)
  useEffect(() => setAt(0), [needle, filter])

  const body = () => {
    if (runs.length === 0) {
      return <Empty>{connected ? 'Nothing has run yet.' : 'Console is not connected.'}</Empty>
    }
    if (shown.length === 0) {
      return <Empty>{needle === '' ? 'Nothing matches that filter.' : 'No matches.'}</Empty>
    }
    return (
      // Searching parks the scroller: a view that jumps to the newest frame
      // while you are reading a match you asked it to find is a search that
      // cannot be used on a console that is still running.
      <Scroller follow={needle === '' && filter === 'all'}>
        {shown.map((run, index) => (
          <Block
            key={run.run}
            run={run}
            needle={needle}
            current={needle !== '' && index === cursor}
          />
        ))}
      </Scroller>
    )
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <Toolbar
        query={query}
        onQuery={setQuery}
        matches={needle === '' ? null : { at: total === 0 ? 0 : cursor + 1, of: total }}
        onStep={(by) => setAt((i) => (total === 0 ? 0 : (i + by + total) % total))}
        filter={filter}
        onFilter={setFilter}
        onClear={clearRuns}
        clearable={runs.length > 0}
      />
      <div className="min-h-0 flex-1">{body()}</div>
    </div>
  )
}

/** The controls, in one row under the panel's title. */
function Toolbar({
  query,
  onQuery,
  matches,
  onStep,
  filter,
  onFilter,
  onClear,
  clearable,
}: {
  query: string
  onQuery: (value: string) => void
  /** Null while nothing is being searched for. */
  matches: { at: number; of: number } | null
  onStep: (by: number) => void
  filter: Filter
  onFilter: (value: Filter) => void
  onClear: () => void
  clearable: boolean
}) {
  return (
    <div className="hairline-b flex h-[28px] shrink-0 items-center gap-1.5 px-3">
      <div className="relative flex min-w-0 flex-1 items-center">
        <Search
          aria-hidden
          className="pointer-events-none absolute left-1.5 size-[11px] text-fg-subtle"
          strokeWidth={2}
        />
        <input
          data-console-search
          value={query}
          onChange={(event) => onQuery(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Escape') onQuery('')
            if (event.key === 'Enter') onStep(event.shiftKey ? -1 : 1)
          }}
          placeholder="Find"
          spellCheck={false}
          className="control h-[19px] w-full min-w-0 py-0 pr-1.5 pl-[22px] text-xs"
        />
      </div>

      {matches ? (
        <>
          <span className="shrink-0 text-2xs tabular-nums text-fg-subtle">
            {matches.of === 0 ? 'none' : `${matches.at}/${matches.of}`}
          </span>
          <Step label="Previous match" disabled={matches.of === 0} onClick={() => onStep(-1)}>
            ‹
          </Step>
          <Step label="Next match" disabled={matches.of === 0} onClick={() => onStep(1)}>
            ›
          </Step>
          <button
            type="button"
            aria-label="Clear the search"
            onClick={() => onQuery('')}
            className="flex size-[19px] shrink-0 items-center justify-center rounded-[5px] text-fg-subtle transition-colors hover:bg-[var(--hover)] hover:text-fg"
          >
            <X className="size-[11px]" strokeWidth={2.2} />
          </button>
        </>
      ) : null}

      <div
        className="flex shrink-0 items-center gap-0.5 rounded-[5px] p-0.5"
        style={{ backgroundColor: 'var(--idle-selection)' }}
      >
        {FILTERS.map((entry) => (
          <button
            key={entry.value}
            type="button"
            aria-pressed={filter === entry.value}
            onClick={() => onFilter(entry.value)}
            className={cn(
              'rounded-[4px] px-1.5 py-[1px] text-2xs transition-colors',
              filter === entry.value ? 'text-fg' : 'text-fg-subtle hover:text-fg-muted',
            )}
            style={
              filter === entry.value
                ? { backgroundColor: 'var(--raised)', boxShadow: '0 1px 2px rgb(0 0 0 / 0.14)' }
                : undefined
            }
          >
            {entry.label}
          </button>
        ))}
      </div>

      <button
        type="button"
        aria-label="Clear the console"
        title="Clear the console"
        disabled={!clearable}
        onClick={onClear}
        className="flex size-[19px] shrink-0 items-center justify-center rounded-[5px] text-fg-subtle transition-colors hover:bg-[var(--hover)] hover:text-fg disabled:pointer-events-none disabled:opacity-35"
      >
        <Trash2 className="size-[12px]" strokeWidth={1.9} />
      </button>
    </div>
  )
}

function Step({
  label,
  disabled,
  onClick,
  children,
}: {
  label: string
  disabled: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      aria-label={label}
      disabled={disabled}
      onClick={onClick}
      className="flex size-[19px] shrink-0 items-center justify-center rounded-[5px] text-base text-fg-subtle transition-colors hover:bg-[var(--hover)] hover:text-fg disabled:pointer-events-none disabled:opacity-35"
    >
      {children}
    </button>
  )
}

function Empty({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-full items-center justify-center px-3 text-xs text-fg-subtle">
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
function Scroller({ follow, children }: { follow: boolean; children: React.ReactNode }) {
  const ref = useRef<HTMLDivElement>(null)
  const [pinned, setPinned] = useState(true)

  // Layout rather than effect: this runs after the new text has been measured
  // and before the frame is painted, so the view never shows the old bottom for
  // a frame and then jumps.
  useLayoutEffect(() => {
    const node = ref.current
    if (node && follow && pinned) node.scrollTop = node.scrollHeight
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

/**
 * Splits text around a needle, so the matches can be drawn and the rest cannot.
 * An empty needle returns the text whole, which is the ordinary case.
 */
function Highlight({ text, needle }: { text: string; needle: string }) {
  if (needle === '' || text === '') return <>{text}</>
  const parts: React.ReactNode[] = []
  const hay = text.toLowerCase()
  let from = 0
  for (let at = hay.indexOf(needle); at !== -1; at = hay.indexOf(needle, from)) {
    if (at > from) parts.push(text.slice(from, at))
    parts.push(
      <mark
        key={at}
        className="rounded-[2px] px-px"
        style={{ backgroundColor: 'var(--accent-wash-strong)', color: 'inherit' }}
      >
        {text.slice(at, at + needle.length)}
      </mark>,
    )
    from = at + needle.length
  }
  if (from < text.length) parts.push(text.slice(from))
  return <>{parts}</>
}

const Block = memo(function Block({
  run,
  needle,
  current,
}: {
  run: LLMRun
  needle: string
  /** The match the ‹ › buttons are on, scrolled to and outlined. */
  current: boolean
}) {
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (current) ref.current?.scrollIntoView({ block: 'nearest' })
  }, [current])

  return (
    <div
      ref={ref}
      className={cn('pt-2', current && 'rounded-[5px] px-1.5')}
      style={current ? { backgroundColor: 'var(--accent-wash)' } : undefined}
    >
      <div className="flex items-baseline gap-2 text-xs text-fg-subtle">
        <span className="font-semibold text-fg-muted">
          <Highlight text={run.label} needle={needle} />
        </span>
        <span className="min-w-0 truncate">
          <Highlight text={run.model} needle={needle} />
        </span>
        <span className="ml-auto shrink-0 tabular-nums">
          <Status run={run} />
        </span>
      </div>
      {run.truncated ? (
        <div className="pt-0.5 text-xs text-fg-subtle italic">
          earlier output dropped to bound the log
        </div>
      ) : null}
      {/* `pre-wrap` rather than `pre`: a model emits very long lines and a
          horizontal scrollbar on a log is a way of hiding text. `break-all`
          because what wraps here is frequently JSON, which has no spaces to
          wrap at. */}
      <pre className="font-mono text-xs leading-[1.45] break-all whitespace-pre-wrap text-fg-muted">
        <Highlight text={run.text} needle={needle} />
        {run.done ? null : <Caret />}
      </pre>
      {/* Wrapped the way the output above it is, because an error here is no
          longer always one line: ffmpeg's failures arrive with the tail of its
          stderr attached, and HTML would fold those newlines into a paragraph —
          which is how a stack of ffmpeg complaints becomes one unreadable
          sentence. */}
      {run.error ? (
        <div className="pt-0.5 font-mono text-xs break-all whitespace-pre-wrap text-[color:var(--failed)]">
          <Highlight text={run.error} needle={needle} />
        </div>
      ) : null}
    </div>
  )
})

/**
 * How the run is going, in the one place a duration belongs.
 *
 * A running one shows elapsed time counted here rather than sent — the server
 * has no reason to emit a frame a second so that a number can tick, and a clock
 * is the one thing a client can keep on its own.
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
 * The block caret at the end of a run still in flight.
 *
 * The one piece of decoration here, and it earns its place: it is what says a
 * model that has gone quiet, or a task that produces no text at all, is still
 * working. Without it a stalled generation and a finished one look the same.
 */
function Caret() {
  return (
    <span className="ml-px inline-block h-[1em] w-[0.5em] translate-y-[0.15em] animate-pulse bg-current align-baseline" />
  )
}
