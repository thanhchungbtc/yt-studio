import { useQuery } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import { AlertTriangle, ChevronDown, ChevronRight, FoldVertical, Plus, Tv } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { useCommands } from './lib/keys'
import { useActiveTab, useWorkbenchStore, type Filter } from './lib/store'
import { IconButton } from './ui/controls'
import {
  EmptyState,
  FilterChip,
  PaneHeader,
  Ring,
  SearchField,
  Skeleton,
  Tooltip,
} from './ui/primitives'
import { VideoStateDot } from './ui/status'
import { api, qk } from '@/core/api'
import { formatRelative } from '@/core/format'
import type { Channel, Video, VideoState } from '@/core/types'
import { cn } from '@/core/utils'

const FILTERS: { value: Filter; label: string; match: (state: VideoState) => boolean }[] = [
  { value: 'all', label: 'All', match: () => true },
  { value: 'live', label: 'Live', match: (s) => s === 'running' },
  { value: 'gated', label: 'Gated', match: (s) => s === 'awaiting_approval' || s === 'blocked' },
  { value: 'done', label: 'Done', match: (s) => s === 'completed' },
  { value: 'failed', label: 'Failed', match: (s) => s === 'failed' },
]

const ROW = 26

interface Group {
  channel: Channel
  videos: Video[]
}

type Row =
  | { type: 'channel'; id: string; group: Group }
  | { type: 'video'; id: string; video: Video; channelId: string }

/**
 * The channel explorer: every channel, every video, one tree, always mounted.
 *
 * Single click opens a preview — the italic tab that the next single click
 * replaces. Double click, or Enter, pins. That pairing is what makes a tab strip
 * survivable: you can walk the whole channel with the arrow keys and finish with
 * one tab open rather than forty.
 */
