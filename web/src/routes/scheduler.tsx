import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useVirtualizer } from '@tanstack/react-virtual'
import { memo, useMemo, useRef, useState } from 'react'

import { PageHeader } from '@/components/app-shell'
import { PoolMeter } from '@/components/pool-meter'
import { RerunDialog } from '@/components/stale'
import { TaskStateBadge } from '@/components/state-badges'
import { Button } from '@/components/ui/button'
import { Select } from '@/components/ui/field'
import {
  EmptyState,
  ErrorNotice,
  Panel,
  PanelHeader,
  PanelTitle,
  Skeleton,
} from '@/components/ui/primitives'
import { api, qk } from '@/core/api'
import { formatDuration, formatRelative, poolLabel, taskLabel } from '@/core/format'
import type { Task, TaskState } from '@/core/types'
import { cn } from '@/core/utils'

const ROW_HEIGHT = 30
const COLUMNS = '110px 100px 52px 76px 104px 56px minmax(0,1fr) 84px 64px'

/**
 * The operator console: pool utilisation, queue depth, and the live task table.
 *
 * The table is virtualised and every row is memoised, so a fifty-chapter render
 * updating hundreds of tasks scrolls at 60 fps and a single task event
 * re-renders exactly one row.
 */
export function SchedulerRoute() {
  const [stateFilter, setStateFilter] = useState<TaskState | ''>('')
  const [poolFilter, setPoolFilter] = useState('')

  const status = useQuery({
    queryKey: qk.scheduler,
    queryFn: api.schedulerStatus,
    refetchInterval: 30_000,
  })
  const tasks = useQuery({
    queryKey: qk.recentTasks,
    queryFn: () => api.listRecentTasks(500),
    refetchInterval: 30_000,
  })
  // The task stream carries video ids; the operator thinks in refs.
  const videos = useQuery({ queryKey: qk.videos({}), queryFn: () => api.listVideos({}) })

  const refById = useMemo(
    () => new Map((videos.data?.videos ?? []).map((v) => [v.id, v.ref])),
    [videos.data],
  )

  const filtered = useMemo(() => {
    const rows = tasks.data ?? []
    if (!stateFilter && !poolFilter) return rows
    return rows.filter(
      (t) => (!stateFilter || t.state === stateFilter) && (!poolFilter || t.pool === poolFilter),
    )
  }, [tasks.data, stateFilter, poolFilter])

  const totals = status.data

  return (
    <>
      <PageHeader
        title="Scheduler"
        subtitle={
          totals
            ? `${totals.videos} video${totals.videos === 1 ? '' : 's'} in flight · up ${formatDuration(totals.uptimeSeconds)} · limits apply without a restart`
            : 'Loading…'
        }
      />

      <div className="grid shrink-0 grid-cols-[minmax(0,1fr)_340px] gap-3 p-4 pb-0">
        <Panel>
          <PanelHeader>
            <PanelTitle>Pools</PanelTitle>
            <Link to="/settings" className="text-[11px] text-subtle hover:text-fg">
              limits are settings rows
            </Link>
          </PanelHeader>
          <div className="space-y-2 px-3 py-3">
            {status.isPending && <Skeleton className="h-24" />}
            {status.data?.pools.map((pool) => (
              <PoolMeter key={pool.pool} stat={pool} />
            ))}
          </div>
        </Panel>

        <Panel className="self-start">
          <PanelHeader>
            <PanelTitle>Queue</PanelTitle>
          </PanelHeader>
          <div className="grid grid-cols-3 gap-px overflow-hidden rounded-b-[var(--radius-md)] bg-[hsl(var(--border))]">
            <Counter label="Running" value={totals?.running ?? 0} tone="accent" />
            <Counter label="Ready" value={totals?.ready ?? 0} tone="info" />
            <Counter label="Blocked" value={totals?.blocked ?? 0} tone="muted" />
            <Counter label="Retrying" value={totals?.retryPending ?? 0} tone="warning" />
            <Counter label="Gated" value={totals?.awaitingApproval ?? 0} tone="warning" />
            <Counter label="Failed" value={totals?.failed ?? 0} tone="danger" />
          </div>
        </Panel>
      </div>

      <div className="flex shrink-0 items-center gap-2 px-4 pt-3">
        <Select
          value={stateFilter}
          onChange={(e) => setStateFilter(e.target.value as TaskState | '')}
          aria-label="Filter by task state"
          className="w-40"
        >
          <option value="">All states</option>
          {(
            [
              'blocked',
              'ready',
              'running',
              'awaiting_approval',
              'succeeded',
              'failed',
              'cancelled',
            ] as TaskState[]
          ).map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </Select>
        <Select
          value={poolFilter}
          onChange={(e) => setPoolFilter(e.target.value)}
          aria-label="Filter by pool"
          className="w-36"
        >
          <option value="">All pools</option>
          {status.data?.pools.map((p) => (
            <option key={p.pool} value={p.pool}>
              {poolLabel(p.pool)}
            </option>
          ))}
        </Select>
        {(stateFilter || poolFilter) && (
          <Button
            size="sm"
            variant="ghost"
            onClick={() => {
              setStateFilter('')
              setPoolFilter('')
            }}
          >
            Clear
          </Button>
        )}
        <span className="ml-auto text-[11.5px] tabular text-subtle">
          {filtered.length} task{filtered.length === 1 ? '' : 's'}
        </span>
      </div>

      <div className="min-h-0 flex-1 p-4 pt-2">
        <Panel className="flex h-full flex-col overflow-hidden">
          <TaskTableHeader />
          {tasks.isPending && <Skeleton className="m-3 h-40" />}
          {tasks.isError && <ErrorNotice error={tasks.error} className="m-3" />}
          {!tasks.isPending && filtered.length === 0 && (
            <EmptyState title="Nothing scheduled" description="Start a video to see tasks here." />
          )}
          {filtered.length > 0 && <TaskTable tasks={filtered} refById={refById} />}
        </Panel>
      </div>
    </>
  )
}

function Counter({
  label,
  value,
  tone,
}: {
  label: string
  value: number
  tone: 'accent' | 'info' | 'warning' | 'danger' | 'muted'
}) {
  const colour = {
    accent: 'text-[hsl(var(--accent))]',
    info: 'text-[hsl(var(--info))]',
    warning: 'text-[hsl(var(--warning))]',
    danger: 'text-[hsl(var(--danger))]',
    muted: 'text-muted',
  }[tone]
  return (
    <div className="flex flex-col gap-0.5 bg-[hsl(var(--bg-elevated))] px-3 py-2">
      <span className="text-[10.5px] uppercase tracking-wide text-subtle">{label}</span>
      <span
        className={cn(
          'tabular text-[18px] font-semibold leading-none',
          value === 0 ? 'text-subtle' : colour,
        )}
      >
        {value}
      </span>
    </div>
  )
}

function TaskTableHeader() {
  return (
    <div
      className="grid shrink-0 items-center gap-2 border-b border-[hsl(var(--border))] bg-subtle px-3 py-1.5 text-[11px] uppercase tracking-wide text-subtle"
      style={{ gridTemplateColumns: COLUMNS }}
    >
      <span>Video</span>
      <span>Task</span>
      <span className="text-right">Ch</span>
      <span>Pool</span>
      <span>State</span>
      <span className="text-right">Try</span>
      <span>Error</span>
      <span className="text-right">Updated</span>
      <span />
    </div>
  )
}

function TaskTable({ tasks, refById }: { tasks: Task[]; refById: Map<string, string> }) {
  const parentRef = useRef<HTMLDivElement>(null)
  const queryClient = useQueryClient()
  const [rerunning, setRerunning] = useState<Task | null>(null)

  // The cascade, offered only from inside the dialog: this resets everything
  // downstream too, which is never what an operator scanning a task table
  // meant by "run it again".
  const retry = useMutation({
    mutationFn: (id: string) => api.retryTask(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: qk.recentTasks })
      void queryClient.invalidateQueries({ queryKey: qk.scheduler })
      setRerunning(null)
    },
  })

  const virtualizer = useVirtualizer({
    count: tasks.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 20,
  })

  return (
    <div ref={parentRef} className="min-h-0 flex-1 overflow-y-auto">
      <div className="relative w-full" style={{ height: virtualizer.getTotalSize() }}>
        {virtualizer.getVirtualItems().map((item) => {
          const task = tasks[item.index]
          if (!task) return null
          return (
            <div
              key={task.id}
              className="absolute left-0 top-0 w-full"
              style={{ height: ROW_HEIGHT, transform: `translateY(${item.start}px)` }}
            >
              <SchedulerTaskRow
                task={task}
                videoRef={refById.get(task.videoId)}
                onRerun={setRerunning}
              />
            </div>
          )
        })}
      </div>

      {rerunning && (
        <RerunDialog
          open
          onOpenChange={(open) => !open && setRerunning(null)}
          videoRef={refById.get(rerunning.videoId) ?? rerunning.videoId}
          videoId={rerunning.videoId}
          taskIds={[rerunning.id]}
          what={taskLabel(rerunning.kind) + (rerunning.ordinal > 0 ? ` ${rerunning.ordinal}` : '')}
          onCascade={() => retry.mutate(rerunning.id)}
          cascadePending={retry.isPending}
        />
      )}
    </div>
  )
}

