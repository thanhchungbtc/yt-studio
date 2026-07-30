import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import {
  AlertTriangle,
  ChevronDown,
  ChevronRight,
  Film,
  ListFilter,
  Plus,
  Search,
  Trash2,
  X,
} from 'lucide-react'
import { memo, useCallback, useLayoutEffect, useMemo, useRef, useState } from 'react'

import { VideoStateDot } from '@/components/state-badges'
import { Button } from '@/components/ui/button'
import { ContextMenu, ContextMenuItem, ContextMenuLabel } from '@/components/ui/menu'
import { ErrorNotice, Kbd, Modal, Ring, Skeleton, Tooltip } from '@/components/ui/primitives'
import { api, qk } from '@/lib/api'
import { useAppCommands } from '@/lib/app-commands'
import { formatRelative, videoStateLabel } from '@/lib/format'
import { useHotkeys } from '@/lib/hotkeys'
import type { Channel, Video, VideoState } from '@/lib/types'
import { cn } from '@/lib/utils'
import { usePersisted } from '@/lib/workspace'

/* ---------------------------------------------------------------- filters */

type Filter = 'all' | 'live' | 'gated' | 'done' | 'failed'

const FILTERS: { value: Filter; label: string; match: (state: VideoState) => boolean }[] = [
  { value: 'all', label: 'All', match: () => true },
  { value: 'live', label: 'Live', match: (s) => s === 'running' },
  { value: 'gated', label: 'Gated', match: (s) => s === 'awaiting_approval' || s === 'blocked' },
  { value: 'done', label: 'Done', match: (s) => s === 'completed' },
  { value: 'failed', label: 'Failed', match: (s) => s === 'failed' },
]

/* ------------------------------------------------------------------ pane */

/**
 * The sidebar: every channel, every video, always present.
 *
 * Mounted by the `/videos` layout route rather than by either child, so moving
 * between two videos swaps only the detail pane and the list keeps its scroll
 * position, its filter and its focus.
 */
