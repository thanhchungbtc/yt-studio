import { useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query'
import {
  ChevronRight,
  Clapperboard,
  ClipboardPaste,
  Copy,
  FolderOpen,
  Pencil,
  Play,
  Plus,
  Search,
  Square,
  SquarePen,
  Trash2,
  Tv,
  type LucideIcon,
} from 'lucide-react'
import {
  memo,
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type MouseEvent,
} from 'react'
import { toast } from 'sonner'

import { executeCommand } from '@/kit/commands/registry'
import { cn } from '@/kit/lib/cn'
import { IconButton } from '@/kit/ui/Button'
import { ContextMenu, DropdownMenu, type MenuEntry } from '@/kit/ui/Menu'
import { EmptyState, Spinner, TextInput } from '@/kit/ui/misc'
import { SlidingThumb } from '@/kit/ui/SlidingThumb'
import { Tooltip } from '@/kit/ui/Tooltip'
import { confirm } from '@/kit/workbench/confirm'

import { api, prefetchVideo, qk } from '../../core/api'
import { listTimestamp, stateLabel } from '../../core/format'
import type { Channel, Video, VideoState } from '../../core/types'
import { useWorkbench, type SidebarScope } from '../../store/workbench'
import { duplicateLabel, duplicateVideos } from '../duplicate-video'
import { editVideo } from '../edit-video'
import { docId, openDoc, pinPreview, useDock } from '../editor/dock'
import { newFromBlueprint } from '../new-from-blueprint'
import { newVideo } from '../new-video'
import { Avatar, avatarColor } from '../ui/avatar'

const SCOPES: Array<{ value: SidebarScope; title: string; icon: LucideIcon; command: string }> = [
  { value: 'videos', title: 'Videos', icon: Clapperboard, command: 'workbench.showVideos' },
  { value: 'channels', title: 'Channels', icon: Tv, command: 'workbench.showChannels' },
]

interface Group {
  channel: Channel
  videos: Video[]
}

const message = (error: unknown) => (error instanceof Error ? error.message : String(error))
const failed = (what: string) => (error: unknown) =>
  toast.error(what, { description: message(error) })

const titleOf = (video: Video) => video.title || video.ref

async function removeVideos(
  targets: Video[],
  client: QueryClient,
  deselect: (gone: string[]) => void,
) {
  const one = targets.length === 1
  const ok = await confirm({
    title: one ? `Delete “${titleOf(targets[0]!)}”?` : `Delete ${targets.length} videos?`,
    message:
      'Their chapters, their tasks and every file nothing else is using go with them. This cannot be undone.',
    confirmLabel: 'Delete',
    danger: true,
  })
  if (!ok) return
  const settled = await Promise.allSettled(targets.map((v) => api.deleteVideo(v.ref)))
  const gone = targets.filter((_, i) => settled[i]!.status === 'fulfilled').map((v) => v.ref)
  const dock = useDock.getState().api
  for (const ref of gone) dock?.getPanel(docId({ kind: 'video', ref }))?.api.close()
  deselect(gone)
  void client.invalidateQueries({ queryKey: qk.videos })
  const rejected = settled.filter((r): r is PromiseRejectedResult => r.status === 'rejected')
  if (rejected.length) {
    failed(`${rejected.length} of ${targets.length} couldn't be deleted`)(rejected[0]!.reason)
  } else {
    toast.success(one ? `Deleted “${titleOf(targets[0]!)}”` : `Deleted ${targets.length} videos`)
  }
}

function videoMenu(
  video: Video,
  channel: Channel,
  many: Video[],
  remove: (targets: Video[]) => void,
): MenuEntry[] {
  const startable = ['draft', 'failed', 'cancelled', 'blocked'].includes(video.state)
  const stoppable = video.state === 'running' || video.state === 'awaiting_approval'
  const entries: MenuEntry[] = [
    {
      label: 'Open',
      icon: FolderOpen,
      onSelect: () =>
        openDoc({ kind: 'video', ref: video.ref }, video.title || 'Untitled', {
          seed: channel.slug,
          initial: channel.name,
        }),
    },
  ]
  if (startable) {
    entries.push({
      label: video.state === 'draft' ? 'Start' : 'Resume',
      icon: Play,
      onSelect: () =>
        void api
          .startVideo(video.ref)
          .then(() => toast(`Started “${titleOf(video)}”`))
          .catch(failed("Couldn't start the video")),
    })
  }
  if (stoppable) {
    entries.push({
      label: 'Cancel Run',
      icon: Square,
      onSelect: () =>
        void api
          .cancelVideo(video.ref)
          .then(() => toast(`Cancelled “${titleOf(video)}”`))
          .catch(failed("Couldn't cancel the video")),
    })
  }
  entries.push(
    { type: 'separator' },
    { label: 'Edit Brief…', icon: Pencil, onSelect: () => editVideo(video) },
    {
      label: duplicateLabel(many.length),
      icon: Copy,
      shortcut: '$mod+d',
      onSelect: () => duplicateVideos(many),
    },
    { type: 'separator' },
    {
      label: many.length > 1 ? `Delete ${many.length} Videos…` : 'Delete…',
      icon: Trash2,
      danger: true,
      onSelect: () => remove(many),
    },
  )
  return entries
}

export function PrimarySidebar() {
  const scope = useWorkbench((s) => s.scope)
  const setScope = useWorkbench((s) => s.setScope)
  const selected = useWorkbench((s) => s.selected)
  const select = useWorkbench((s) => s.select)

  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set())
  const [anchor, setAnchor] = useState<string | null>(null)
  const [filter, setFilter] = useState('')

  const client = useQueryClient()
  const channels = useQuery({ queryKey: qk.channels, queryFn: api.listChannels })
  const videos = useQuery({ queryKey: qk.videos, queryFn: api.listVideos })

  const needle = filter.trim().toLowerCase()
  const groups = useMemo<Group[]>(() => {
    const byId = new Map((channels.data ?? []).map((channel) => [channel.id, channel]))
    const collected = new Map<string, Group>()
    for (const video of videos.data ?? []) {
      const channel = byId.get(video.channelId)
      if (!channel) continue
      if (
        needle &&
        !`${video.title} ${video.ref} ${channel.name} ${stateLabel(video.state)}`
          .toLowerCase()
          .includes(needle)
      )
        continue
      const group = collected.get(channel.id) ?? { channel, videos: [] }
      group.videos.push(video)
      collected.set(channel.id, group)
    }
    // By creation, not activity: rows must not move while the pipeline runs.
    for (const group of collected.values()) {
      group.videos.sort((a, b) => b.createdAt.localeCompare(a.createdAt))
    }
    return [...collected.values()].sort((a, b) =>
      (b.videos[0]?.createdAt ?? '').localeCompare(a.videos[0]?.createdAt ?? ''),
    )
  }, [channels.data, videos.data, needle])

  const sortedChannels = useMemo(
    () =>
      [...(channels.data ?? [])]
        .filter((c) => !needle || `${c.name} ${c.slug}`.toLowerCase().includes(needle))
        .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt)),
    [channels.data, needle],
  )

  const isCollapsed = (id: string) => collapsed.has(id) && !needle
  const visible = useMemo(
    () =>
      groups.flatMap((group) =>
        collapsed.has(group.channel.id) && !needle ? [] : group.videos.map((video) => video.ref),
      ),
    [groups, collapsed, needle],
  )

  const live = useRef({ selected, visible, anchor, all: videos.data ?? [], groups })
  live.current = { selected, visible, anchor, all: videos.data ?? [], groups }

  const hoverTimer = useRef(0)
  useEffect(() => () => window.clearTimeout(hoverTimer.current), [])

  const actions = useMemo<VideoRowActions>(() => {
    const toggle = (ref: string) => {
      const { selected: current } = live.current
      select(current.includes(ref) ? current.filter((id) => id !== ref) : [...current, ref])
      setAnchor(ref)
    }
    const extend = (ref: string) => {
      const { anchor: from0, visible: rows } = live.current
      const from = from0 ? rows.indexOf(from0) : -1
      const to = rows.indexOf(ref)
      if (from < 0 || to < 0) {
        select([ref])
        setAnchor(ref)
        return
      }
      const [start, end] = from <= to ? [from, to] : [to, from]
      select(rows.slice(start, end + 1))
    }
    const remove = (targets: Video[]) =>
      void removeVideos(targets, client, (gone) =>
        select(live.current.selected.filter((id) => !gone.includes(id))),
      )
    return {
      select: (video, channel, event) => {
        if (event.shiftKey) return extend(video.ref)
        if (event.metaKey) return toggle(video.ref)
        select([video.ref])
        setAnchor(video.ref)
        openDoc({ kind: 'video', ref: video.ref }, video.title || 'Untitled', {
          preview: true,
          seed: channel.slug,
          initial: channel.name,
        })
      },
      open: (video) => pinPreview(docId({ kind: 'video', ref: video.ref })),
      contextMenu: (video) => {
        if (live.current.selected.includes(video.ref)) return
        select([video.ref])
        setAnchor(video.ref)
      },
      hover: (video, hovering) => {
        window.clearTimeout(hoverTimer.current)
        if (hovering) hoverTimer.current = window.setTimeout(() => prefetchVideo(client, video), 90)
      },
      menu: (video, channel) =>
        videoMenu(
          video,
          channel,
          targetsFor(video, live.current.selected, live.current.all),
          remove,
        ),
    }
  }, [select, client])

  const activeDoc = useDock((s) => s.activeDoc)
  useEffect(() => {
    if (!activeDoc) return
    const id =
      activeDoc.kind === 'video' && scope === 'videos'
        ? activeDoc.ref
        : activeDoc.kind === 'channel' && scope === 'channels'
          ? activeDoc.slug
          : null
    if (!id) return
    if (!useWorkbench.getState().selected.includes(id)) {
      select([id])
      setAnchor(id)
    }
    document
      .querySelector(`[data-row-id="${CSS.escape(docId(activeDoc))}"]`)
      ?.scrollIntoView({ block: 'nearest' })
  }, [activeDoc, scope, select])

  const onListKey = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
    if (event.metaKey || event.altKey || event.ctrlKey || scope !== 'videos') return
    const { visible: rows, selected: current, anchor: from, groups: all } = live.current
    if (rows.length === 0) return
    event.preventDefault()
    const step = event.key === 'ArrowDown' ? 1 : -1
    const head = current[current.length - 1] ?? from
    const at = head ? rows.indexOf(head) : -1
    const index =
      at < 0 ? (step > 0 ? 0 : rows.length - 1) : Math.min(rows.length - 1, Math.max(0, at + step))
    const ref = rows[index]
    if (!ref) return
    if (event.shiftKey && from) {
      const start = rows.indexOf(from)
      const [lo, hi] = start <= index ? [start, index] : [index, start]
      select([...rows.slice(lo, hi + 1).filter((row) => row !== ref), ref])
    } else {
      select([ref])
      setAnchor(ref)
      for (const group of all) {
        const video = group.videos.find((row) => row.ref === ref)
        if (!video) continue
        openDoc({ kind: 'video', ref }, video.title || 'Untitled', {
          preview: true,
          seed: group.channel.slug,
          initial: group.channel.name,
        })
        break
      }
    }
    document.querySelector<HTMLElement>(`[data-row-id="video:${CSS.escape(ref)}"]`)?.focus()
  }

  const selectedSet = useMemo(() => new Set(selected), [selected])
  const toggleGroup = (id: string) =>
    setCollapsed((previous) => {
      const next = new Set(previous)
      if (!next.delete(id)) next.add(id)
      return next
    })

  const loading = channels.isLoading || videos.isLoading
  const failure = channels.error ?? videos.error
  const empty = scope === 'videos' ? groups.length === 0 : sortedChannels.length === 0
  const channelFor = () => (scope === 'channels' ? selected[0] : undefined)
  const create: MenuEntry[] = [
    {
      label: 'New Video…',
      icon: SquarePen,
      shortcut: '$mod+n',
      onSelect: () => newVideo(channelFor()),
    },
    {
      label: 'New Video from Blueprint…',
      icon: ClipboardPaste,
      shortcut: '$mod+Alt+n',
      onSelect: () => newFromBlueprint(channelFor()),
    },
    { type: 'separator' },
    {
      label: 'New Channel…',
      icon: Tv,
      shortcut: '$mod+Shift+n',
      onSelect: () => void executeCommand('channel.new'),
    },
  ]

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="drag-region flex h-[var(--strip-height)] shrink-0 items-center gap-2 px-2">
        <div className="w-[var(--traffic-lights-inset)] shrink-0" />
        <div
          role="tablist"
          aria-label="Library"
          className="no-drag relative flex items-center gap-0.5 rounded-full bg-well p-0.5"
        >
          <SlidingThumb active={scope} />
          {SCOPES.map((s) => (
            <Tooltip key={s.value} content={s.title} command={s.command}>
              <button
                role="tab"
                aria-label={s.title}
                aria-selected={scope === s.value}
                data-thumb-key={s.value}
                onClick={() => setScope(s.value)}
                className={cn(
                  'relative flex h-6 min-w-8 items-center justify-center rounded-full px-2 transition-colors duration-150',
                  scope === s.value ? 'text-fg' : 'text-fg-subtle hover:text-fg',
                )}
              >
                <s.icon className="size-[15px]" strokeWidth={1.8} />
              </button>
            </Tooltip>
          ))}
        </div>
        <div className="flex-1" />
        <div className="no-drag glass-pill">
          <DropdownMenu
            align="end"
            items={create}
            trigger={
              <button
                aria-label="Create"
                className="inline-flex size-6 shrink-0 items-center justify-center rounded-full text-fg-muted transition-colors duration-100 hover:bg-hover hover:text-fg data-[state=open]:bg-active data-[state=open]:text-fg"
              >
                <SquarePen className="size-3.5" strokeWidth={1.9} />
              </button>
            }
          />
        </div>
      </div>

      <div className="px-2 pt-0.5 pb-1.5">
        <TextInput
          value={filter}
          onChange={setFilter}
          placeholder={scope === 'videos' ? 'Filter videos' : 'Filter channels'}
          icon={Search}
          onKeyDown={(e) => {
            if (e.key === 'Escape') setFilter('')
          }}
        />
      </div>

      <div
        key={scope}
        onKeyDown={onListKey}
        className="view-enter scroll-edge min-h-0 flex-1 overflow-x-hidden overflow-y-auto pb-3"
      >
        {loading && (
          <div className="flex justify-center py-8 text-fg-subtle">
            <Spinner />
          </div>
        )}
        {failure && <EmptyState title="Couldn't load the library">{failure.message}</EmptyState>}
        {!loading && !failure && empty && (
          <EmptyState
            icon={needle ? Search : scope === 'videos' ? Clapperboard : Tv}
            title={
              needle
                ? `Nothing matches “${filter.trim()}”`
                : scope === 'videos'
                  ? 'No videos yet'
                  : 'No channels yet'
            }
            action={
              needle ? undefined : (
                <button
                  onClick={() =>
                    scope === 'videos' ? newVideo() : void executeCommand('channel.new')
                  }
                  className="text-xs font-medium text-accent hover:underline"
                >
                  {scope === 'videos' ? 'New Video…' : 'New Channel…'}
                </button>
              )
            }
          />
        )}

        {scope === 'channels'
          ? sortedChannels.map((channel) => (
              <ChannelRow
                key={channel.id}
                channel={channel}
                selected={selectedSet.has(channel.slug)}
                onSelect={() => {
                  select([channel.slug])
                  openDoc({ kind: 'channel', slug: channel.slug }, channel.name, {
                    preview: true,
                    seed: channel.slug,
                    initial: channel.name,
                  })
                }}
              />
            ))
          : groups.map((group) => {
              const closed = isCollapsed(group.channel.id)
              return (
                <div key={group.channel.id} className="mb-1">
                  <GroupHeader
                    group={group}
                    collapsed={closed}
                    onToggle={() => toggleGroup(group.channel.id)}
                  />
                  {!closed &&
                    group.videos.map((video) => (
                      <VideoRow
                        key={video.id}
                        video={video}
                        channel={group.channel}
                        selected={selectedSet.has(video.ref)}
                        actions={actions}
                      />
                    ))}
                </div>
              )
            })}
      </div>
    </div>
  )
}

