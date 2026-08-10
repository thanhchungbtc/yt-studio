import { useQuery } from '@tanstack/react-query'
import { Plus, Search, Settings as SettingsIcon, Terminal } from 'lucide-react'
import type { ReactNode } from 'react'

import { useWorkbenchStore } from '../lib/store'
import { Kbd } from '../ui/primitives'
import { api, qk } from '@/core/api'
import { formatRelative } from '@/core/format'

/**
 * The empty editor. It is what a group with no tabs shows, and it is where the
 * keyboard reference lives: the one screen you see before choosing anything is
 * the right place to say what the keys do.
 */
export function Welcome({
  onNewVideo,
  onOpenPalette,
}: {
  onNewVideo: () => void
  onOpenPalette: () => void
}) {
  const open = useWorkbenchStore((s) => s.open)
  const showBottom = useWorkbenchStore((s) => s.showBottom)
  const videos = useQuery({ queryKey: qk.videos({}), queryFn: () => api.listVideos({}) })

  const recent = [...(videos.data?.videos ?? [])]
    .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
    .slice(0, 6)

  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto max-w-2xl px-8 py-16">
        <div className="flex items-center gap-3">
          <span className="flex h-9 w-9 items-center justify-center rounded-[var(--radius-md)] bg-[hsl(var(--accent))] text-[hsl(var(--accent-fg))]">
            <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor" aria-hidden>
              <path d="M9 7v10l8-5z" />
            </svg>
          </span>
          <div>
            <h1 className="text-[18px] font-semibold tracking-[-0.01em] text-fg">yt-studio</h1>
            <p className="text-[12px] text-muted">Pick a video from the explorer, or start here.</p>
          </div>
        </div>

        <div className="mt-8 grid gap-6 sm:grid-cols-2">
          <div className="space-y-1">
            <Heading>Start</Heading>
            <Action icon={<Plus className="h-3.5 w-3.5" />} keys="$mod+KeyN" onClick={onNewVideo}>
              New video
            </Action>
            <Action
              icon={<Search className="h-3.5 w-3.5" />}
              keys="$mod+KeyP"
              onClick={onOpenPalette}
            >
              Go to anything
            </Action>
            <Action
              icon={<SettingsIcon className="h-3.5 w-3.5" />}
              keys="$mod+Comma"
              onClick={() => open({ kind: 'settings' }, { preview: false })}
            >
              Settings
            </Action>
            <Action
              icon={<Terminal className="h-3.5 w-3.5" />}
              keys="$mod+Digit2"
              onClick={() => showBottom('console')}
            >
              Console
            </Action>
          </div>

          <div className="space-y-1">
            <Heading>Recent</Heading>
            {recent.length === 0 && <p className="px-2 text-[12px] text-subtle">Nothing yet.</p>}
            {recent.map((video) => (
              <button
                key={video.id}
                type="button"
                onClick={() => open({ kind: 'video', ref: video.ref }, { preview: false })}
                className="flex w-full items-baseline gap-2 rounded-[var(--radius-sm)] px-2 py-1 text-left transition-colors hover:bg-[hsl(var(--bg-hover))]"
              >
                <span className="shrink-0 font-mono text-[10.5px] font-semibold text-[hsl(var(--accent))]">
                  {video.ref}
                </span>
                <span className="min-w-0 flex-1 truncate text-[12px] text-fg/90">
                  {video.title}
                </span>
                <span className="tabular shrink-0 text-[10.5px] text-subtle">
                  {formatRelative(video.updatedAt)}
                </span>
              </button>
            ))}
          </div>
        </div>

        <div className="mt-10 space-y-1">
          <Heading>Keyboard</Heading>
          <dl className="grid gap-x-8 gap-y-1 text-[11.5px] sm:grid-cols-2">
            <Shortcut keys="$mod+KeyP" label="Go to a video or channel" />
            <Shortcut keys="$mod+Shift+KeyP" label="Run a command" />
            <Shortcut keys="$mod+Digit1" label="Primary sidebar" />
            <Shortcut keys="$mod+Digit2" label="Bottom panel" />
            <Shortcut keys="$mod+Digit3" label="Secondary sidebar" />
            <Shortcut keys="$mod+Backslash" label="Split the editor" />
            <Shortcut keys="$mod+KeyW" label="Close the tab" />
            <Shortcut keys="$mod+Alt+ArrowRight" label="Next tab" />
            <Shortcut keys="Alt+ArrowDown" label="Next video (preview)" />
            <Shortcut keys="$mod+Enter" label="Approve the open gate" />
          </dl>
          <p className="px-2 pt-2 text-[11px] text-subtle">
            A single click previews — one italic tab, reused. Double-click, or press Enter, to keep
            it.
          </p>
        </div>
      </div>
    </div>
  )
}

function Heading({ children }: { children: ReactNode }) {
  return (
    <h2 className="mb-2 text-[10.5px] font-semibold uppercase tracking-[0.08em] text-subtle">
      {children}
    </h2>
  )
}

function Action({
  icon,
  keys,
  onClick,
  children,
}: {
  icon: ReactNode
  keys?: string
  onClick: () => void
  children: ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex w-full items-center gap-2 rounded-[var(--radius-sm)] px-2 py-1 text-left text-[12.5px] text-fg transition-colors hover:bg-[hsl(var(--bg-hover))]"
    >
      <span className="text-subtle">{icon}</span>
      <span className="min-w-0 flex-1 truncate">{children}</span>
      {keys && <Kbd keys={keys} />}
    </button>
  )
}

function Shortcut({ keys, label }: { keys: string; label: string }) {
  return (
    <div className="flex items-center gap-2 px-2 py-0.5">
      <dt className="min-w-0 flex-1 truncate text-muted">{label}</dt>
      <dd>
        <Kbd keys={keys} />
      </dd>
    </div>
  )
}
