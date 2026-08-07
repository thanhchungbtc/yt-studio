import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  ArrowDown,
  ArrowUp,
  Check,
  ChevronDown,
  ChevronRight,
  Copy,
  Eye,
  Layers,
  ListTree,
  Lock,
  Paperclip,
  RefreshCw,
  Rows3,
  X,
} from 'lucide-react'
import type { KeyboardEvent, ReactNode } from 'react'
import { memo, useCallback, useDeferredValue, useEffect, useMemo, useRef, useState } from 'react'

import { AssetPreview, useAssetViewer } from '@/components/asset-viewer'
import { RerunDialog, StaleDot } from '@/components/stale'
import { TaskStateBadge, TaskStateDot } from '@/components/state-badges'
import { Badge, type Tone } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  ContextMenu,
  ContextMenuItem,
  ContextMenuLabel,
  ContextMenuSeparator,
} from '@/components/ui/menu'
import {
  Divider,
  EmptyState,
  ErrorNotice,
  FilterChip,
  KeyValue,
  Mono,
  Progress,
  SearchField,
  Segmented,
  Skeleton,
  Tooltip,
} from '@/components/ui/primitives'
import { api, qk } from '@/lib/api'
import type { ViewerItem } from '@/lib/assets'
import {
  formatAbsolute,
  formatCompactDuration,
  formatRelative,
  poolLabel,
  taskKindRank,
  taskLabel,
  taskSeconds,
  taskStateLabel,
} from '@/lib/format'
import { useHotkeys } from '@/lib/hotkeys'
import type { Chapter, Task, TaskState, Video } from '@/lib/types'
import { cn } from '@/lib/utils'
import { usePersisted } from '@/lib/workspace'

/*
  The task table.

  A video's task list is the closest thing the server has to a log, and at fifty
  chapters it is several hundred rows — so it is a filtered, grouped, virtualised
  table rather than a dump. Three decisions shape it:

  - The counts live on the filters. Which lens is worth opening is the question
    an operator arrives with, and a chip that reads "Failed 3" answers it before
    it is clicked. A lens with nothing behind it is not drawn at all.
  - One row is one line. The error is the only field that will not fit, so it
    rides the name column truncated and lands in full in the inspector.
  - Selection is for one verb. Re-running a dozen tasks is the reason to select
    a dozen tasks; everything else the row does, it does on its own.
*/

/* ------------------------------------------------------------------ lenses */

type Lens =
  'all' | 'failed' | 'gated' | 'stale' | 'running' | 'ready' | 'blocked' | 'done' | 'cancelled'

const LENSES: { value: Lens; label: string; tone: Tone; match: (task: Task) => boolean }[] = [
  { value: 'failed', label: 'Failed', tone: 'danger', match: (t) => t.state === 'failed' },
  {
    value: 'gated',
    label: 'Gated',
    tone: 'warning',
    match: (t) => t.state === 'awaiting_approval',
  },
  { value: 'stale', label: 'Stale', tone: 'warning', match: (t) => t.stale },
  { value: 'running', label: 'Running', tone: 'accent', match: (t) => t.state === 'running' },
  { value: 'ready', label: 'Queued', tone: 'info', match: (t) => t.state === 'ready' },
  { value: 'blocked', label: 'Blocked', tone: 'neutral', match: (t) => t.state === 'blocked' },
  { value: 'done', label: 'Done', tone: 'success', match: (t) => t.state === 'succeeded' },
  {
    value: 'cancelled',
    label: 'Cancelled',
    tone: 'neutral',
    match: (t) => t.state === 'cancelled',
  },
]

type GroupBy = 'stage' | 'chapter' | 'none'
type SortKey = 'pipeline' | 'updated' | 'duration' | 'attempt'
interface Sort {
  key: SortKey
  dir: 'asc' | 'desc'
}

/* ------------------------------------------------------------- row geometry */

const GROUP_ROW = 28
const TASK_ROW = 32
/**
 * One step of the tree, applied to the name column alone. Indenting the whole
 * row would carry the state, the timing and the counts along with it, and a
 * table whose right-hand columns move per group is no longer a table.
 */
const INDENT = 16

type Row =
  | { type: 'group'; key: string; label: string; caption: string; tasks: Task[] }
  | { type: 'task'; key: string; task: Task; depth: number }

/**
 * A blueprint may only be re-run once it has failed or been cancelled: the whole
 * DAG below it was built from the chapters it produced, and that expansion is
 * one-way. Rejecting the outline is the way back.
 */
function rerunLock(task: Task, expanded: boolean): string | undefined {
  if (task.kind !== 'blueprint' || !expanded) return undefined
  if (task.state === 'failed' || task.state === 'cancelled') return undefined
  return 'The pipeline below is built from this outline. Reject the gate first, or start a new video.'
}

/* ------------------------------------------------------------------- table */