function targetsFor(video: Video, selected: string[], all: Video[]): Video[] {
  if (!selected.includes(video.ref)) return [video]
  const wanted = new Set(selected)
  const targets = all.filter((candidate) => wanted.has(candidate.ref))
  return targets.length > 0 ? targets : [video]
}

interface VideoRowActions {
  select: (video: Video, channel: Channel, event: MouseEvent<HTMLButtonElement>) => void
  open: (video: Video) => void
  contextMenu: (video: Video) => void
  hover: (video: Video, hovering: boolean) => void
  menu: (video: Video, channel: Channel) => MenuEntry[]
}

const STATE_TONE: Partial<Record<VideoState, string>> = {
  awaiting_approval: 'text-warning',
  running: 'text-accent',
  failed: 'text-danger',
  blocked: 'text-danger',
}

function StateMark({ state }: { state: VideoState }) {
  switch (state) {
    case 'awaiting_approval':
      return <span className="size-2 animate-pulse rounded-full bg-warning" />
    case 'running':
      return <Spinner className="size-3 text-accent" />
    case 'failed':
    case 'blocked':
      return <span className="size-2 rounded-full bg-danger" />
    case 'completed':
      return <span className="size-1.5 rounded-full bg-done" />
    default:
      return <span className="size-1.5 rounded-full bg-fg-faint/60" />
  }
}

