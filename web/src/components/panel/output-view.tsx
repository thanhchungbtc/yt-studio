import { useVirtualizer } from '@tanstack/react-virtual'
import { useEffect, useRef, useState } from 'react'

import { IconButton } from '../ui/controls'
import { Tooltip } from '../ui/primitives'
import { taskLabel, taskStateLabel } from '@/core/format'
import { subscribeStream } from '@/core/events'
import type { StreamEvent } from '@/core/types'
import { cn } from '@/core/utils'
import { Trash2, ArrowDownToLine } from 'lucide-react'

interface Line {
  id: number
  at: string
  tone: 'info' | 'good' | 'warn' | 'bad'
  text: string
}

/** Enough to scroll back through a chapter's worth of work, bounded so a long
 *  session cannot grow the tab into a memory leak. */
const LIMIT = 2000

/**
 * The output log: the event stream as it arrives.
 *
 * Everything else in this window is the *result* of the stream — the cache
 * patched into shape. This is the stream itself, which is the thing you want
 * when the question is "is it doing anything?" rather than "what does it hold?".
 */
export function OutputView() {
  const [lines, setLines] = useState<Line[]>([])
  const [follow, setFollow] = useState(true)
  const parent = useRef<HTMLDivElement>(null)
  const counter = useRef(0)

  useEffect(
    () =>
      subscribeStream((event) => {
        const next = format(event, () => (counter.current += 1))
        if (next.length === 0) return
        setLines((prev) => {
          const merged = prev.concat(next)
          return merged.length > LIMIT ? merged.slice(merged.length - LIMIT) : merged
        })
      }),
    [],
  )

  const rows = useVirtualizer({
    count: lines.length,
    getScrollElement: () => parent.current,
    estimateSize: () => 18,
    overscan: 20,
  })

  // Tail the log only while the operator has not scrolled away from the bottom;
  // yanking the viewport back mid-read is the one thing a log must not do.
  useEffect(() => {
    if (follow && lines.length > 0) rows.scrollToIndex(lines.length - 1)
  }, [follow, lines.length, rows])

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-7 shrink-0 items-center gap-1 border-b border-[hsl(var(--border))] px-2 no-select">
        <span className="tabular text-[10.5px] text-subtle">{lines.length} events</span>
        <div className="ml-auto flex items-center gap-0.5">
          <Tooltip label={follow ? 'Following the tail' : 'Follow the tail'} side="top">
            <IconButton
              aria-label="Follow the tail"
              active={follow}
              onClick={() => setFollow((prev) => !prev)}
            >
              <ArrowDownToLine className="h-3.5 w-3.5" />
            </IconButton>
          </Tooltip>
          <Tooltip label="Clear" side="top">
            <IconButton aria-label="Clear the log" onClick={() => setLines([])}>
              <Trash2 className="h-3.5 w-3.5" />
            </IconButton>
          </Tooltip>
        </div>
      </div>

      <div
        ref={parent}
        onWheel={(event) => {
          // Any upward scroll is a request to stop being dragged along.
          if (event.deltaY < 0) setFollow(false)
        }}
        className="min-h-0 flex-1 overflow-y-auto font-mono text-[11px]"
      >
        {lines.length === 0 ? (
          <p className="px-3 py-6 text-center font-sans text-[12px] text-subtle">
            Nothing yet. Task and scheduler frames appear here as they arrive.
          </p>
        ) : (
          <div className="relative w-full" style={{ height: rows.getTotalSize() }}>
            {rows.getVirtualItems().map((row) => {
              const line = lines[row.index]
              if (!line) return null
              return (
                <div
                  key={line.id}
                  className="absolute inset-x-0 top-0 flex items-center gap-2 px-3 leading-[18px]"
                  style={{ height: row.size, transform: `translateY(${row.start}px)` }}
                >
                  <span className="shrink-0 text-subtle">{line.at}</span>
                  <span
                    className={cn(
                      'min-w-0 flex-1 truncate',
                      line.tone === 'good' && 'text-[hsl(var(--success))]',
                      line.tone === 'warn' && 'text-[hsl(var(--warning))]',
                      line.tone === 'bad' && 'text-[hsl(var(--danger))]',
                      line.tone === 'info' && 'text-muted',
                    )}
                  >
                    {line.text}
                  </span>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}

function clock(iso: string): string {
  const date = new Date(iso)
  return Number.isNaN(date.getTime())
    ? '--:--:--'
    : date.toLocaleTimeString(undefined, { hour12: false })
}

/**
 * One frame, as lines. Scheduler frames are summarised rather than listed —
 * they arrive on a timer and would otherwise drown everything worth reading.
 */
function format(event: StreamEvent, nextId: () => number): Line[] {
  const at = clock(event.at)
  const lines: Line[] = []

  for (const task of event.tasks ?? []) {
    const where = task.ordinal > 0 ? `#${task.ordinal}` : ''
    const detail = task.error ? ` — ${task.error}` : ''
    lines.push({
      id: nextId(),
      at,
      tone:
        task.state === 'failed'
          ? 'bad'
          : task.state === 'awaiting_approval'
            ? 'warn'
            : task.state === 'succeeded'
              ? 'good'
              : 'info',
      text: `${taskLabel(task.kind)}${where} ${taskStateLabel(task.state).toLowerCase()}${detail}`,
    })
  }

  if (event.video) {
    lines.push({
      id: nextId(),
      at,
      tone: event.video.failed > 0 ? 'bad' : 'info',
      text: `video ${event.video.state} ${event.video.done}/${event.video.total}`,
    })
  }

  if (event.scheduler && lines.length === 0) {
    const busy = event.scheduler.pools.reduce((sum, pool) => sum + pool.inFlight, 0)
    lines.push({
      id: nextId(),
      at,
      tone: 'info',
      text: `scheduler — ${busy} running, ${event.scheduler.ready} ready, ${event.scheduler.blocked} blocked`,
    })
  }

  return lines
}