export function TaskTable({
  video,
  tasks,
  chapters,
  artifacts,
  loading,
}: {
  video: Video
  tasks: Task[]
  chapters: Chapter[]
  /** Every artifact the video has produced, so a row can show what it left. */
  artifacts: ViewerItem[]
  loading: boolean
}) {
  const queryClient = useQueryClient()
  const openViewer = useAssetViewer()

  const [query, setQuery] = useState('')
  const search = useDeferredValue(query).trim().toLowerCase()
  const [lens, setLens] = usePersisted<Lens>('video.tasks.lens', 'all')
  const [group, setGroup] = usePersisted<GroupBy>('video.tasks.group', 'stage')
  const [sort, setSort] = usePersisted<Sort>('video.tasks.sort', { key: 'pipeline', dir: 'asc' })
  const [folded, setFolded] = useState<ReadonlySet<string>>(() => new Set())
  const [cursor, setCursor] = useState<string | null>(null)
  const [inspecting, setInspecting] = useState<string | null>(null)
  const [rerunning, setRerunning] = useState<{ ids: string[]; what: string } | null>(null)

  const searchRef = useRef<HTMLInputElement>(null)
  const scrollRef = useRef<HTMLDivElement>(null)

  useHotkeys([
    {
      keys: '/',
      label: 'Filter the task list',
      group: 'Video',
      run: () => searchRef.current?.focus(),
    },
  ])

  /*
    One clock for the whole table. A running task's elapsed time has to tick, and
    two hundred rows each holding their own interval is two hundred timers for
    one second hand.
  */
  const [now, setNow] = useState(() => Date.now())
  const running = tasks.some((task) => task.state === 'running')
  useEffect(() => {
    if (!running) return
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [running])

  const chapterTitles = useMemo(() => {
    const byOrdinal = new Map<number, string>()
    for (const chapter of chapters) byOrdinal.set(chapter.ordinal, chapter.title)
    return byOrdinal
  }, [chapters])

  const artifactsByTask = useMemo(() => {
    const byTask = new Map<string, ViewerItem[]>()
    for (const item of artifacts) {
      if (!item.taskId) continue
      const list = byTask.get(item.taskId)
      if (list) list.push(item)
      else byTask.set(item.taskId, [item])
    }
    return byTask
  }, [artifacts])

  const counts = useMemo(() => {
    const byLens = new Map<Lens, number>()
    for (const entry of LENSES) byLens.set(entry.value, 0)
    for (const task of tasks) {
      for (const entry of LENSES) {
        if (entry.match(task)) byLens.set(entry.value, (byLens.get(entry.value) ?? 0) + 1)
      }
    }
    return byLens
  }, [tasks])

  // A lens that empties out while it is selected leaves a blank pane with no
  // explanation, so it falls back to "all" the moment its count reaches zero.
  const activeLens: Lens = lens !== 'all' && (counts.get(lens) ?? 0) > 0 ? lens : 'all'

  const filtered = useMemo(() => {
    const matcher = LENSES.find((entry) => entry.value === activeLens)?.match
    return tasks.filter((task) => {
      if (matcher && !matcher(task)) return false
      if (!search) return true
      const haystack = [
        taskLabel(task.kind),
        task.kind,
        task.pool,
        task.state,
        taskStateLabel(task.state),
        task.error ?? '',
        task.ordinal > 0 ? `chapter ${task.ordinal}` : 'video',
        chapterTitles.get(task.ordinal) ?? '',
        task.id,
      ]
        .join(' ')
        .toLowerCase()
      return haystack.includes(search)
    })
  }, [tasks, activeLens, search, chapterTitles])

  // The clock orders rows only while the sort is by duration; otherwise a
  // ticking second hand rebuilds every row for an unchanged answer.
  const sortNow = sort.key === 'duration' ? now : 0
  const rows = useMemo(
    () => buildRows(filtered, group, sort, chapterTitles, folded, sortNow),
    [filtered, group, sort, chapterTitles, folded, sortNow],
  )

  /** The task rows as drawn, which is what the cursor walks and shift-click spans. */
  const visible = useMemo(
    () => rows.flatMap((row) => (row.type === 'task' ? [row.task.id] : [])),
    [rows],
  )

  const byId = useMemo(() => new Map(tasks.map((task) => [task.id, task])), [tasks])
  const expanded = tasks.length > 1

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: (index) => (rows[index]?.type === 'group' ? GROUP_ROW : TASK_ROW),
    getItemKey: (index) => rows[index]?.key ?? index,
    overscan: 12,
  })

  /* --------------------------------------------------------------- actions */

  const invalidate = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: qk.videoTasks(video.id) })
    void queryClient.invalidateQueries({ queryKey: qk.video(video.ref) })
    void queryClient.invalidateQueries({ queryKey: ['videos'] })
  }, [queryClient, video.id, video.ref])

  const accept = useMutation({
    mutationFn: (ids: string[]) => api.acceptStale(video.ref, ids),
    onSuccess: invalidate,
  })

  const askRerun = useCallback(
    (ids: string[], what: string) => {
      if (ids.length > 0) setRerunning({ ids, what })
    },
    [setRerunning],
  )

  const rerunTask = useCallback(
    (task: Task) => {
      if (rerunLock(task, expanded)) return
      askRerun([task.id], describe(task))
    },
    [askRerun, expanded],
  )

  // Stable identities, so the memoised rows survive the second hand: the parent
  // re-renders once a second while anything is running.
  const openRow = useCallback((id: string) => {
    setCursor(id)
    setInspecting(id)
  }, [])
  const acceptOne = useCallback((id: string) => accept.mutate([id]), [accept])
  const viewArtifacts = useCallback(
    (id: string) => {
      const items = artifactsByTask.get(id)
      if (items?.length) openViewer(items, 0)
    },
    [artifactsByTask, openViewer],
  )

  /* -------------------------------------------------------------- keyboard */

  const moveCursor = useCallback(
    (delta: number) => {
      if (visible.length === 0) return
      const at = cursor ? visible.indexOf(cursor) : -1
      const next = at === -1 ? (delta > 0 ? 0 : visible.length - 1) : at + delta
      const id = visible[Math.min(visible.length - 1, Math.max(0, next))]
      if (!id) return
      setCursor(id)
      if (inspecting) setInspecting(id)
      const index = rows.findIndex((row) => row.type === 'task' && row.task.id === id)
      if (index >= 0) virtualizer.scrollToIndex(index, { align: 'auto' })
    },
    [cursor, inspecting, rows, virtualizer, visible],
  )

  const onKeyDown = useCallback(
    (event: KeyboardEvent<HTMLDivElement>) => {
      const task = cursor ? byId.get(cursor) : undefined
      switch (event.key) {
        case 'ArrowDown':
          event.preventDefault()
          moveCursor(1)
          break
        case 'ArrowUp':
          event.preventDefault()
          moveCursor(-1)
          break
        case 'Enter':
          if (!task) return
          event.preventDefault()
          setInspecting((prev) => (prev === task.id ? null : task.id))
          break
        case 'r':
          if (!task) return
          event.preventDefault()
          rerunTask(task)
          break
        case 'Escape':
          if (!inspecting) return
          event.preventDefault()
          setInspecting(null)
          break
        default:
          break
      }
    },
    [byId, cursor, inspecting, moveCursor, rerunTask],
  )

  /* ----------------------------------------------------------------- chrome */

  if (loading) {
    return (
      <div className="space-y-2 p-4">
        <Skeleton className="h-7 w-full" />
        {Array.from({ length: 10 }, (_, i) => (
          <Skeleton key={i} className="h-7 w-full" />
        ))}
      </div>
    )
  }
  if (tasks.length === 0) {
    return (
      <EmptyState
        icon={<ListTree />}
        title="No tasks yet"
        description="The DAG is built when the video starts. Nothing is enqueued until then."
      />
    )
  }

  const inspected = inspecting ? byId.get(inspecting) : undefined

  return (
    <div className="flex h-full min-h-0 flex-col">
      {/* ------------------------------------------------------------ filters */}
      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-[hsl(var(--border))] bg-subtle px-3 py-2 no-select">
        <SearchField
          value={query}
          onChange={setQuery}
          inputRef={searchRef}
          placeholder="Filter tasks, chapters, errors"
          keys="/"
          className="w-[220px]"
        />
        <Divider />
        <div className="flex min-w-0 flex-wrap items-center gap-1.5">
          <FilterChip
            label="All"
            count={tasks.length}
            selected={activeLens === 'all'}
            onClick={() => setLens('all')}
          />
          {LENSES.map((entry) => {
            const count = counts.get(entry.value) ?? 0
            if (count === 0) return null
            return (
              <FilterChip
                key={entry.value}
                label={entry.label}
                count={count}
                tone={entry.tone}
                selected={activeLens === entry.value}
                onClick={() => setLens(entry.value)}
              />
            )
          })}
        </div>

        <div className="ml-auto flex shrink-0 items-center gap-2">
          <Tooltip
            label={
              group === 'stage'
                ? 'Grouped by stage of the pipeline'
                : group === 'chapter'
                  ? 'Grouped by chapter'
                  : 'One flat list'
            }
          >
            {/* The span is the tooltip's trigger: `Segmented` is a plain
                component and cannot take the ref Radix would hand it. */}
            <span>
              <Segmented
                aria-label="Group tasks by"
                value={group}
                onChange={setGroup}
                options={[
                  { value: 'stage', label: <Layers className="h-3.5 w-3.5" /> },
                  { value: 'chapter', label: <ListTree className="h-3.5 w-3.5" /> },
                  { value: 'none', label: <Rows3 className="h-3.5 w-3.5" /> },
                ]}
                className="w-[102px]"
              />
            </span>
          </Tooltip>
        </div>
      </div>

      {accept.isError && <ErrorNotice error={accept.error} className="mx-3 mt-2" />}

      {/* ---------------------------------------------------------- the table */}
      <div className="flex min-h-0 flex-1">
        <div className="@container flex min-w-0 flex-1 flex-col">
          <div className="flex h-[26px] shrink-0 items-center gap-2 border-b border-[hsl(var(--border))] bg-subtle px-3 text-[10.5px] font-semibold uppercase tracking-wider text-subtle no-select">
            <span className="w-[7px] shrink-0" aria-hidden />
            <SortHeader sort={sort} onSort={setSort} column="pipeline" className="min-w-0 flex-1">
              Task
            </SortHeader>
            <span className="hidden w-[104px] shrink-0 @2xl:block">Chapter</span>
            <span className="hidden w-[52px] shrink-0 @3xl:block">Pool</span>
            <span className="w-[86px] shrink-0">State</span>
            <SortHeader
              sort={sort}
              onSort={setSort}
              column="attempt"
              className="hidden w-[40px] shrink-0 justify-end @xl:flex"
            >
              Try
            </SortHeader>
            <SortHeader
              sort={sort}
              onSort={setSort}
              column="duration"
              className="w-[58px] shrink-0 justify-end"
            >
              Time
            </SortHeader>
            <SortHeader
              sort={sort}
              onSort={setSort}
              column="updated"
              className="hidden w-[70px] shrink-0 justify-end @4xl:flex"
            >
              Updated
            </SortHeader>
            <span className="w-7 shrink-0" aria-hidden />
          </div>

          {rows.length === 0 ? (
            <EmptyState
              title="Nothing matches"
              description="No task matches the filter and the search together."
              action={
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    setQuery('')
                    setLens('all')
                  }}
                >
                  Clear the filters
                </Button>
              }
            />
          ) : (
            <div
              ref={scrollRef}
              role="grid"
              aria-label="Tasks"
              aria-rowcount={filtered.length}
              aria-activedescendant={cursor ? `task-row-${cursor}` : undefined}
              tabIndex={0}
              onKeyDown={onKeyDown}
              className="min-h-0 flex-1 overflow-y-auto outline-none"
            >
              <div className="relative w-full" style={{ height: virtualizer.getTotalSize() }}>
                {virtualizer.getVirtualItems().map((virtual) => {
                  const row = rows[virtual.index]
                  if (!row) return null
                  return (
                    <div
                      key={virtual.key}
                      className="absolute left-0 top-0 w-full"
                      style={{ height: virtual.size, transform: `translateY(${virtual.start}px)` }}
                    >
                      {row.type === 'group' ? (
                        <GroupHeader
                          label={row.label}
                          caption={row.caption}
                          tasks={row.tasks}
                          folded={folded.has(row.key)}
                          onFold={() =>
                            setFolded((prev) => {
                              const next = new Set(prev)
                              if (next.has(row.key)) next.delete(row.key)
                              else next.add(row.key)
                              return next
                            })
                          }
                        />
                      ) : (
                        <TaskRow
                          task={row.task}
                          chapterTitle={chapterTitles.get(row.task.ordinal)}
                          // A finished row's duration cannot change, so it is
                          // handed a frozen clock and the memo holds.
                          now={row.task.finishedAt ? 0 : now}
                          depth={row.depth}
                          cursored={cursor === row.task.id}
                          inspected={inspecting === row.task.id}
                          lock={rerunLock(row.task, expanded)}
                          produced={artifactsByTask.get(row.task.id)?.length ?? 0}
                          onOpen={openRow}
                          onRerun={rerunTask}
                          onAccept={acceptOne}
                          onView={viewArtifacts}
                        />
                      )}
                    </div>
                  )
                })}
              </div>
            </div>
          )}

          <div className="flex h-[24px] shrink-0 items-center gap-3 border-t border-[hsl(var(--border))] bg-subtle px-3 text-[11px] text-subtle no-select">
            <span className="tabular">
              {filtered.length === tasks.length
                ? `${tasks.length} tasks`
                : `${filtered.length} of ${tasks.length} tasks`}
            </span>
            <span className="hidden @2xl:inline">
              <Kbdish>↑</Kbdish> <Kbdish>↓</Kbdish> move · <Kbdish>↵</Kbdish> details ·{' '}
              <Kbdish>r</Kbdish> re-run
            </span>
            {video.counts.stale > 0 && (
              <span className="ml-auto flex items-center gap-1.5 text-[hsl(var(--warning))]">
                <StaleDot />
                {video.counts.stale} stale
              </span>
            )}
          </div>
        </div>

        {inspected && (
          <TaskInspector
            task={inspected}
            video={video}
            chapterTitle={chapterTitles.get(inspected.ordinal)}
            now={now}
            produced={artifactsByTask.get(inspected.id) ?? []}
            lock={rerunLock(inspected, expanded)}
            onClose={() => setInspecting(null)}
            onRerun={() => rerunTask(inspected)}
            onAccept={() => accept.mutate([inspected.id])}
            onView={(items) => openViewer(items, 0)}
          />
        )}
      </div>

      {rerunning && (
        <RerunDialog
          open
          onOpenChange={(open) => {
            if (!open) setRerunning(null)
          }}
          videoRef={video.ref}
          videoId={video.id}
          taskIds={rerunning.ids}
          what={rerunning.what}
        />
      )}
    </div>
  )
}