const VideoRow = memo(function VideoRow({
  video,
  channel,
  selected,
  actions,
}: {
  video: Video
  channel: Channel
  selected: boolean
  actions: VideoRowActions
}) {
  const settled = video.state === 'completed' || video.state === 'cancelled'
  const tone = STATE_TONE[video.state]
  return (
    <ContextMenu items={() => actions.menu(video, channel)}>
      <button
        type="button"
        data-row-id={docId({ kind: 'video', ref: video.ref })}
        onClick={(event) => actions.select(video, channel, event)}
        onDoubleClick={() => actions.open(video)}
        onContextMenu={() => actions.contextMenu(video)}
        onPointerEnter={() => actions.hover(video, true)}
        onPointerLeave={() => actions.hover(video, false)}
        aria-current={selected ? 'page' : undefined}
        className={cn(
          'group/row mx-1.5 flex h-11 w-[calc(100%-12px)] items-center gap-2.5 rounded-[var(--radius-inner)] px-2 text-left transition-[background-color,opacity] duration-100',
          selected ? 'row-selected' : 'hover:bg-hover',
          settled && !selected && 'opacity-50 hover:opacity-100',
        )}
      >
        <span className="flex size-3.5 shrink-0 items-center justify-center self-start pt-[9px]">
          <StateMark state={video.state} />
        </span>
        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="flex min-w-0 items-baseline gap-2">
            <span
              className={cn(
                'min-w-0 flex-1 truncate text-sm',
                selected ? 'font-medium text-fg' : 'text-fg',
              )}
            >
              {video.title || 'Untitled'}
            </span>
            <span className="shrink-0 text-2xs text-fg-faint tabular-nums">
              {listTimestamp(video.createdAt)}
            </span>
          </span>
          <span className="flex min-w-0 items-center gap-1.5 text-xs text-fg-subtle">
            <span className="shrink-0 font-medium text-fg-muted tabular-nums">{video.ref}</span>
            <span className="text-fg-faint">·</span>
            <span className={cn('truncate', tone)}>{stateLabel(video.state)}</span>
          </span>
        </span>
      </button>
    </ContextMenu>
  )
})

