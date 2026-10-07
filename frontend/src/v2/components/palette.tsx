import { useQuery } from '@tanstack/react-query'
import { Command as Cmdk } from 'cmdk'
import { Clapperboard, Search, Tv } from 'lucide-react'
import { Dialog as RDialog } from 'radix-ui'
import { startTransition, useEffect, useMemo, useState } from 'react'

import { evaluateWhen } from '@/kit/commands/context'
import { executeCommand, useCommandRegistry } from '@/kit/commands/registry'
import { keepFocusIfMoved } from '@/kit/lib/focus'
import { matchScore } from '@/kit/lib/match'
import { KeybindingHint } from '@/kit/ui/Kbd'

import { api, qk } from '../core/api'
import { useWorkbench } from '../store/workbench'
import { openDoc } from './editor/dock'
import { listTimestamp, stateLabel } from '../core/format'
import { Avatar } from './ui/avatar'

const itemClass =
  'flex h-8 cursor-default items-center gap-2.5 rounded-lg px-2.5 text-sm text-fg data-[selected=true]:bg-row-highlight'

/** ⌘K: commands, videos and channels in one list. ⌘P: videos only. */
export function CommandPalette() {
  const { open, mode } = useWorkbench((s) => s.palette)
  const closePalette = useWorkbench((s) => s.closePalette)
  const [search, setSearch] = useState('')
  const commands = useCommandRegistry((s) => s.commands)
  const videos = useQuery({ queryKey: qk.videos, queryFn: api.listVideos, enabled: open })
  const channels = useQuery({ queryKey: qk.channels, queryFn: api.listChannels, enabled: open })

  const visibleCommands = useMemo(
    () =>
      open && mode === 'commands'
        ? Object.values(commands)
            .filter((c) => !c.hidden && evaluateWhen(c.when))
            .sort(
              (a, b) =>
                (a.category ?? '').localeCompare(b.category ?? '') ||
                a.title.localeCompare(b.title),
            )
        : [],
    [commands, open, mode],
  )
  // The first screenful paints at once; the rest mount right after.
  const [everything, setEverything] = useState(false)
  useEffect(() => {
    if (!open) return setEverything(false)
    const frame = requestAnimationFrame(() => startTransition(() => setEverything(true)))
    return () => cancelAnimationFrame(frame)
  }, [open])

  const channelOf = useMemo(
    () => new Map((channels.data ?? []).map((c) => [c.id, c])),
    [channels.data],
  )
  const allVideos = videos.data ?? []
  const videoLimit = mode === 'videos' || search ? 500 : everything ? 6 : 4
  const shownCommands = everything || search ? visibleCommands : visibleCommands.slice(0, 24)

  const close = () => {
    closePalette()
    setSearch('')
  }
  const run = (fn: () => void) => {
    close()
    // After the dialog has closed and returned focus.
    requestAnimationFrame(fn)
  }

  return (
    <RDialog.Root modal={false} open={open} onOpenChange={(next) => !next && close()}>
      <RDialog.Portal>
        <RDialog.Content
          aria-describedby={undefined}
          onCloseAutoFocus={keepFocusIfMoved}
          className="glass-pop glass-dense animate-pop-in fixed top-[12vh] left-1/2 z-50 w-[min(620px,calc(100vw-48px))] -translate-x-1/2 overflow-hidden rounded-[20px] outline-none"
        >
          <RDialog.Title className="sr-only">Command palette</RDialog.Title>
          <Cmdk
            loop
            label="Command palette"
            filter={matchScore}
            className="flex max-h-[60vh] flex-col"
          >
            <div className="flex items-center gap-2.5 border-b border-line px-4">
              <Search className="size-4 text-fg-subtle" />
              <Cmdk.Input
                value={search}
                onValueChange={setSearch}
                placeholder={
                  mode === 'videos' ? 'Go to video…' : 'Search commands, videos and channels…'
                }
                className="text-md h-12 flex-1 bg-transparent text-fg outline-none placeholder:text-fg-subtle"
              />
            </div>
            <Cmdk.List className="min-h-0 overflow-auto p-1.5 [&_[cmdk-group-heading]]:px-2.5 [&_[cmdk-group-heading]]:pt-2 [&_[cmdk-group-heading]]:pb-1 [&_[cmdk-group-heading]]:text-2xs [&_[cmdk-group-heading]]:font-semibold [&_[cmdk-group-heading]]:tracking-wide [&_[cmdk-group-heading]]:text-fg-subtle [&_[cmdk-group-heading]]:uppercase">
              <Cmdk.Empty className="px-3 py-6 text-center text-sm text-fg-subtle">
                No results
              </Cmdk.Empty>

              <Cmdk.Group heading="Videos">
                {allVideos.slice(0, videoLimit).map((v) => {
                  const channel = channelOf.get(v.channelId)
                  return (
                    <Cmdk.Item
                      key={v.id}
                      value={`video ${v.ref} ${v.title} ${channel?.name ?? ''} ${v.id}`}
                      className={itemClass}
                      onSelect={() =>
                        run(() =>
                          openDoc({ kind: 'video', ref: v.ref }, v.title || 'Untitled', {
                            seed: channel?.slug,
                            initial: channel?.name,
                          }),
                        )
                      }
                    >
                      <Clapperboard className="size-4 shrink-0 opacity-70" />
                      <span className="shrink-0 text-fg-subtle tabular-nums">{v.ref}</span>
                      <span className="min-w-0 flex-1 truncate">{v.title || 'Untitled'}</span>
                      <span className="flex shrink-0 items-center gap-1.5 text-2xs text-fg-subtle">
                        {stateLabel(v.state)} · {listTimestamp(v.updatedAt)}
                      </span>
                    </Cmdk.Item>
                  )
                })}
              </Cmdk.Group>

              {mode === 'commands' && (
                <Cmdk.Group heading="Channels">
                  {(channels.data ?? []).slice(0, search ? 100 : 3).map((c) => (
                    <Cmdk.Item
                      key={c.id}
                      value={`channel ${c.name} ${c.slug}`}
                      className={itemClass}
                      onSelect={() =>
                        run(() =>
                          openDoc({ kind: 'channel', slug: c.slug }, c.name, {
                            seed: c.slug,
                            initial: c.name,
                          }),
                        )
                      }
                    >
                      <Avatar name={c.name} seed={c.slug} className="size-4 text-[9px]" />
                      <span className="min-w-0 flex-1 truncate">{c.name}</span>
                      <Tv className="size-3.5 shrink-0 text-fg-subtle" />
                    </Cmdk.Item>
                  ))}
                </Cmdk.Group>
              )}

              {mode === 'commands' && (
                <Cmdk.Group heading="Commands">
                  {shownCommands.map((c) => {
                    const Icon = c.icon
                    return (
                      <Cmdk.Item
                        key={c.id}
                        value={`${c.category ?? ''} ${c.title} ${c.id}`}
                        className={itemClass}
                        onSelect={() => run(() => void executeCommand(c.id))}
                      >
                        {Icon ? (
                          <Icon className="size-4 shrink-0 opacity-70" />
                        ) : (
                          <span className="size-4" />
                        )}
                        <span className="min-w-0 flex-1 truncate">
                          {c.category && <span className="text-fg-subtle">{c.category}: </span>}
                          {c.title}
                        </span>
                        <KeybindingHint command={c.id} />
                      </Cmdk.Item>
                    )
                  })}
                </Cmdk.Group>
              )}
            </Cmdk.List>
          </Cmdk>
        </RDialog.Content>
      </RDialog.Portal>
    </RDialog.Root>
  )
}