/** A keycap-ish inline hint for the footer, where a real `Kbd` would be too loud. */
function Kbdish({ children }: { children: ReactNode }) {
  return (
    <span className="rounded-[var(--radius-xs)] bg-[hsl(var(--bg-hover))] px-1 font-mono text-[10px] text-muted">
      {children}
    </span>
  )
}

/* ------------------------------------------------------------- row building */

function buildRows(
  tasks: Task[],
  group: GroupBy,
  sort: Sort,
  chapterTitles: Map<number, string>,
  folded: ReadonlySet<string>,
  now: number,
): Row[] {
  const compare = comparator(sort, now)

  // Flat is flat: with no header above them there is nothing for the rows to sit
  // under, and an indent that leads nowhere is just a wasted column.
  if (group === 'none') {
    return [...tasks]
      .sort(compare)
      .map((task) => ({ type: 'task', key: task.id, task, depth: 0 }) satisfies Row)
  }

  const groups = new Map<string, { label: string; order: number; tasks: Task[] }>()
  for (const task of tasks) {
    const key = group === 'stage' ? task.kind : String(task.ordinal)
    const entry = groups.get(key) ?? {
      label:
        group === 'stage'
          ? taskLabel(task.kind)
          : task.ordinal > 0
            ? `Chapter ${task.ordinal}${chapterTitles.has(task.ordinal) ? ` · ${chapterTitles.get(task.ordinal)}` : ''}`
            : 'Whole video',
      order: group === 'stage' ? taskKindRank(task.kind) : task.ordinal,
      tasks: [],
    }
    entry.tasks.push(task)
    groups.set(key, entry)
  }

  const rows: Row[] = []
  for (const [key, entry] of [...groups.entries()].sort((a, b) => a[1].order - b[1].order)) {
    const members = [...entry.tasks].sort(compare)
    rows.push({
      type: 'group',
      key: `g:${key}`,
      label: entry.label,
      caption: census(members),
      tasks: members,
    })
    if (folded.has(`g:${key}`)) continue
    for (const task of members) rows.push({ type: 'task', key: task.id, task, depth: 1 })
  }
  return rows
}