function GroupHeader({
  group,
  collapsed,
  onToggle,
}: {
  group: Group
  collapsed: boolean
  onToggle: () => void
}) {
  const { channel, videos } = group
  const items: MenuEntry[] = [
    { label: 'New Video…', icon: SquarePen, onSelect: () => newVideo(channel.slug) },
    {
      label: 'New Video from Blueprint…',
      icon: ClipboardPaste,
      onSelect: () => newFromBlueprint(channel.slug),
    },
    { type: 'separator' },
    {
      label: 'Open Channel',
      icon: Tv,
      onSelect: () =>
        openDoc({ kind: 'channel', slug: channel.slug }, channel.name, {
          seed: channel.slug,
          initial: channel.name,
        }),
    },
  ]
  return (
    <ContextMenu items={items}>
      <div className="group/proj mt-1 flex h-7 items-center gap-1 pr-2 pl-2.5">
        <button
          onClick={onToggle}
          aria-expanded={!collapsed}
          className="flex min-w-0 flex-1 items-center gap-1.5 text-left"
        >
          <ChevronRight
            className={cn(
              'size-3 shrink-0 text-fg-subtle transition-transform duration-150',
              !collapsed && 'rotate-90',
            )}
            strokeWidth={2.4}
          />
          <span
            className="size-2 shrink-0 rounded-full"
            style={{ backgroundColor: avatarColor(channel.slug) }}
          />
          <span className="min-w-0 truncate text-xs font-semibold text-fg-muted">
            {channel.name}
          </span>
          {collapsed && (
            <span className="shrink-0 text-2xs font-medium text-fg-faint tabular-nums">
              {videos.length}
            </span>
          )}
        </button>
        <IconButton
          icon={Plus}
          label={`New video in ${channel.name}`}
          size="xs"
          className="opacity-0 group-hover/proj:opacity-100 focus-visible:opacity-100"
          onClick={() => newVideo(channel.slug)}
        />
      </div>
    </ContextMenu>
  )
}

