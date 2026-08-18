import * as DialogPrimitive from '@radix-ui/react-dialog'
import { useQuery } from '@tanstack/react-query'
import { Command } from 'cmdk'
import { Search, Terminal, Tv } from 'lucide-react'
import type { ReactNode } from 'react'
import { useEffect, useState } from 'react'

import { useAllCommands } from './lib/keys'
import { useWorkbenchStore } from './lib/store'
import { Kbd } from './ui/primitives'
import { VideoStateDot } from './ui/status'
import { api, qk } from '@/core/api'

export type PaletteMode = 'go' | 'command'

/**
 * Quick Open and the Command Palette — two modes of one surface, as in the
 * reference. `⌘P` goes to a thing; `⌘⇧P` runs a command; typing `>` at the head
 * of the query switches from the first to the second, and deleting it switches
 * back.
 *
 * Filtering, scoring, roving focus and the aria wiring belong to `cmdk`. The
 * first draft of this hand-rolled a substring match and called it search.
 */
export function Palette({
  open,
  mode,
  onOpenChange,
}: {
  open: boolean
  mode: PaletteMode
  onOpenChange: (open: boolean) => void
}) {
  const openDoc = useWorkbenchStore((s) => s.open)
  const commands = useAllCommands()
  const [query, setQuery] = useState('')

  // The mode is a property of how it was opened, but the query can change it:
  // a leading `>` is the command prefix, exactly where a user expects it.
  const commanding = query.startsWith('>') || (mode === 'command' && query === '')

  useEffect(() => {
    if (open) setQuery(mode === 'command' ? '>' : '')
  }, [open, mode])

  const videos = useQuery({
    queryKey: qk.videos({}),
    queryFn: () => api.listVideos({}),
    enabled: open,
  })
  const channels = useQuery({ queryKey: qk.channels, queryFn: api.listChannels, enabled: open })

  const close = () => onOpenChange(false)

  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="animate-in-scrim data-[state=closed]:animate-out-scrim fixed inset-0 z-40 bg-black/40" />
        {/* Centred by auto margins rather than by translating itself, so the
            animation owns `transform` outright. The previous version centred
            with a transform and animated one too: the keyframe was written for
            a dialog centred on both axes, and against a palette that is centred
            on one it worked out as a 300-pixel leap up the window. */}
        <DialogPrimitive.Content
          aria-describedby={undefined}
          className="animate-in-dialog data-[state=closed]:animate-out-dialog fixed inset-x-0 top-[12vh] z-50 mx-auto flex max-h-[70vh] w-[calc(100vw-2rem)] max-w-xl flex-col overflow-hidden rounded-[var(--radius-lg)] border border-[hsl(var(--border-strong))] bg-[hsl(var(--bg-elevated))] elev-3"
        >
          <DialogPrimitive.Title className="sr-only">
            {commanding ? 'Run a command' : 'Go to'}
          </DialogPrimitive.Title>

          <Command
            loop
            // cmdk scores on the item's `value`; the labels and keywords are
            // supplied per item so a ref matches as readily as a title.
            className="flex min-h-0 flex-col"
          >
            <div className="flex h-11 shrink-0 items-center gap-2 border-b border-[hsl(var(--border))] px-3">
              {commanding ? (
                <Terminal className="h-4 w-4 shrink-0 text-subtle" aria-hidden />
              ) : (
                <Search className="h-4 w-4 shrink-0 text-subtle" aria-hidden />
              )}
              <Command.Input
                value={query}
                onValueChange={setQuery}
                autoFocus
                placeholder={
                  commanding ? 'Run a command' : 'Go to a video or channel — > for commands'
                }
                className="h-full w-full bg-transparent text-[13px] text-fg outline-none placeholder:text-subtle"
              />
              <Kbd keys="Escape" />
            </div>

            <Command.List className="min-h-0 flex-1 overflow-y-auto p-1">
              <Command.Empty className="px-3 py-6 text-center text-[12px] text-subtle">
                Nothing matches.
              </Command.Empty>

              {commanding ? (
                <CommandGroups commands={commands} onRun={close} />
              ) : (
                <>
                  <Command.Group
                    heading="Videos"
                    className="[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:pb-0.5 [&_[cmdk-group-heading]]:pt-2 [&_[cmdk-group-heading]]:text-[10px] [&_[cmdk-group-heading]]:font-semibold [&_[cmdk-group-heading]]:uppercase [&_[cmdk-group-heading]]:tracking-[0.08em] [&_[cmdk-group-heading]]:text-subtle"
                  >
                    {(videos.data?.videos ?? []).map((video) => (
                      <Row
                        key={video.id}
                        value={`${video.ref} ${video.title}`}
                        icon={<VideoStateDot state={video.state} />}
                        label={video.title}
                        hint={video.ref}
                        onSelect={() => {
                          openDoc({ kind: 'video', ref: video.ref }, { preview: false })
                          close()
                        }}
                      />
                    ))}
                  </Command.Group>

                  <Command.Group
                    heading="Channels"
                    className="[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:pb-0.5 [&_[cmdk-group-heading]]:pt-2 [&_[cmdk-group-heading]]:text-[10px] [&_[cmdk-group-heading]]:font-semibold [&_[cmdk-group-heading]]:uppercase [&_[cmdk-group-heading]]:tracking-[0.08em] [&_[cmdk-group-heading]]:text-subtle"
                  >
                    {(channels.data ?? []).map((channel) => (
                      <Row
                        key={channel.id}
                        value={`${channel.slug} ${channel.name}`}
                        icon={<Tv className="h-3.5 w-3.5" />}
                        label={channel.name}
                        hint={channel.slug}
                        onSelect={() => {
                          openDoc({ kind: 'channel', slug: channel.slug }, { preview: false })
                          close()
                        }}
                      />
                    ))}
                  </Command.Group>
                </>
              )}
            </Command.List>
          </Command>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}