/** What a group amounts to, in the six words a header has room for. */
function census(tasks: Task[]): string {
  const done = tasks.filter((task) => task.state === 'succeeded').length
  const failed = tasks.filter((task) => task.state === 'failed').length
  const stale = tasks.filter((task) => task.stale).length
  const parts = [`${done}/${tasks.length}`]
  if (failed > 0) parts.push(`${failed} failed`)
  if (stale > 0) parts.push(`${stale} stale`)
  return parts.join(' · ')
}

function comparator(sort: Sort, now: number): (a: Task, b: Task) => number {
  const sign = sort.dir === 'asc' ? 1 : -1
  return (a, b) => {
    switch (sort.key) {
      case 'updated':
        return sign * a.updatedAt.localeCompare(b.updatedAt)
      case 'attempt':
        return sign * (a.attempt - b.attempt || pipelineOrder(a, b))
      case 'duration':
        return (
          sign * ((taskSeconds(a, now) ?? -1) - (taskSeconds(b, now) ?? -1) || pipelineOrder(a, b))
        )
      case 'pipeline':
      default:
        return sign * pipelineOrder(a, b)
    }
  }
}

function pipelineOrder(a: Task, b: Task): number {
  return taskKindRank(a.kind) - taskKindRank(b.kind) || a.ordinal - b.ordinal || a.index - b.index
}