export function VideoSidebar({ activeRef }: { activeRef?: string }) {
  const navigate = useNavigate()
  const { openCreateVideo } = useAppCommands()

  // The text filter is transient — a reload should not leave the list quietly
  // hiding things. The state filter is a stance, so it persists.
  const [query, setQuery] = useState('')
  const [filter, setFilter] = usePersisted<Filter>('sidebar.filter', 'all')
  const [collapsed, setCollapsed] = usePersisted<string[]>('sidebar.collapsedChannels', [])

  const searchRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)

  // The confirmation lives here rather than in the row, so the dialog survives
  // the row unmounting the moment the delete succeeds.
  const [pendingDelete, setPendingDelete] = useState<Video | null>(null)
  const requestDelete = useCallback((video: Video) => setPendingDelete(video), [])

  const videos = useQuery({ queryKey: qk.videos({}), queryFn: () => api.listVideos({}) })
  const channels = useQuery({ queryKey: qk.channels, queryFn: api.listChannels })

  const groups = useMemo(
    () => groupByChannel(videos.data?.videos ?? [], channels.data ?? [], query, filter),
    [videos.data, channels.data, query, filter],
  )

  // While searching, a folded group would hide the very thing being looked for.
  const searching = query.trim().length > 0
  const isCollapsed = useCallback(
    (id: string) => !searching && collapsed.includes(id),
    [collapsed, searching],
  )

  const toggleGroup = useCallback(
    (id: string) =>
      setCollapsed((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id])),
    [setCollapsed],
  )

  /** The rows the arrow keys walk: the visible videos, in drawn order. */
  const flat = useMemo(
    () => groups.filter((g) => !isCollapsed(g.channel.id)).flatMap((g) => g.videos),
    [groups, isCollapsed],
  )

  const step = useCallback(
    (delta: number) => {
      if (flat.length === 0) return
      const current = flat.findIndex((v) => v.ref === activeRef)
      // From nothing selected, ↓ lands on the first row and ↑ on the last.
      const next =
        current === -1
          ? delta > 0
            ? 0
            : flat.length - 1
          : (current + delta + flat.length) % flat.length
      const video = flat[next]
      if (video) void navigate({ to: '/videos/$ref', params: { ref: video.ref } })
    },
    [activeRef, flat, navigate],
  )

  useHotkeys([
    {
      keys: 'alt+arrowdown',
      label: 'Next video',
      group: 'Sidebar',
      whileTyping: true,
      run: () => step(1),
    },
    {
      keys: 'alt+arrowup',
      label: 'Previous video',
      group: 'Sidebar',
      whileTyping: true,
      run: () => step(-1),
    },
    {
      keys: 'mod+f',
      label: 'Filter the sidebar',
      group: 'Sidebar',
      whileTyping: true,
      run: () => searchRef.current?.select(),
    },
  ])

  // Keep the selected row visible when the selection changed from elsewhere —
  // the palette, a keyboard step, a deep link.
  useLayoutEffect(() => {
    if (!activeRef) return
    listRef.current
      ?.querySelector(`[data-ref="${CSS.escape(activeRef)}"]`)
      ?.scrollIntoView({ block: 'nearest' })
  }, [activeRef, groups])

  const total = videos.data?.videos.length ?? 0
  const shown = groups.reduce((sum, g) => sum + g.videos.length, 0)

  return (
    <div className="flex h-full min-h-0 flex-col bg-panel">
      {/* Title row — the pane's identity, matching the detail pane's toolbar. */}
      <div className="flex h-11 shrink-0 items-center gap-2 border-b border-[hsl(var(--border))] px-3 no-select">
        <h2 className="text-[11px] font-semibold uppercase tracking-wider text-subtle">Videos</h2>
        <span className="tabular rounded-full bg-[hsl(var(--fg)/0.08)] px-1.5 text-[10.5px] leading-[16px] text-subtle">
          {shown === total ? total : `${shown}/${total}`}
        </span>
        <div className="ml-auto flex items-center gap-0.5">
          <Tooltip label="Fold every channel" side="bottom">
            <Button
              size="icon"
              variant="ghost"
              aria-label="Fold every channel"
              onClick={() =>
                setCollapsed((prev) =>
                  prev.length === groups.length ? [] : groups.map((g) => g.channel.id),
                )
              }
            >
              <ListFilter className="h-3.5 w-3.5" />
            </Button>
          </Tooltip>
          <Tooltip label="New video" keys="mod+n" side="bottom">
            <Button
              size="icon"
              variant="ghost"
              aria-label="New video"
              onClick={() => openCreateVideo()}
            >
              <Plus className="h-4 w-4" />
            </Button>
          </Tooltip>
        </div>
      </div>

      {/* Search */}
      <div className="shrink-0 px-2.5 pt-2.5">
        <div className="relative">
          <Search
            className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-subtle"
            aria-hidden
          />
          <input
            ref={searchRef}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Escape' && query) {
                setQuery('')
                e.stopPropagation()
              } else if (e.key === 'ArrowDown') {
                e.preventDefault()
                const first = flat[0]
                if (first) void navigate({ to: '/videos/$ref', params: { ref: first.ref } })
              }
            }}
            placeholder="Filter by title or ref"
            aria-label="Filter videos"
            className="h-7 w-full rounded-[var(--radius-sm)] border border-[hsl(var(--border))] bg-[hsl(var(--bg))] pl-7 pr-7 text-[12px] text-fg outline-none transition-colors placeholder:text-subtle focus:border-[hsl(var(--accent))]"
          />
          {query ? (
            <button
              type="button"
              onClick={() => setQuery('')}
              aria-label="Clear the filter"
              className="absolute right-1.5 top-1/2 -translate-y-1/2 rounded-[var(--radius-xs)] p-0.5 text-subtle hover:bg-[hsl(var(--bg-hover))] hover:text-fg"
            >
              <X className="h-3 w-3" />
            </button>
          ) : (
            <Kbd keys="mod+f" className="absolute right-1.5 top-1/2 -translate-y-1/2 opacity-70" />
          )}
        </div>
      </div>

      {/* State filter */}
      {/* Wraps rather than scrolls: a clipped chip reads as a missing filter,
          and the sidebar is narrow by design. */}
      <div className="flex shrink-0 flex-wrap gap-1 px-2.5 py-2 no-select">
        {FILTERS.map((option) => {
          const count = (videos.data?.videos ?? []).filter((v) => option.match(v.state)).length
          return (
            <button
              key={option.value}
              type="button"
              onClick={() => setFilter(option.value)}
              aria-pressed={filter === option.value}
              className={cn(
                'flex shrink-0 items-center gap-1.5 rounded-full border px-2 py-[1px] text-[11px] transition-colors',
                filter === option.value
                  ? 'border-[hsl(var(--accent)/0.4)] bg-[hsl(var(--accent)/0.15)] font-medium text-[hsl(var(--accent))]'
                  : 'border-transparent text-muted hover:bg-[hsl(var(--bg-hover))] hover:text-fg',
              )}
            >
              {option.label}
              <span className="tabular text-[10px] opacity-70">{count}</span>
            </button>
          )
        })}
      </div>

      {/* The tree */}
      <div ref={listRef} className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden pb-3">
        {videos.isPending && (
          <div className="space-y-1.5 px-2.5 pt-1">
            {Array.from({ length: 7 }, (_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
        )}

        {!videos.isPending && shown === 0 && (
          <div className="px-4 py-10 text-center">
            <Film className="mx-auto mb-2 h-5 w-5 text-subtle" aria-hidden />
            <p className="text-[12px] text-muted">
              {searching || filter !== 'all' ? 'Nothing matches.' : 'No videos yet.'}
            </p>
            {(searching || filter !== 'all') && (
              <button
                type="button"
                onClick={() => {
                  setQuery('')
                  setFilter('all')
                }}
                className="mt-1.5 text-[11.5px] text-[hsl(var(--accent))] hover:underline"
              >
                Clear the filter
              </button>
            )}
          </div>
        )}

        {groups.map((group) => (
          <ChannelGroup
            key={group.channel.id}
            channel={group.channel}
            videos={group.videos}
            collapsed={isCollapsed(group.channel.id)}
            activeRef={activeRef}
            onToggle={() => toggleGroup(group.channel.id)}
            onCreate={() => openCreateVideo(group.channel.slug)}
            onRequestDelete={requestDelete}
          />
        ))}
      </div>

      {/* Keyed on the target, so a failed attempt on one video never greets the
          next one with the previous error still on screen. */}
      {pendingDelete && (
        <DeleteVideoDialog
          key={pendingDelete.ref}
          video={pendingDelete}
          active={pendingDelete.ref === activeRef}
          onClose={() => setPendingDelete(null)}
        />
      )}
    </div>
  )
}

/* ---------------------------------------------------------------- deletion */

/**
 * Deleting a video is irreversible and takes its chapters and its whole task
 * graph with it, so it asks first and says exactly what it is about to remove.
 */
function DeleteVideoDialog({
  video,
  active,
  onClose,
}: {
  video: Video
  active: boolean
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  const remove = useMutation({
    mutationFn: () => api.deleteVideo(video.ref),
    onSuccess: () => {
      // Leave first, drop the cache second. The detail pane is looking at a video
      // that no longer exists, and pulling its data out from under it while it is
      // still mounted only buys a refetch that 404s.
      if (active) void navigate({ to: '/videos' })
      onClose()

      // Both keys: the pane reads the video by ref and its chapters, tasks and
      // artifacts by id, and the event stream writes to both.
      queryClient.removeQueries({ queryKey: qk.video(video.ref) })
      queryClient.removeQueries({ queryKey: qk.video(video.id) })
      void queryClient.invalidateQueries({ queryKey: qk.videos({}) })
      void queryClient.invalidateQueries({ queryKey: qk.channels })
      // The console's table can still be holding this video's tasks.
      void queryClient.invalidateQueries({ queryKey: qk.recentTasks })
    },
  })

  const running = video.state === 'running' || video.state === 'awaiting_approval'

  return (
    <Modal
      open
      onOpenChange={(next) => {
        if (!next && !remove.isPending) onClose()
      }}
      title={`Delete ${video.ref}?`}
      description="The video, its chapters and its task graph are removed. This cannot be undone."
      footer={
        <>
          <Button variant="ghost" disabled={remove.isPending} onClick={onClose}>
            Cancel
          </Button>
          <Button variant="danger" disabled={remove.isPending} onClick={() => remove.mutate()}>
            {remove.isPending ? 'Deleting…' : 'Delete'}
          </Button>
        </>
      }
    >
      <div className="space-y-2 text-[12.5px] text-muted">
        <p className="text-fg">{video.title}</p>
        {running && (
          <p className="text-[hsl(var(--warning))]">
            This video is still working. Deleting it stops its tasks first.
          </p>
        )}
        <p>
          Its generated files are deleted from the store too — apart from any another video also
          uses, which stay.
        </p>
        {remove.isError && <ErrorNotice error={remove.error} />}
      </div>
    </Modal>
  )
}

/* ---------------------------------------------------------------- groups */

interface Group {
  channel: Channel
  videos: Video[]
}

function groupByChannel(
  videos: Video[],
  channels: Channel[],
  query: string,
  filter: Filter,
): Group[] {
  const needle = query.trim().toLowerCase()
  const match = FILTERS.find((f) => f.value === filter)?.match ?? (() => true)

  const buckets = new Map<string, Video[]>()
  for (const video of videos) {
    if (!match(video.state)) continue
    if (
      needle &&
      !video.title.toLowerCase().includes(needle) &&
      !video.ref.toLowerCase().includes(needle) &&
      !video.topic.toLowerCase().includes(needle)
    ) {
      continue
    }
    const list = buckets.get(video.channelId)
    if (list) list.push(video)
    else buckets.set(video.channelId, [video])
  }

  const groups: Group[] = []
  for (const channel of channels) {
    const list = buckets.get(channel.id)
    if (!list) continue
    // Most recently touched first: the thing you are working on is at the top.
    list.sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
    groups.push({ channel, videos: list })
  }

  // A channel that has been deleted can still own videos until they are purged.
  for (const [channelId, list] of buckets) {
    if (channels.some((c) => c.id === channelId)) continue
    groups.push({
      channel: { id: channelId, name: 'Unknown channel', slug: channelId } as Channel,
      videos: list,
    })
  }

  return groups.sort((a, b) => a.channel.name.localeCompare(b.channel.name))
}

const ChannelGroup = memo(function ChannelGroup({
  channel,
  videos,
  collapsed,
  activeRef,
  onToggle,
  onCreate,
  onRequestDelete,
}: {
  channel: Channel
  videos: Video[]
  collapsed: boolean
  activeRef: string | undefined
  onToggle: () => void
  onCreate: () => void
  onRequestDelete: (video: Video) => void
}) {
  const live = videos.filter((v) => v.state === 'running').length
  const gated = videos.filter((v) => v.state === 'awaiting_approval').length
  const stale = videos.filter((v) => v.counts.stale > 0).length

  return (
    <section>
      <div
        className={cn(
          'group/chan sticky top-0 z-10 flex h-7 items-center gap-1 bg-panel px-1.5 no-select',
          // A hairline under a sticky header, so rows scroll *under* it visibly.
          'shadow-[0_1px_0_hsl(var(--border))]',
        )}
      >
        <button
          type="button"
          onClick={onToggle}
          aria-expanded={!collapsed}
          className="flex min-w-0 flex-1 items-center gap-1 rounded-[var(--radius-xs)] px-1 py-0.5 text-left transition-colors hover:bg-[hsl(var(--bg-hover))]"
        >
          {collapsed ? (
            <ChevronRight className="h-3 w-3 shrink-0 text-subtle" aria-hidden />
          ) : (
            <ChevronDown className="h-3 w-3 shrink-0 text-subtle" aria-hidden />
          )}
          <span className="truncate text-[11px] font-semibold uppercase tracking-wide text-muted">
            {channel.name}
          </span>
          <span className="tabular shrink-0 text-[10.5px] text-subtle">{videos.length}</span>
          {live > 0 && (
            <span
              className="ml-0.5 h-1.5 w-1.5 shrink-0 rounded-full bg-[hsl(var(--accent))] pulse-live"
              title={`${live} running`}
            />
          )}
          {(gated > 0 || stale > 0) && (
            <span
              className="h-1.5 w-1.5 shrink-0 rounded-full bg-[hsl(var(--warning))]"
              title={[
                gated > 0 ? `${gated} awaiting approval` : '',
                stale > 0 ? `${stale} with stale work` : '',
              ]
                .filter(Boolean)
                .join(', ')}
            />
          )}
        </button>
        <Tooltip label={`New video in ${channel.name}`} side="right">
          <Button
            size="icon"
            variant="ghost"
            className="h-5 w-5 opacity-0 transition-opacity group-hover/chan:opacity-100 focus-visible:opacity-100"
            aria-label={`New video in ${channel.name}`}
            onClick={onCreate}
          >
            <Plus className="h-3 w-3" />
          </Button>
        </Tooltip>
      </div>

      {!collapsed && (
        <ul className="px-1.5 pt-1">
          {videos.map((video) => (
            <VideoRow
              key={video.id}
              video={video}
              active={video.ref === activeRef}
              onRequestDelete={onRequestDelete}
            />
          ))}
        </ul>
      )}
    </section>
  )
})

/** Memoised on the video object: an SSE delta re-renders exactly one row. */
const VideoRow = memo(function VideoRow({
  video,
  active,
  onRequestDelete,
}: {
  video: Video
  active: boolean
  onRequestDelete: (video: Video) => void
}) {
  const running = video.state === 'running'
  const { succeeded, total, failed } = video.counts

  return (
    <li>
      <ContextMenu
        items={
          <>
            <ContextMenuLabel>{video.ref}</ContextMenuLabel>
            <ContextMenuItem tone="danger" onSelect={() => onRequestDelete(video)}>
              <Trash2 className="h-3.5 w-3.5" aria-hidden />
              Delete video…
            </ContextMenuItem>
          </>
        }
      >
        <Link
          to="/videos/$ref"
          params={{ ref: video.ref }}
          data-ref={video.ref}
          aria-current={active ? 'page' : undefined}
          className={cn(
            'group relative flex items-center gap-2.5 rounded-[var(--radius-sm)] px-2 py-1.5 outline-none transition-colors',
            active
              ? 'bg-[hsl(var(--bg-active))]'
              : 'hover:bg-[hsl(var(--bg-hover))] focus-visible:bg-[hsl(var(--bg-hover))]',
          )}
        >
          {active && (
            <span
              aria-hidden
              className="absolute inset-y-1 left-0 w-[2px] rounded-full bg-[hsl(var(--accent))]"
            />
          )}

          <Tooltip
            label={`${succeeded} of ${total} tasks${failed ? `, ${failed} failed` : ''}`}
            side="right"
          >
            <span className="relative flex h-4 w-4 shrink-0 items-center justify-center">
              {total > 0 ? (
                <Ring value={succeeded} total={total} failed={failed} size={16} />
              ) : (
                <VideoStateDot state={video.state} />
              )}
            </span>
          </Tooltip>

          <span className="min-w-0 flex-1">
            <span className="flex items-baseline gap-1.5">
              <span
                className={cn(
                  'shrink-0 font-mono text-[10.5px] font-semibold',
                  active ? 'text-[hsl(var(--accent))]' : 'text-subtle',
                )}
              >
                {video.ref}
              </span>
              <span
                className={cn(
                  'truncate text-[12.5px]',
                  active ? 'font-medium text-fg' : 'text-fg/90',
                )}
              >
                {video.title}
              </span>
            </span>
            <span className="mt-[1px] flex items-center gap-1.5 text-[10.5px] text-subtle">
              <VideoStateDot state={video.state} className="h-[5px] w-[5px]" />
              <span className="truncate">{videoStateLabel(video.state)}</span>
              {running && total > 0 && (
                <span className="tabular shrink-0">
                  {succeeded}/{total}
                </span>
              )}
              <span className="ml-auto shrink-0 tabular">{formatRelative(video.updatedAt)}</span>
            </span>
          </span>

          {video.counts.stale > 0 && (
            <Tooltip label={`${video.counts.stale} stale — an input changed after they ran`}>
              <span className="tabular shrink-0 rounded-full bg-[hsl(var(--warning)/0.18)] px-1.5 text-[10px] font-medium leading-[16px] text-[hsl(var(--warning))]">
                {video.counts.stale}
              </span>
            </Tooltip>
          )}
          {failed > 0 && (
            <AlertTriangle
              className="h-3.5 w-3.5 shrink-0 text-[hsl(var(--danger))]"
              aria-label={`${failed} failed tasks`}
            />
          )}
        </Link>
      </ContextMenu>
    </li>
  )
})
