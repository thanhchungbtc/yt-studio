import { useQuery } from '@tanstack/react-query'
import { ChevronDown, ChevronRight, SquarePen, Tv } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'

import { api, qk } from '../../core/api'
import { listTimestamp } from '../../core/format'
import type { Channel, Video, VideoState } from '../../core/types'
import { useWorkbench, type SidebarScope } from '../../store/workbench'
import { openDoc, pinPreview, docId } from '../editor/dock'
import { newVideo } from '../new-video'
import { avatarColor } from '../ui/avatar'
import { DragRegion } from '../ui/drag-region'
import { Menu } from '../ui/menu'
import { Segmented, type Segment } from '../ui/segmented'
import { Row } from './row'

/**
 * The primary sidebar: the source list.
 *
 * It is one list with two scopes rather than two lists, because a channel and
 * its videos are the same journey at different depths — and because a segmented
 * control is how macOS says "same list, different slice", where a second
 * sidebar section would say "different thing entirely".
 *
 * Videos are grouped under their channel and each group collapses, which is
 * what makes a long library navigable without a tree: two levels, no more.
 *
 * There is no search field and there are no pane toggles. Both were controls
 * standing in for a keystroke — ⌘1, ⌘2, ⌘3 — and a sidebar that spends its
 * first fifty pixels on chrome has fifty fewer for the library.
 */

const SCOPES: readonly Segment<SidebarScope>[] = [
  { value: 'videos', label: 'Videos' },
  { value: 'channels', label: 'Channels' },
]

/** The colour a state earns on the token; a settled draft earns none. */
function stateDot(state: VideoState): string | undefined {
  switch (state) {
    case 'running':
      return 'var(--running)'
    case 'awaiting_approval':
      return 'var(--accent)'
    case 'failed':
    case 'blocked':
      return 'var(--failed)'
    case 'completed':
      return 'var(--done)'
    default:
      return undefined
  }
}

const STATE_LABEL: Record<VideoState, string> = {
  draft: 'Draft',
  running: 'Running',
  awaiting_approval: 'Needs approval',
  blocked: 'Blocked',
  completed: 'Completed',
  failed: 'Failed',
  cancelled: 'Cancelled',
}

interface Group {
  channel: Channel
  videos: Video[]
}