/** Memoised on the task object: a delta replaces only that element. */
const SchedulerTaskRow = memo(function SchedulerTaskRow({
  task,
  videoRef,
  onRerun,
}: {
  task: Task
  videoRef: string | undefined
  onRerun: (task: Task) => void
}) {
  return (
    <div
      className="grid h-full items-center gap-2 border-b border-[hsl(var(--border))] px-3 text-[12px] hover:bg-[hsl(var(--bg-hover))]"
      style={{ gridTemplateColumns: COLUMNS }}
    >
      <Link
        to="/videos/$ref"
        params={{ ref: videoRef ?? task.videoId }}
        className="truncate font-mono text-[11px] font-semibold text-[hsl(var(--accent))] hover:underline"
      >
        {videoRef ?? task.videoId.slice(0, 10)}
      </Link>
      <span className="truncate text-fg">
        {taskLabel(task.kind)}
        {task.index >= 0 && <span className="text-subtle"> {task.index + 1}</span>}
      </span>
      <span className="tabular text-right text-muted">{task.ordinal > 0 ? task.ordinal : '—'}</span>
      <span className="truncate text-muted">{poolLabel(task.pool)}</span>
      <span>
        <TaskStateBadge state={task.state} />
      </span>
      <span className="tabular text-right text-muted">
        {task.attempt}/{task.maxAttempts}
      </span>
      <span className="truncate text-[hsl(var(--danger))]" title={task.error}>
        {task.error ?? ''}
      </span>
      <span className="text-right text-subtle">{formatRelative(task.updatedAt)}</span>
      <span className="text-right">
        {task.state !== 'running' && task.state !== 'blocked' && (
          <Button size="xs" variant="ghost" onClick={() => onRerun(task)}>
            Re-run
          </Button>
        )}
      </span>
    </div>
  )
})

export default SchedulerRoute
