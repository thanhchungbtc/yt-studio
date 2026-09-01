import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, ChevronRight, SquarePen, Trash2, Tv } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'

import { api, qk } from '../../core/api'
import { listTimestamp } from '../../core/format'
import type { Channel, Video, VideoState } from '../../core/types'
import { useWorkbench, type SidebarScope } from '../../store/workbench'
import { openDoc, pinPreview, docId, useDock } from '../editor/dock'
import { newVideo } from '../new-video'
import { avatarColor } from '../ui/avatar'
import { Button } from '../ui/button'
import { Dialog } from '../ui/dialog'
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

/**
 * The mark a state earns on the token, if any.
 *
 * Three states earn one and four do not, and that ratio is the whole point. A
 * dot on every row is not a signal — a finished library used to carry a green
 * one on all of them — so the settled states stay bare and the marked ones mean
 * *this wants you* or *this is moving*.
 *
 * The rhythm carries as much as the hue: waiting beats, working orbits. Read
 * with the colour thrown away, the two are still different marks.
 */
function stateMark(
  state: VideoState,
): { tone: 'accent' | 'running' | 'failed'; motion: 'working' | 'attention' } | undefined {
  switch (state) {
    case 'awaiting_approval':
      return { tone: 'accent', motion: 'attention' }
    case 'running':
      return { tone: 'running', motion: 'working' }
    case 'failed':
    case 'blocked':
      return { tone: 'failed', motion: 'attention' }
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
  // Where a ⇧-click measures from. Deliberately not in the store: it is the
  // shape of a gesture in progress, and nobody should find one waiting for them
  // after a relaunch.
  const [anchor, setAnchor] = useState<string | null>(null)
  // The videos the confirmation is about, and the only reason there is a
  // confirmation: deleting one unlinks files, and nothing puts them back.
  const [pending, setPending] = useState<Video[] | null>(null)
  const [partial, setPartial] = useState<string | null>(null)

  const client = useQueryClient()
  const remove = useMutation({
    // One request each: the server deletes by key and has no bulk verb. Settled
    // rather than all, because five deletions are five chances to fail and the
    // four that worked should still be gone.
    mutationFn: async (refs: string[]) => {
      const settled = await Promise.allSettled(refs.map((ref) => api.deleteVideo(ref)))
      const gone: string[] = []
      const failed: PromiseRejectedResult[] = []
      settled.forEach((result, index) => {
        const ref = refs[index]
        if (ref === undefined) return
        if (result.status === 'fulfilled') gone.push(ref)
        else failed.push(result)
      })
      return { gone, failed }
    },
    onSuccess: ({ gone, failed }) => {
      // A tab is a view of a row. With the row gone the document cannot load,
      // so the tab goes with it rather than being left to fail.
      const dock = useDock.getState().api
      for (const ref of gone) dock?.getPanel(docId({ kind: 'video', ref }))?.api.close()
      select(selected.filter((id) => !gone.includes(id)))
      // Nothing on the stream announces a deletion — the deltas are about work
      // happening, not about rows disappearing — so the list is asked again.
      void client.invalidateQueries({ queryKey: qk.videos })

      // A partial failure keeps the sheet up and says so. Reporting success
      // because most of it worked is how you lose track of what is still there.
      if (failed.length === 0) {
        setPending(null)
        setPartial(null)
        return
      }
      const first = failed[0]?.reason
      const reason = first instanceof Error ? first.message : 'the server refused'
      setPartial(
        `${failed.length} of ${gone.length + failed.length} could not be deleted — ${reason}`,
      )
    },
  })

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

    /*
      Newest first, at both levels, and by *creation* rather than by activity.

      Sorting by `updatedAt` is the obvious thing and it makes the list unusable
      while the pipeline runs: every task delta advances the video's timestamp,
      so rows overtake each other under the pointer and whole channel sections
      swap places between one frame and the next. You cannot click a row that
      moves while you reach for it.

      A creation date never changes, so the order is fixed the moment a video
      exists and the list is somewhere you can learn your way around. What each
      video is *doing* is what the mark on its token is for.
    */
    for (const group of collected.values()) {
      group.videos.sort((a, b) => b.createdAt.localeCompare(a.createdAt))
    }
    return [...collected.values()].sort((a, b) =>
      (b.videos[0]?.createdAt ?? '').localeCompare(a.videos[0]?.createdAt ?? ''),
    )
  }, [channels.data, videos.data])

  const sortedChannels = useMemo(
    () => [...(channels.data ?? [])].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt)),
    [channels.data],
  )

  /*
    Every video row on screen, in the order it is drawn.

    ⇧-click means "everything between here and the anchor", and *between* is a
    fact about the rendered list rather than about the data: the groups are
    ordered, and a collapsed one contributes nothing to reach across.
  */
  const visible = useMemo(
    () =>
      groups.flatMap((group) =>
        collapsed.has(group.channel.id) ? [] : group.videos.map((video) => video.ref),
      ),
    [groups, collapsed],
  )

  /** ⌘-click: add this row to the selection, or take it out again. */
  const toggle = (ref: string) => {
    select(selected.includes(ref) ? selected.filter((id) => id !== ref) : [...selected, ref])
    setAnchor(ref)
  }

  /** ⇧-click: everything from the anchor to here, inclusive, in drawn order. */
  const extend = (ref: string) => {
    const from = anchor ? visible.indexOf(anchor) : -1
    const to = visible.indexOf(ref)
    if (from < 0 || to < 0) {
      select([ref])
      setAnchor(ref)
      return
    }
    const [start, end] = from <= to ? [from, to] : [to, from]
    select(visible.slice(start, end + 1))
  }

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
              onSelect: () => newVideo(scope === 'channels' ? selected[0] : undefined),
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
                tone={channel.credentials === 'valid' ? undefined : 'failed'}
                selected={selected.includes(channel.slug)}
                onSelect={() => {
                  select([channel.slug])
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
                        // The state leads, in full strength, because it is the
                        // one part of this line anyone reads. The slug follows
                        // it rather than pushing it off the end.
                        subtitle={
                          <>
                            {/* The state at full strength, except when it is
                                "Completed" — a finished row whose loudest word
                                tells you to ignore it is backwards. */}
                            <span className={video.state === 'completed' ? undefined : 'row-state'}>
                              {STATE_LABEL[video.state]}
                            </span>
                            {` · ${group.channel.slug}`}
                          </>
                        }
                        // The date the order is built from. Showing "last
                        // touched" beside a list sorted by creation puts an
                        // older stamp above a newer one and reads as a bug.
                        timestamp={listTimestamp(video.createdAt)}
                        avatarName={group.channel.name}
                        avatarSeed={group.channel.slug}
                        tone={stateMark(video.state)?.tone}
                        motion={stateMark(video.state)?.motion}
                        finished={video.state === 'completed'}
                        selected={selected.includes(video.ref)}
                        // A plain click is what it always was: select this row
                        // and show it. The modifiers only ever change the
                        // selection — neither opens anything, because a gesture
                        // for picking five things should not also open five
                        // documents.
                        onSelect={(event) => {
                          if (event.shiftKey) return extend(video.ref)
                          if (event.metaKey) return toggle(video.ref)
                          select([video.ref])
                          setAnchor(video.ref)
                          openDoc({ kind: 'video', ref: video.ref }, video.title || video.ref, {
                            preview: true,
                            seed: group.channel.slug,
                            initial: group.channel.name,
                          })
                        }}
                        onOpen={() => pinPreview(docId({ kind: 'video', ref: video.ref }))}
                        // Finder's rule, and the one that stops you losing the
                        // wrong thing: right-clicking inside the selection
                        // leaves it alone, right-clicking outside it collapses
                        // onto the row you pointed at.
                        onContextMenu={() => {
                          if (selected.includes(video.ref)) return
                          select([video.ref])
                          setAnchor(video.ref)
                        }}
                        menu={[
                          {
                            label: deleteLabel(targetsFor(video, selected, videos.data ?? [])),
                            icon: Trash2,
                            danger: true,
                            // The ellipsis is the promise: this opens a question
                            // rather than doing the thing.
                            onSelect: () =>
                              setPending(targetsFor(video, selected, videos.data ?? [])),
                          },
                        ]}
                      />
                    ))}
                  </div>
                )}
              </section>
            )
          })
        )}
      </div>

      {pending ? (
        <Dialog
          open
          onOpenChange={(next) => {
            if (!next) {
              setPending(null)
              setPartial(null)
            }
          }}
          width={400}
        >
          <Dialog.Header
            title={
              pending.length > 1
                ? `Delete ${pending.length} videos?`
                : `Delete “${pending[0]?.title || pending[0]?.ref}”?`
            }
            description="Their chapters, their tasks and every file nothing else is using go with them. This cannot be undone."
          />
          {(partial ?? remove.error) ? (
            <Dialog.Body>
              <p className="text-[12px] text-[var(--failed)]">
                {partial ?? (remove.error as Error).message}
              </p>
            </Dialog.Body>
          ) : null}
          {/* Cancel is the default and therefore last, which is the macOS order
              and the right one when the other button cannot be undone. */}
          <Dialog.Footer>
            <span className="mr-auto" />
            <Button
              onClick={() => remove.mutate(pending.map((video) => video.ref))}
              disabled={remove.isPending}
            >
              {remove.isPending ? 'Deleting…' : 'Delete'}
            </Button>
            <Button
              primary
              onClick={() => {
                setPending(null)
                setPartial(null)
              }}
            >
              Cancel
            </Button>
          </Dialog.Footer>
        </Dialog>
      ) : null}
    </div>
  )
}

/**
 * What a delete on this row would take.
 *
 * The selection when the row is part of it, and just the row when it is not —
 * the same rule the right-click applies, restated here because the menu's label
 * and the menu's action must never disagree about what they mean.
 */
function targetsFor(video: Video, selected: string[], all: Video[]): Video[] {
  if (!selected.includes(video.ref)) return [video]
  const wanted = new Set(selected)
  const targets = all.filter((candidate) => wanted.has(candidate.ref))
  return targets.length > 0 ? targets : [video]
}

function deleteLabel(targets: Video[]): string {
  return targets.length > 1 ? `Delete ${targets.length} Videos…` : 'Delete…'
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