export function Explorer({ onNewVideo }: { onNewVideo: (channelSlug?: string) => void }) {
  const open = useWorkbenchStore((s) => s.open)
  const filter = useWorkbenchStore((s) => s.filter)
  const setFilter = useWorkbenchStore((s) => s.setFilter)
  const folded = useWorkbenchStore((s) => s.folded)
  const toggleFold = useWorkbenchStore((s) => s.toggleFold)
  const setFolded = useWorkbenchStore((s) => s.setFolded)

  const activeTab = useActiveTab()
  const activeRef = activeTab?.doc.kind === 'video' ? activeTab.doc.ref : undefined
  const activeChannel = activeTab?.doc.kind === 'channel' ? activeTab.doc.slug : undefined

  const [query, setQuery] = useState('')
  const searchRef = useRef<HTMLInputElement>(null)
  const parent = useRef<HTMLDivElement>(null)

  const videos = useQuery({ queryKey: qk.videos({}), queryFn: () => api.listVideos({}) })
  const channels = useQuery({ queryKey: qk.channels, queryFn: api.listChannels })

  const groups = useMemo(
    () => groupByChannel(videos.data?.videos ?? [], channels.data ?? [], query, filter),
    [videos.data, channels.data, query, filter],
  )

  // While searching, a folded group would hide the very thing being looked for.
  const searching = query.trim().length > 0

  /** The tree as a flat list — what the virtualizer walks and what ↑/↓ steps. */
  const rows = useMemo<Row[]>(() => {
    const flat: Row[] = []
    for (const group of groups) {
      flat.push({ type: 'channel', id: `c:${group.channel.id}`, group })
      if (!searching && folded.includes(group.channel.id)) continue
      for (const video of group.videos) {
        flat.push({ type: 'video', id: video.id, video, channelId: group.channel.id })
      }
    }
    return flat
  }, [groups, folded, searching])

  const videoRows = useMemo(
    () => rows.filter((row): row is Extract<Row, { type: 'video' }> => row.type === 'video'),
    [rows],
  )

  const virtual = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parent.current,
    estimateSize: () => ROW,
    overscan: 14,
  })

  const step = useCallback(
    (delta: number) => {
      if (videoRows.length === 0) return
      const current = videoRows.findIndex((row) => row.video.ref === activeRef)
      // From nothing selected, ↓ lands on the first row and ↑ on the last.
      const next =
        current === -1
          ? delta > 0
            ? 0
            : videoRows.length - 1
          : (current + delta + videoRows.length) % videoRows.length
      const row = videoRows[next]
      // Stepping is browsing, so it reuses the preview tab rather than pinning
      // one document per keystroke.
      if (row) open({ kind: 'video', ref: row.video.ref }, { preview: true })
    },
    [activeRef, videoRows, open],
  )

  useCommands([
    {
      id: 'explorer.filter',
      label: 'Filter the explorer',
      category: 'Explorer',
      keys: '$mod+KeyF',
      whileTyping: true,
      run: () => searchRef.current?.select(),
    },
    {
      id: 'explorer.next',
      label: 'Next video',
      category: 'Explorer',
      keys: 'Alt+ArrowDown',
      whileTyping: true,
      run: () => step(1),
    },
    {
      id: 'explorer.previous',
      label: 'Previous video',
      category: 'Explorer',
      keys: 'Alt+ArrowUp',
      whileTyping: true,
      run: () => step(-1),
    },
  ])

  // Keep the selected row on screen when the selection moved from elsewhere —
  // the palette, a keyboard step, a restored session.
  //
  // Keyed on the selection alone. `rows` is a new array on every SSE delta, and
  // depending on it would yank the viewport back to the selected video each time
  // any task anywhere reported progress.
  const rowsRef = useRef(rows)
  rowsRef.current = rows
  useEffect(() => {
    if (!activeRef) return
    const index = rowsRef.current.findIndex(
      (row) => row.type === 'video' && row.video.ref === activeRef,
    )
    if (index >= 0) virtual.scrollToIndex(index, { align: 'auto' })
  }, [activeRef, virtual])

  const total = videos.data?.videos.length ?? 0
  const shown = groups.reduce((sum, group) => sum + group.videos.length, 0)
  const allFolded = groups.length > 0 && folded.length >= groups.length

  // Sticky scroll: whichever channel owns the topmost visible row is drawn as a
  // fixed header over the list, so the tree always says where you are.
  const first = virtual.getVirtualItems()[0]
  const topRow = first ? rows[first.index] : undefined
  const stuck =
    topRow?.type === 'video'
      ? groups.find((group) => group.channel.id === topRow.channelId)
      : undefined

  return (
    <div className="flex h-full min-h-0 flex-col bg-panel">
      <PaneHeader title="Explorer">
        <span className="tabular mr-1 text-[10.5px] text-subtle">
          {shown === total ? total : `${shown}/${total}`}
        </span>
        <Tooltip label={allFolded ? 'Unfold every channel' : 'Fold every channel'} side="bottom">
          <IconButton
            aria-label={allFolded ? 'Unfold every channel' : 'Fold every channel'}
            onClick={() => setFolded(allFolded ? [] : groups.map((g) => g.channel.id))}
          >
            <FoldVertical className="h-3.5 w-3.5" />
          </IconButton>
        </Tooltip>
        <Tooltip label="New video" keys="$mod+KeyN" side="bottom">
          <IconButton aria-label="New video" onClick={() => onNewVideo()}>
            <Plus className="h-4 w-4" />
          </IconButton>
        </Tooltip>
      </PaneHeader>

      <div className="shrink-0 px-2.5 pt-2.5">
        <SearchField
          value={query}
          onChange={setQuery}
          placeholder="Filter by title or ref"
          inputRef={searchRef}
          keys="$mod+KeyF"
        />
      </div>

      {/* Wraps rather than scrolls: a clipped chip reads as a missing filter. */}
      <div className="flex shrink-0 flex-wrap gap-1 px-2.5 py-2 no-select">
        {FILTERS.map((option) => (
          <FilterChip
            key={option.value}
            label={option.label}
            count={(videos.data?.videos ?? []).filter((v) => option.match(v.state)).length}
            selected={filter === option.value}
            onClick={() => setFilter(option.value)}
          />
        ))}
      </div>

      <div className="relative min-h-0 flex-1">
        {videos.isPending && (
          <div className="space-y-1 px-2.5 pt-1">
            {Array.from({ length: 9 }, (_, i) => (
              <Skeleton key={i} className="h-6 w-full" />
            ))}
          </div>
        )}

        {!videos.isPending && rows.length === 0 && (
          <EmptyState
            icon={<Tv />}
            title={searching || filter !== 'all' ? 'Nothing matches' : 'No videos yet'}
            description={
              searching || filter !== 'all'
                ? 'Clear the filter to see the rest.'
                : 'A channel carries the ref sequence every video inherits.'
            }
            className="px-3 py-8"
          />
        )}

        <div ref={parent} className="h-full overflow-y-auto overflow-x-hidden pb-3">
          <div className="relative w-full" style={{ height: virtual.getTotalSize() }}>
            {virtual.getVirtualItems().map((item) => {
              const row = rows[item.index]
              if (!row) return null
              return (
                <div
                  key={row.id}
                  className="absolute inset-x-0 top-0"
                  style={{ height: item.size, transform: `translateY(${item.start}px)` }}
                >
                  {row.type === 'channel' ? (
                    <ChannelRow
                      group={row.group}
                      folded={!searching && folded.includes(row.group.channel.id)}
                      selected={activeChannel === row.group.channel.slug}
                      onToggle={() => toggleFold(row.group.channel.id)}
                      onOpen={(pin) =>
                        open({ kind: 'channel', slug: row.group.channel.slug }, { preview: !pin })
                      }
                      onCreate={() => onNewVideo(row.group.channel.slug)}
                    />
                  ) : (
                    <VideoRow
                      video={row.video}
                      active={row.video.ref === activeRef}
                      onOpen={(pin) =>
                        open({ kind: 'video', ref: row.video.ref }, { preview: !pin })
                      }
                    />
                  )}
                </div>
              )
            })}
          </div>
        </div>

        {stuck && (
          <div className="pointer-events-none absolute inset-x-0 top-0 shadow-[0_1px_0_hsl(var(--border))]">
            <div className="pointer-events-auto">
              <ChannelRow
                group={stuck}
                folded={false}
                selected={activeChannel === stuck.channel.slug}
                onToggle={() => toggleFold(stuck.channel.id)}
                onOpen={(pin) =>
                  open({ kind: 'channel', slug: stuck.channel.slug }, { preview: !pin })
                }
                onCreate={() => onNewVideo(stuck.channel.slug)}
              />
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

/* ---------------------------------------------------------------- grouping */

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
    // An empty channel still gets a node: it is what you click to reach the
    // channel document, and hiding it would make a new channel unreachable from
    // the only navigation there is. Under a filter it drops out — there the
    // absence is the answer being asked for.
    if (!list && (needle || filter !== 'all')) continue
    const sorted = list ?? []
    // Most recently touched first: what is being worked on is at the top.
    sorted.sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
    groups.push({ channel, videos: sorted })
  }

  // A deleted channel can still own videos until they are purged.
  for (const [channelId, list] of buckets) {
    if (channels.some((c) => c.id === channelId)) continue
    groups.push({
      channel: { id: channelId, name: 'Unknown channel', slug: channelId } as Channel,
      videos: list,
    })
  }

  return groups.sort((a, b) => a.channel.name.localeCompare(b.channel.name))
}

/* -------------------------------------------------------------------- rows */

function ChannelRow({
  group,
  folded,
  selected,
  onToggle,
  onOpen,
  onCreate,
}: {
  group: Group
  folded: boolean
  selected: boolean
  onToggle: () => void
  onOpen: (pin: boolean) => void
  onCreate: () => void
}) {
  const { channel, videos } = group
  const live = videos.filter((v) => v.state === 'running').length
  const gated = videos.filter((v) => v.state === 'awaiting_approval').length
  const stale = videos.filter((v) => v.counts.stale > 0).length

  return (
    <div className="group/chan flex h-[26px] items-center gap-0.5 bg-panel pl-1 pr-1.5 no-select">
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={!folded}
        aria-label={folded ? `Unfold ${channel.name}` : `Fold ${channel.name}`}
        className="flex h-5 w-5 shrink-0 items-center justify-center rounded-[var(--radius-xs)] text-subtle hover:bg-[hsl(var(--bg-hover))] hover:text-fg"
      >
        {folded ? (
          <ChevronRight className="h-3 w-3" aria-hidden />
        ) : (
          <ChevronDown className="h-3 w-3" aria-hidden />
        )}
      </button>

      {/* The name is the document, not the fold: a channel is a thing you can
          open, and the chevron beside it is how you decline to. */}
      <button
        type="button"
        onClick={() => onOpen(false)}
        onDoubleClick={() => onOpen(true)}
        aria-current={selected ? 'page' : undefined}
        className={cn(
          'flex min-w-0 flex-1 items-center gap-1.5 rounded-[var(--radius-xs)] px-1 py-0.5 text-left transition-colors',
          selected ? 'bg-[hsl(var(--bg-active))]' : 'hover:bg-[hsl(var(--bg-hover))]',
        )}
      >
        <span
          className={cn(
            'truncate text-[10.5px] font-semibold uppercase tracking-[0.06em]',
            selected ? 'text-fg' : 'text-muted',
          )}
        >
          {channel.name}
        </span>
        <span className="tabular shrink-0 text-[10px] text-subtle">{videos.length}</span>
        {live > 0 && (
          <span
            className="h-1.5 w-1.5 shrink-0 rounded-full bg-[hsl(var(--accent))] pulse-live"
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
        <button
          type="button"
          aria-label={`New video in ${channel.name}`}
          onClick={onCreate}
          className="flex h-5 w-5 shrink-0 items-center justify-center rounded-[var(--radius-xs)] text-subtle opacity-0 transition-opacity hover:bg-[hsl(var(--bg-hover))] hover:text-fg focus-visible:opacity-100 group-hover/chan:opacity-100"
        >
          <Plus className="h-3 w-3" />
        </button>
      </Tooltip>
    </div>
  )
}

/**
 * One row, one line. The shell spent two lines per video on a state label the
 * dot already carries; a tree row that fits on one line is how twice as much of
 * the channel is on screen at once.
 */
function VideoRow({
  video,
  active,
  onOpen,
}: {
  video: Video
  active: boolean
  onOpen: (pin: boolean) => void
}) {
  const { succeeded, total, failed, stale } = video.counts

  return (
    <div className="px-1.5">
      <button
        type="button"
        onClick={() => onOpen(false)}
        onDoubleClick={() => onOpen(true)}
        onKeyDown={(event) => {
          // Enter means "I want to keep this", the same as a double click.
          if (event.key === 'Enter') {
            event.preventDefault()
            onOpen(true)
          }
        }}
        aria-current={active ? 'page' : undefined}
        title={video.title}
        className={cn(
          'group relative flex h-[26px] w-full items-center gap-2 rounded-[var(--radius-sm)] pl-5 pr-1.5 text-left outline-none transition-colors',
          active
            ? 'bg-[hsl(var(--bg-active))]'
            : 'hover:bg-[hsl(var(--bg-hover))] focus-visible:bg-[hsl(var(--bg-hover))]',
        )}
      >
        {active && (
          <span
            aria-hidden
            className="absolute inset-y-[3px] left-0 w-[2px] rounded-full bg-[hsl(var(--accent))]"
          />
        )}

        <Tooltip
          label={`${succeeded} of ${total} tasks${failed ? `, ${failed} failed` : ''}`}
          side="right"
        >
          <span className="flex h-4 w-4 shrink-0 items-center justify-center">
            {total > 0 ? (
              <Ring value={succeeded} total={total} failed={failed} size={14} />
            ) : (
              <VideoStateDot state={video.state} />
            )}
          </span>
        </Tooltip>

        <span
          className={cn(
            'shrink-0 font-mono text-[10px] font-semibold',
            active ? 'text-[hsl(var(--accent))]' : 'text-subtle',
          )}
        >
          {video.ref}
        </span>
        <span
          className={cn(
            'min-w-0 flex-1 truncate text-[12px]',
            active ? 'font-medium text-fg' : 'text-fg/90',
          )}
        >
          {video.title}
        </span>

        {stale > 0 && (
          <Tooltip label={`${stale} stale — an input changed after they ran`}>
            <span className="tabular shrink-0 rounded-full bg-[hsl(var(--warning)/0.18)] px-1.5 text-[10px] font-medium leading-[15px] text-[hsl(var(--warning))]">
              {stale}
            </span>
          </Tooltip>
        )}
        {failed > 0 && (
          <AlertTriangle
            className="h-3.5 w-3.5 shrink-0 text-[hsl(var(--danger))]"
            aria-label={`${failed} failed tasks`}
          />
        )}
        {/* The timestamp yields the moment a marker needs the room. */}
        <span className="tabular hidden shrink-0 text-[10px] text-subtle group-hover:inline">
          {formatRelative(video.updatedAt)}
        </span>
      </button>
    </div>
  )
}