/** How a task is named when it is spoken about — in a dialog, in a menu. */
function describe(task: Task): string {
  const parts = [taskLabel(task.kind).toLowerCase()]
  if (task.ordinal > 0) parts.push(`chapter ${task.ordinal}`)
  if (task.index >= 0) parts.push(`#${task.index + 1}`)
  return parts.join(' ')
}

/* -------------------------------------------------------------- the header */

function SortHeader({
  sort,
  onSort,
  column,
  className,
  children,
}: {
  sort: Sort
  onSort: (next: Sort) => void
  column: SortKey
  className?: string
  children: ReactNode
}) {
  const active = sort.key === column
  const Arrow = sort.dir === 'asc' ? ArrowUp : ArrowDown
  return (
    <button
      type="button"
      onClick={() =>
        onSort(
          active
            ? { key: column, dir: sort.dir === 'asc' ? 'desc' : 'asc' }
            : { key: column, dir: column === 'pipeline' ? 'asc' : 'desc' },
        )
      }
      aria-sort={active ? (sort.dir === 'asc' ? 'ascending' : 'descending') : 'none'}
      className={cn(
        'flex items-center gap-1 truncate text-left uppercase tracking-wider transition-colors hover:text-fg',
        active && 'text-fg',
        className,
      )}
    >
      <span className="truncate">{children}</span>
      {active && <Arrow className="h-2.5 w-2.5 shrink-0" aria-hidden />}
    </button>
  )
}