function CommandGroups({
  commands,
  onRun,
}: {
  commands: ReturnType<typeof useAllCommands>
  onRun: () => void
}) {
  const categories = [...new Set(commands.filter((c) => !c.hidden).map((c) => c.category))]

  return (
    <>
      {categories.map((category) => (
        <Command.Group
          key={category}
          heading={category}
          className="[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:pb-0.5 [&_[cmdk-group-heading]]:pt-2 [&_[cmdk-group-heading]]:text-[10px] [&_[cmdk-group-heading]]:font-semibold [&_[cmdk-group-heading]]:uppercase [&_[cmdk-group-heading]]:tracking-[0.08em] [&_[cmdk-group-heading]]:text-subtle"
        >
          {commands
            .filter((command) => !command.hidden && command.category === category)
            .map((command) => (
              <Row
                key={command.id}
                // The `>` the user typed must not be scored against the label.
                value={`> ${command.category} ${command.label}`}
                label={command.label}
                disabled={command.disabled}
                trailing={command.keys ? <Kbd keys={command.keys} /> : null}
                onSelect={() => {
                  command.run()
                  onRun()
                }}
              />
            ))}
        </Command.Group>
      ))}
    </>
  )
}

function Row({
  value,
  icon,
  label,
  hint,
  trailing,
  disabled,
  onSelect,
}: {
  value: string
  icon?: ReactNode
  label: string
  hint?: string
  trailing?: ReactNode
  disabled?: boolean
  onSelect: () => void
}) {
  return (
    <Command.Item
      value={value}
      disabled={disabled}
      onSelect={onSelect}
      className="flex cursor-pointer items-center gap-2.5 rounded-[var(--radius-xs)] px-2.5 py-1.5 text-[12.5px] text-fg data-[disabled=true]:opacity-40 data-[selected=true]:bg-[hsl(var(--bg-active))]"
    >
      {icon && (
        <span className="flex h-4 w-4 shrink-0 items-center justify-center text-subtle">
          {icon}
        </span>
      )}
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {hint && <span className="shrink-0 font-mono text-[10.5px] text-subtle">{hint}</span>}
      {trailing}
    </Command.Item>
  )
}
