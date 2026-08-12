import { useQuery } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useMemo, useRef, useState } from 'react'

import { Select } from '../ui/controls'
import { ErrorNotice, Skeleton } from '../ui/primitives'
import { PoolMeter, TaskStateDot } from '../ui/status'
import { api, qk } from '@/core/api'
import {
  formatCompactDuration,
  formatDuration,
  formatRelative,
  poolLabel,
  taskLabel,
  taskSeconds,
  taskStateLabel,
} from '@/core/format'
import type { PoolName, TaskState } from '@/core/types'
import { cn } from '@/core/utils'

const STATES: TaskState[] = [
  'running',
  'ready',
  'blocked',
  'awaiting_approval',
  'failed',
  'succeeded',
  'cancelled',
]

/**
 * The operator console, in the bottom panel rather than in the rail.
 *
 * It belongs here for the same reason the reference's problems and output views
 * do: it is about the machine, not about a document, and you want it *under*
 * what you are working on rather than instead of it.
 */
export function ConsoleView() {
  const status = useQuery({
    queryKey: qk.scheduler,
    queryFn: api.schedulerStatus,
    refetchInterval: 30_000,
  })
  const tasks = useQuery({ queryKey: qk.recentTasks, queryFn: () => api.listRecentTasks(300) })

  const [state, setState] = useState<TaskState | ''>('')
  const [pool, setPool] = useState<PoolName | ''>('')

  const rows = useMemo(
    () =>
      (tasks.data ?? []).filter((t) => (!state || t.state === state) && (!pool || t.pool === pool)),
    [tasks.data, state, pool],
  )

  // One clock for the whole table, so two hundred rows share a tick.
  const now = Date.now()
  const totals = status.data

  const parent = useRef<HTMLDivElement>(null)
  const virtual = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parent.current,
    estimateSize: () => 24,
    overscan: 16,
  })

  return (
    <div className="flex h-full min-h-0">
      <aside className="w-[300px] shrink-0 overflow-y-auto border-r border-[hsl(var(--border))] p-3">
        <div className="space-y-2">
          {status.isPending && <Skeleton className="h-24" />}
          {status.isError && <ErrorNotice error={status.error} />}
          {status.data?.pools.map((p) => (
            <PoolMeter key={p.pool} stat={p} />
          ))}
        </div>

        <div className="mt-3 grid grid-cols-3 gap-px overflow-hidden rounded-[var(--radius-sm)] bg-[hsl(var(--border))]">
          <Counter label="Running" value={totals?.running ?? 0} tone="accent" />
          <Counter label="Ready" value={totals?.ready ?? 0} tone="info" />
          <Counter label="Blocked" value={totals?.blocked ?? 0} tone="neutral" />
          <Counter label="Retrying" value={totals?.retryPending ?? 0} tone="warning" />
          <Counter label="Gated" value={totals?.awaitingApproval ?? 0} tone="warning" />
          <Counter label="Failed" value={totals?.failed ?? 0} tone="danger" />
        </div>

        {totals && (
          <p className="mt-2 text-[10.5px] text-subtle">
            {totals.videos} video{totals.videos === 1 ? '' : 's'} in flight · up{' '}
            {formatDuration(totals.uptimeSeconds)}
          </p>
        )}
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex h-7 shrink-0 items-center gap-1.5 border-b border-[hsl(var(--border))] px-2">
          <span className="tabular text-[10.5px] text-subtle">{rows.length} tasks</span>
          <div className="ml-auto flex items-center gap-1.5">
            <Select
              value={state}
              aria-label="Filter by state"
              className="h-6 w-32 text-[11px]"
              onChange={(event) => setState(event.target.value as TaskState | '')}
            >
              <option value="">Any state</option>
              {STATES.map((value) => (
                <option key={value} value={value}>
                  {taskStateLabel(value)}
                </option>
              ))}
            </Select>
            <Select
              value={pool}
              aria-label="Filter by pool"
              className="h-6 w-28 text-[11px]"
              onChange={(event) => setPool(event.target.value as PoolName | '')}
            >
              <option value="">Any pool</option>
              {(status.data?.pools ?? []).map((p) => (
                <option key={p.pool} value={p.pool}>
                  {poolLabel(p.pool)}
                </option>
              ))}
            </Select>
          </div>
        </div>

        <div ref={parent} className="min-h-0 flex-1 overflow-y-auto">
          {tasks.isPending && <Skeleton className="m-3 h-40" />}
          <div className="relative w-full" style={{ height: virtual.getTotalSize() }}>
            {virtual.getVirtualItems().map((item) => {
              const task = rows[item.index]
              if (!task) return null
              return (
                <div
                  key={task.id}
                  className="absolute inset-x-0 top-0 flex items-center gap-2.5 px-3 text-[11.5px] transition-colors hover:bg-[hsl(var(--bg-hover))]"
                  style={{ height: item.size, transform: `translateY(${item.start}px)` }}
                >
                  <TaskStateDot state={task.state} stale={task.stale} />
                  <span className="w-28 shrink-0 truncate font-medium text-fg">
                    {taskLabel(task.kind)}
                  </span>
                  <span className="tabular w-10 shrink-0 text-subtle">
                    {task.ordinal > 0 ? `#${task.ordinal}` : ''}
                  </span>
                  <span className="w-20 shrink-0 truncate text-muted">{poolLabel(task.pool)}</span>
                  <span className="w-24 shrink-0 truncate text-muted">
                    {taskStateLabel(task.state)}
                  </span>
                  <span className="min-w-0 flex-1 truncate text-[hsl(var(--danger))]">
                    {task.error ?? ''}
                  </span>
                  <span className="tabular w-16 shrink-0 text-right text-subtle">
                    {formatCompactDuration(taskSeconds(task, now))}
                  </span>
                  <span className="tabular w-16 shrink-0 text-right text-subtle">
                    {formatRelative(task.updatedAt)}
                  </span>
                </div>
              )
            })}
          </div>
        </div>
      </div>
    </div>
  )
}

const TONES = {
  accent: 'text-[hsl(var(--accent))]',
  info: 'text-[hsl(var(--info))]',
  warning: 'text-[hsl(var(--warning))]',
  danger: 'text-[hsl(var(--danger))]',
  neutral: 'text-muted',
} as const

function Counter({
  label,
  value,
  tone,
}: {
  label: string
  value: number
  tone: keyof typeof TONES
}) {
  return (
    <div className="bg-[hsl(var(--bg-elevated))] px-2 py-1.5">
      <p className={cn('tabular text-[15px] font-semibold leading-tight', TONES[tone])}>{value}</p>
      <p className="text-[9.5px] uppercase tracking-wide text-subtle">{label}</p>
    </div>
  )
}