/* --------------------------------------------------------------- the rows */

/**
 * The state column, as text. Colour is spent only where it buys something: a
 * failure, a gate waiting on someone, work in flight. Everything else is the
 * quiet majority and reads as such.
 */
const STATE_TEXT: Record<TaskState, string> = {
  failed: 'text-[hsl(var(--danger))] font-medium',
  awaiting_approval: 'text-[hsl(var(--warning))] font-medium',
  running: 'text-[hsl(var(--accent))] font-medium',
  succeeded: 'text-muted',
  ready: 'text-muted',
  blocked: 'text-subtle',
  cancelled: 'text-subtle',
}

const GroupHeader = memo(function GroupHeader({
  label,
  caption,
  tasks,
  folded,
  onFold,
}: {
  label: string
  caption: string
  tasks: Task[]
  folded: boolean
  onFold: () => void
}) {
  const done = tasks.filter((task) => task.state === 'succeeded').length
  const failed = tasks.filter((task) => task.state === 'failed').length
  return (
    <div className="flex h-full items-center gap-2 border-b border-[hsl(var(--border))] bg-[hsl(var(--bg-panel))] px-3 no-select">
      <button
        type="button"
        onClick={onFold}
        aria-expanded={!folded}
        className="flex min-w-0 flex-1 items-center gap-1.5 text-left"
      >
        {folded ? (
          <ChevronRight className="h-3 w-3 shrink-0 text-subtle" />
        ) : (
          <ChevronDown className="h-3 w-3 shrink-0 text-subtle" />
        )}
        <span className="truncate text-[11.5px] font-semibold text-fg">{label}</span>
        <span className="tabular shrink-0 text-[10.5px] text-subtle">{caption}</span>
      </button>
      <Progress
        value={done}
        total={tasks.length}
        failed={failed}
        className="h-1 w-16 shrink-0"
        aria-label={`${label} progress`}
      />
    </div>
  )
})