function ChannelRow({
  channel,
  selected,
  onSelect,
}: {
  channel: Channel
  selected: boolean
  onSelect: () => void
}) {
  const items: MenuEntry[] = [
    { label: 'New Video…', icon: SquarePen, onSelect: () => newVideo(channel.slug) },
    {
      label: 'New Video from Blueprint…',
      icon: ClipboardPaste,
      onSelect: () => newFromBlueprint(channel.slug),
    },
  ]
  return (
    <ContextMenu items={items}>
      <button
        type="button"
        data-row-id={docId({ kind: 'channel', slug: channel.slug })}
        onClick={onSelect}
        onDoubleClick={() => pinPreview(docId({ kind: 'channel', slug: channel.slug }))}
        aria-current={selected ? 'page' : undefined}
        className={cn(
          'mx-1.5 flex h-8 w-[calc(100%-12px)] items-center gap-2 rounded-[var(--radius-inner)] px-2 text-left text-sm transition-colors duration-100',
          selected ? 'row-selected text-fg' : 'text-fg-muted hover:bg-hover hover:text-fg',
        )}
      >
        <Avatar name={channel.name} seed={channel.slug} className="size-5 text-[9px]" />
        <span className="min-w-0 flex-1 truncate">{channel.name}</span>
        {channel.credentials !== 'valid' && (
          <Tooltip content="Not connected to YouTube">
            <span className="size-1.5 shrink-0 rounded-full bg-warning" />
          </Tooltip>
        )}
        <span className="shrink-0 text-2xs text-fg-faint tabular-nums">
          {listTimestamp(channel.updatedAt)}
        </span>
      </button>
    </ContextMenu>
  )
}