export function PrimarySidebar() {
  const scope = useWorkbench((s) => s.scope)
  const setScope = useWorkbench((s) => s.setScope)
  const selected = useWorkbench((s) => s.selected)
  const select = useWorkbench((s) => s.select)

  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set())

  const channels = useQuery({ queryKey: qk.channels, queryFn: api.listChannels })
  const videos = useQuery({ queryKey: qk.videos, queryFn: api.listVideos })

  const groups = useMemo<Group[]>(() => {
    const byId = new Map((channels.data ?? []).map((channel) => [channel.id, channel]))
    const collected = new Map<string, Group>()

    for (const video of videos.data ?? []) {
      const channel = byId.get(video.channelId)
      if (!channel) continue
      const group = collected.get(channel.id) ?? { channel, videos: [] }
      group.videos.push(video)
      collected.set(channel.id, group)
    }

    // Most recently touched first, at both levels: a library sorts itself by
    // what you were last doing, not by what it was called.
    for (const group of collected.values()) {
      group.videos.sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
    }
    return [...collected.values()].sort((a, b) =>
      (b.videos[0]?.updatedAt ?? '').localeCompare(a.videos[0]?.updatedAt ?? ''),
    )
  }, [channels.data, videos.data])

  const sortedChannels = useMemo(
    () => [...(channels.data ?? [])].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt)),
    [channels.data],
  )

  const toggleGroup = (id: string) =>
    setCollapsed((previous) => {
      const next = new Set(previous)
      if (!next.delete(id)) next.add(id)
      return next
    })

  const loading = channels.isLoading || videos.isLoading
  const failure = channels.error ?? videos.error
  const empty = scope === 'videos' ? groups.length === 0 : sortedChannels.length === 0

  return (
    <div className="surface-chrome flex h-full flex-col">
      {/* The strip the traffic lights sit over, and the widest piece of chrome
          in the window to pick it up by. */}
      <DragRegion className="flex h-[38px] shrink-0 items-center pr-2 pl-[var(--traffic-lights)]" />

      <DragRegion className="flex h-[30px] shrink-0 items-center gap-1 pr-1.5 pl-3">
        <span className="min-w-0 flex-1 truncate text-[11px] font-semibold tracking-[0.06em] text-tertiary uppercase">
          Library
        </span>
        <Menu
          items={[
            {
              label: 'New Video',
              icon: SquarePen,
              shortcut: '⌘N',
              onSelect: () => newVideo(scope === 'channels' ? (selected ?? undefined) : undefined),
            },
            {
              label: 'New Channel',
              icon: Tv,
              shortcut: '⇧⌘N',
              onSelect: () => openDoc({ kind: 'new', of: 'channel' }, 'New Channel'),
            },
          ]}
        >
          <button
            type="button"
            aria-label="Create"
            className="flex size-[22px] shrink-0 items-center justify-center rounded-md text-secondary transition-colors hover:bg-[var(--hover)] hover:text-primary"
          >
            <SquarePen className="size-[15px]" strokeWidth={1.75} />
          </button>
        </Menu>
      </DragRegion>

      <div className="px-2.5 pb-2">
        <Segmented segments={SCOPES} value={scope} onChange={setScope} />
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto pb-3">
        {loading ? <Notice>Loading…</Notice> : null}
        {failure ? <Notice>{failure.message}</Notice> : null}
        {!loading && !failure && empty ? <Notice>Nothing here yet.</Notice> : null}

        {scope === 'channels' ? (
          <div className="px-2">
            {sortedChannels.map((channel) => (
              <Row
                key={channel.id}
                id={docId({ kind: 'channel', slug: channel.slug })}
                title={channel.name}
                subtitle={channel.description || channel.slug}
                timestamp={listTimestamp(channel.updatedAt)}
                avatarName={channel.name}
                avatarSeed={channel.slug}
                dotColor={channel.credentials === 'valid' ? undefined : 'var(--failed)'}
                selected={selected === channel.slug}
                onSelect={() => {
                  select(channel.slug)
                  openDoc({ kind: 'channel', slug: channel.slug }, channel.name, {
                    preview: true,
                    seed: channel.slug,
                    initial: channel.name,
                  })
                }}
                onOpen={() => pinPreview(docId({ kind: 'channel', slug: channel.slug }))}
              />
            ))}
          </div>
        ) : (
          groups.map((group) => {
            const isCollapsed = collapsed.has(group.channel.id)
            return (
              <section key={group.channel.id}>
                <GroupHeader
                  name={group.channel.name}
                  color={avatarColor(group.channel.slug)}
                  count={group.videos.length}
                  collapsed={isCollapsed}
                  onToggle={() => toggleGroup(group.channel.id)}
                />
                {isCollapsed ? null : (
                  <div className="px-2 pt-1">
                    {group.videos.map((video) => (
                      <Row
                        key={video.id}
                        id={docId({ kind: 'video', ref: video.ref })}
                        title={video.title || 'Untitled'}
                        subtitle={`${group.channel.slug} · ${STATE_LABEL[video.state]}`}
                        timestamp={listTimestamp(video.updatedAt)}
                        avatarName={group.channel.name}
                        avatarSeed={group.channel.slug}
                        dotColor={stateDot(video.state)}
                        selected={selected === video.ref}
                        onSelect={() => {
                          select(video.ref)
                          openDoc({ kind: 'video', ref: video.ref }, video.title || video.ref, {
                            preview: true,
                            seed: group.channel.slug,
                            initial: group.channel.name,
                          })
                        }}
                        onOpen={() => pinPreview(docId({ kind: 'video', ref: video.ref }))}
                      />
                    ))}
                  </div>
                )}
              </section>
            )
          })
        )}
      </div>
    </div>
  )
}

function Notice({ children }: { children: ReactNode }) {
  return <p className="px-3 py-3 text-[12px] text-tertiary">{children}</p>
}

interface GroupHeaderProps {
  name: string
  color: string
  count: number
  collapsed: boolean
  onToggle: () => void
}

/**
 * The channel band.
 *
 * Full-bleed, edge to edge, against inset rows — the contrast is the whole
 * point. A header that shared the rows' margin would read as another row, and
 * a source list with two levels needs the levels to look unlike each other.
 */
function GroupHeader({ name, color, count, collapsed, onToggle }: GroupHeaderProps) {
  const Chevron = collapsed ? ChevronRight : ChevronDown
  return (
    <button
      type="button"
      onClick={onToggle}
      aria-expanded={!collapsed}
      className="surface-band hairline-b sticky top-0 z-10 flex w-full items-center gap-1.5 px-2.5 py-[5px] text-[11px] font-semibold tracking-[0.05em] text-tertiary uppercase"
    >
      <Chevron className="size-3 shrink-0" strokeWidth={2.5} />
      <span className="size-[7px] shrink-0 rounded-full" style={{ backgroundColor: color }} />
      <span className="min-w-0 flex-1 truncate text-left">{name}</span>
      <span className="shrink-0 tabular-nums">{count}</span>
    </button>
  )
}