const TaskRow = memo(function TaskRow({
  task,
  chapterTitle,
  now,
  depth,
  cursored,
  inspected,
  lock,
  produced,
  onOpen,
  onRerun,
  onAccept,
  onView,
}: {
  task: Task
  chapterTitle: string | undefined
  now: number
  /** How deep in the tree this row sits — 0 when the list is flat. */
  depth: number
  cursored: boolean
  inspected: boolean
  lock: string | undefined
  produced: number
  onOpen: (id: string) => void
  onRerun: (task: Task) => void
  onAccept: (id: string) => void
  onView: (id: string) => void
}) {
  const seconds = taskSeconds(task, now)
  return (
    <ContextMenu
      items={
        <>
          <ContextMenuLabel>{describe(task)}</ContextMenuLabel>
          <ContextMenuItem onSelect={() => onOpen(task.id)}>
            <Eye className="h-3.5 w-3.5" />
            Show the details
          </ContextMenuItem>
          {produced > 0 && (
            <ContextMenuItem onSelect={() => onView(task.id)}>
              <Eye className="h-3.5 w-3.5" />
              View {produced === 1 ? 'the artifact' : `${produced} artifacts`}
            </ContextMenuItem>
          )}
          {task.stale && (
            <ContextMenuItem onSelect={() => onAccept(task.id)}>
              <Check className="h-3.5 w-3.5" />
              Keep it — clear the stale flag
            </ContextMenuItem>
          )}
          {!lock && (
            <ContextMenuItem onSelect={() => onRerun(task)}>
              <RefreshCw className="h-3.5 w-3.5" />
              Re-run this step
            </ContextMenuItem>
          )}
          <ContextMenuSeparator />
          <ContextMenuItem onSelect={() => void navigator.clipboard.writeText(task.id)}>
            <Copy className="h-3.5 w-3.5" />
            Copy the task id
          </ContextMenuItem>
        </>
      }
    >
      <div
        id={`task-row-${task.id}`}
        role="row"
        aria-selected={inspected}
        className={cn(
          'group relative flex h-full items-center gap-2 border-b border-[hsl(var(--border))] px-3 text-[12px]',
          'transition-colors hover:bg-[hsl(var(--bg-hover))]',
          inspected && 'bg-[hsl(var(--bg-active))]',
          cursored && 'ring-1 ring-inset ring-[hsl(var(--accent))]',
        )}
      >
        {/* The rule that makes the indent read as a branch rather than as
            margin. Drawn per row: the list is virtualised, so no element spans a
            whole group to hang a border on. It lines up under the chevron of the
            header above it. */}
        {Array.from({ length: depth }, (_, level) => (
          <span
            key={level}
            aria-hidden
            className="absolute inset-y-0 w-px bg-[hsl(var(--border))]"
            style={{ left: 18 + level * INDENT }}
          />
        ))}
        {depth > 0 && <span className="shrink-0" style={{ width: depth * INDENT }} aria-hidden />}
        <TaskStateDot state={task.state} stale={task.stale} />

        <button
          type="button"
          onClick={() => onOpen(task.id)}
          className="flex min-w-0 flex-1 items-baseline gap-1.5 text-left"
        >
          <span className="shrink-0 font-medium text-fg">
            {taskLabel(task.kind)}
            {task.index >= 0 && <span className="text-subtle"> #{task.index + 1}</span>}
          </span>
          {task.gate && (
            <Badge tone="warning" className="shrink-0">
              gate
            </Badge>
          )}
          {/* The error is the only field that will not fit in a column, so it
              rides the name and lands in full in the inspector. */}
          {task.error ? (
            <span className="min-w-0 truncate text-[11.5px] text-[hsl(var(--danger))]">
              {task.error}
            </span>
          ) : (
            // A mark rather than prose: sixty rows reading "1 artifact" is a
            // column of noise for a fact the icon states.
            produced > 0 && (
              <span
                className="flex shrink-0 items-center gap-0.5 text-[10.5px] text-subtle"
                title={produced === 1 ? '1 artifact' : `${produced} artifacts`}
              >
                <Paperclip className="h-2.5 w-2.5" aria-hidden />
                {produced > 1 && <span className="tabular">{produced}</span>}
              </span>
            )
          )}
        </button>

        <span className="hidden w-[104px] shrink-0 truncate text-[11.5px] text-muted @2xl:block">
          {task.ordinal > 0 ? (
            <>
              <span className="tabular">{task.ordinal}</span>
              {chapterTitle && <span className="text-subtle"> · {chapterTitle}</span>}
            </>
          ) : (
            <span className="text-subtle">video</span>
          )}
        </span>
        <span className="hidden w-[52px] shrink-0 truncate text-[11.5px] text-muted @3xl:block">
          {poolLabel(task.pool)}
        </span>
        {/*
          A word, not a pill. The dot in the gutter already carries the colour,
          and eighty-nine badges down a page is a column of confetti to read one
          fact through — so only the states that want something from the operator
          keep their colour.
        */}
        <span className={cn('w-[86px] shrink-0 truncate text-[11.5px]', STATE_TEXT[task.state])}>
          {taskStateLabel(task.state)}
          {task.stale && <span className="text-[hsl(var(--warning))]"> · stale</span>}
        </span>
        {/* One attempt of one is the norm; printing it on every row makes the
            rows that were retried harder to spot, not easier. */}
        <span
          className={cn(
            'tabular hidden w-[40px] shrink-0 text-right text-[11.5px] @xl:block',
            task.attempt > 1 ? 'text-[hsl(var(--warning))]' : 'text-subtle',
          )}
        >
          {task.attempt > 1 ? `${task.attempt}/${task.maxAttempts}` : ''}
        </span>
        <span
          className={cn(
            'tabular w-[58px] shrink-0 text-right text-[11.5px]',
            task.state === 'running' ? 'text-[hsl(var(--accent))]' : 'text-muted',
          )}
        >
          {formatCompactDuration(seconds)}
        </span>
        <span className="tabular hidden w-[70px] shrink-0 text-right text-[11px] text-subtle @4xl:block">
          {formatRelative(task.updatedAt)}
        </span>

        <span className="flex w-7 shrink-0 justify-end">
          {lock ? (
            <Tooltip label={lock}>
              <span className="flex h-6 w-6 items-center justify-center text-subtle">
                <Lock className="h-3 w-3" aria-label="Locked" />
              </span>
            </Tooltip>
          ) : (
            <Tooltip label="Run this step again. Anything below it is flagged, not re-run.">
              <Button
                size="icon"
                variant="ghost"
                className="h-6 w-6 opacity-0 transition-opacity focus-visible:opacity-100 group-hover:opacity-100"
                onClick={() => onRerun(task)}
                aria-label={`Re-run ${describe(task)}`}
              >
                <RefreshCw className="h-3 w-3" />
              </Button>
            </Tooltip>
          )}
        </span>
      </div>
    </ContextMenu>
  )
})

/* ---------------------------------------------------------- the inspector */

/**
 * One task, in full. The table is deliberately one line per row, which leaves
 * three things homeless: the whole error text, the times, and the artifacts the
 * step left behind. They live here, beside the row rather than over it, so the
 * list keeps its place while an operator reads.
 */
function TaskInspector({
  task,
  video,
  chapterTitle,
  now,
  produced,
  lock,
  onClose,
  onRerun,
  onAccept,
  onView,
}: {
  task: Task
  video: Video
  chapterTitle: string | undefined
  now: number
  produced: ViewerItem[]
  lock: string | undefined
  onClose: () => void
  onRerun: () => void
  onAccept: () => void
  onView: (items: ViewerItem[]) => void
}) {
  const seconds = taskSeconds(task, now)
  return (
    <aside
      aria-label="Task details"
      className="flex w-[300px] shrink-0 flex-col overflow-y-auto border-l border-[hsl(var(--border))] bg-[hsl(var(--bg-panel))]"
    >
      <header className="flex items-start gap-2 border-b border-[hsl(var(--border))] px-3 py-2.5">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5">
            <h3 className="truncate text-[13px] font-semibold text-fg">{taskLabel(task.kind)}</h3>
            {task.index >= 0 && <span className="text-[12px] text-subtle">#{task.index + 1}</span>}
          </div>
          <p className="truncate text-[11.5px] text-subtle">
            {task.ordinal > 0
              ? `Chapter ${task.ordinal}${chapterTitle ? ` · ${chapterTitle}` : ''}`
              : video.ref}
          </p>
        </div>
        <Button size="icon" variant="ghost" onClick={onClose} aria-label="Close the details">
          <X className="h-3.5 w-3.5" />
        </Button>
      </header>

      <div className="flex flex-wrap items-center gap-1.5 border-b border-[hsl(var(--border))] px-3 py-2">
        <TaskStateBadge state={task.state} />
        {task.stale && (
          <Badge tone="warning" dot>
            stale
          </Badge>
        )}
        {task.gate && <Badge tone="warning">{task.gate} gate</Badge>}
        {task.state === 'blocked' && task.depsRemaining > 0 && (
          <Badge tone="neutral">waiting on {task.depsRemaining}</Badge>
        )}
      </div>

      {task.error && (
        <section className="border-b border-[hsl(var(--border))] px-3 py-2.5">
          <h4 className="mb-1.5 text-[11px] font-semibold uppercase tracking-wider text-subtle">
            Failure
          </h4>
          <p className="select-text whitespace-pre-wrap break-words rounded-[var(--radius-sm)] border border-[hsl(var(--danger)/0.35)] bg-[hsl(var(--danger-soft))] px-2 py-1.5 font-mono text-[11.5px] leading-relaxed text-[hsl(var(--danger))]">
            {task.error}
          </p>
        </section>
      )}

      {produced.length > 0 && (
        <section className="border-b border-[hsl(var(--border))] px-3 py-2.5">
          <div className="mb-1.5 flex items-center justify-between gap-2">
            <h4 className="text-[11px] font-semibold uppercase tracking-wider text-subtle">
              Produced
            </h4>
            <Button size="xs" variant="ghost" onClick={() => onView(produced)}>
              <Eye className="h-3 w-3" />
              View
            </Button>
          </div>
          <div className="grid grid-cols-4 gap-1.5">
            {produced.slice(0, 8).map((item, index) => (
              <button
                key={item.id + index}
                type="button"
                onClick={() => onView(produced.slice(index))}
                aria-label={`Preview ${item.title}`}
                className="aspect-square overflow-hidden rounded-[var(--radius-xs)] border border-[hsl(var(--border))] transition-colors hover:border-[hsl(var(--accent))]"
              >
                <AssetPreview item={item} />
              </button>
            ))}
          </div>
        </section>
      )}

      <section className="border-b border-[hsl(var(--border))] px-3 py-2">
        <dl>
          <KeyValue label="Pool">{poolLabel(task.pool)}</KeyValue>
          <KeyValue label="Attempt">
            {task.attempt} of {task.maxAttempts}
          </KeyValue>
          <KeyValue label="Duration">{formatCompactDuration(seconds)}</KeyValue>
          <KeyValue label="Started">{formatAbsolute(task.startedAt)}</KeyValue>
          <KeyValue label="Finished">{formatAbsolute(task.finishedAt)}</KeyValue>
          {task.notBefore && (
            <KeyValue label="Retry after">{formatAbsolute(task.notBefore)}</KeyValue>
          )}
          <KeyValue label="Updated">{formatRelative(task.updatedAt)}</KeyValue>
          <KeyValue label="Id">
            <Mono className="text-[11px]">{task.id.slice(0, 12)}</Mono>
          </KeyValue>
        </dl>
      </section>

      <div className="mt-auto flex flex-col gap-2 px-3 py-3">
        {task.stale && (
          <Button size="sm" variant="outline" onClick={onAccept}>
            <Check className="h-3.5 w-3.5" />
            Keep this artifact
          </Button>
        )}
        {lock ? (
          <p className="flex items-start gap-1.5 text-[11.5px] leading-relaxed text-subtle">
            <Lock className="mt-[2px] h-3 w-3 shrink-0" aria-hidden />
            {lock}
          </p>
        ) : (
          <>
            <Button size="sm" variant="primary" onClick={onRerun}>
              <RefreshCw className="h-3.5 w-3.5" />
              Re-run this step
            </Button>
            <p className="text-[11px] leading-relaxed text-subtle">
              Only this step runs. Anything below keeps its artifact and is flagged for you to
              decide on.
            </p>
          </>
        )}
      </div>
    </aside>
  )
}
