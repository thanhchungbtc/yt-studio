import * as DialogPrimitive from '@radix-ui/react-dialog'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import {
  Activity,
  BookOpen,
  CornerDownLeft,
  Film,
  Moon,
  PanelLeft,
  Plus,
  Search,
  Settings as SettingsIcon,
  Sun,
  Tv,
  Wand2,
} from 'lucide-react'
import type { ReactNode } from 'react'
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'

import { Kbd } from '@/components/ui/primitives'
import { api, qk } from '@/lib/api'
import { useAppCommands } from '@/lib/app-commands'
import { videoStateLabel } from '@/lib/format'
import { useSidebar, useTheme } from '@/lib/workspace'
import { cn } from '@/lib/utils'

interface Command {
  id: string
  group: string
  label: string
  /** Extra text that should match but is drawn separately. */
  detail?: string
  hint?: string
  icon: ReactNode
  keys?: string
  run: () => void
}

/**
 * The command palette: routes, actions and every video by ref or title, all one
 * keystroke away.
 *
 * Matching is a subsequence scan scored on how early and how contiguously the
 * query lands, which is enough for a few hundred videos.
 */
export function CommandPalette({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { openCreateVideo } = useAppCommands()
  const [theme, toggleTheme] = useTheme()
  const { toggle: toggleSidebar } = useSidebar()
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const listRef = useRef<HTMLDivElement>(null)

  // Only fetched while the palette is open; it is never on the first-paint path.
  const videos = useQuery({
    queryKey: qk.videos({}),
    queryFn: () => api.listVideos({}),
    enabled: open,
  })
  const channels = useQuery({ queryKey: qk.channels, queryFn: api.listChannels, enabled: open })
  const presets = useQuery({ queryKey: qk.presets, queryFn: api.listPresets, enabled: open })

  const commands = useMemo<Command[]>(() => {
    const close = (run: () => void) => () => {
      onOpenChange(false)
      run()
    }

    const items: Command[] = [
      {
        id: 'nav-videos',
        group: 'Go to',
        label: 'Videos',
        icon: <Film className="h-4 w-4" />,
        keys: 'mod+1',
        run: close(() => void navigate({ to: '/videos' })),
      },
      {
        id: 'nav-channels',
        group: 'Go to',
        label: 'Channels',
        icon: <Tv className="h-4 w-4" />,
        keys: 'mod+2',
        run: close(() => void navigate({ to: '/channels' })),
      },
      {
        id: 'nav-scheduler',
        group: 'Go to',
        label: 'Scheduler',
        icon: <Activity className="h-4 w-4" />,
        keys: 'mod+3',
        run: close(() => void navigate({ to: '/scheduler' })),
      },
      {
        id: 'nav-settings',
        group: 'Go to',
        label: 'Settings',
        icon: <SettingsIcon className="h-4 w-4" />,
        keys: 'mod+4',
        run: close(() => void navigate({ to: '/settings' })),
      },
      {
        id: 'action-new-video',
        group: 'Actions',
        label: 'New video',
        detail: 'create render pipeline',
        icon: <Plus className="h-4 w-4" />,
        keys: 'mod+n',
        run: close(() => openCreateVideo()),
      },
      {
        id: 'action-sidebar',
        group: 'Actions',
        label: 'Toggle sidebar',
        icon: <PanelLeft className="h-4 w-4" />,
        keys: 'mod+b',
        run: close(toggleSidebar),
      },
      {
        id: 'action-theme',
        group: 'Actions',
        label: theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme',
        icon: theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />,
        run: close(toggleTheme),
      },
      {
        id: 'action-docs',
        group: 'Actions',
        label: 'Open the API documentation',
        detail: 'openapi huma docs',
        icon: <BookOpen className="h-4 w-4" />,
        run: close(() => window.open('/api/docs', '_blank', 'noopener')),
      },
    ]

    // Switching every provider at once is the settings edit most worth reaching
    // without going to the settings screen for it.
    for (const preset of presets.data ?? []) {
      items.push({
        id: `preset-${preset.name}`,
        group: 'Presets',
        label: `Switch backends to ${preset.title}`,
        detail: `preset providers ${preset.name}`,
        icon: <Wand2 className="h-4 w-4" />,
        run: close(() => {
          void api.applyPreset(preset.name).then(() => {
            void queryClient.invalidateQueries({ queryKey: qk.settings })
            void queryClient.invalidateQueries({ queryKey: qk.scheduler })
          })
        }),
      })
    }

    const channelName = new Map(channels.data?.map((c) => [c.id, c.name]))
    for (const video of videos.data?.videos ?? []) {
      items.push({
        id: `video-${video.id}`,
        group: 'Videos',
        label: video.title,
        detail: `${video.ref} ${channelName.get(video.channelId) ?? ''}`,
        hint: videoStateLabel(video.state),
        icon: (
          <span className="font-mono text-[10.5px] font-semibold text-[hsl(var(--accent))]">
            {video.ref}
          </span>
        ),
        run: close(() => void navigate({ to: '/videos/$ref', params: { ref: video.ref } })),
      })
    }

    for (const channel of channels.data ?? []) {
      items.push({
        id: `channel-${channel.id}`,
        group: 'Channels',
        label: channel.name,
        detail: channel.slug,
        icon: <Tv className="h-4 w-4" />,
        run: close(() => void navigate({ to: '/channels' })),
      })
    }

    return items
  }, [
    channels.data,
    navigate,
    onOpenChange,
    openCreateVideo,
    presets.data,
    queryClient,
    theme,
    toggleSidebar,
    toggleTheme,
    videos.data,
  ])

  const results = useMemo(() => rank(commands, query), [commands, query])

  // A new query invalidates the cursor; clamping instead of resetting would
  // leave the highlight on whatever happens to be at that index.
  useEffect(() => setActive(0), [query])
  useEffect(() => {
    if (open) {
      setQuery('')
      setActive(0)
    }
  }, [open])

  // Keep the cursor in view without smooth-scrolling, which lags behind a held
  // arrow key.
  useLayoutEffect(() => {
    listRef.current?.querySelector('[data-active="true"]')?.scrollIntoView({ block: 'nearest' })
  }, [active, results])

  const onKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === 'ArrowDown') {
      setActive((prev) => (results.length ? (prev + 1) % results.length : 0))
    } else if (event.key === 'ArrowUp') {
      setActive((prev) => (results.length ? (prev - 1 + results.length) % results.length : 0))
    } else if (event.key === 'Enter') {
      results[active]?.run()
    } else if (event.key === 'Home') {
      setActive(0)
    } else if (event.key === 'End') {
      setActive(Math.max(0, results.length - 1))
    } else {
      return
    }
    event.preventDefault()
  }

  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="animate-in-fade fixed inset-0 z-40 bg-black/50 backdrop-blur-[2px]" />
        <DialogPrimitive.Content
          onKeyDown={onKeyDown}
          className="animate-in-pop fixed left-1/2 top-[14vh] z-50 flex max-h-[62vh] w-[calc(100vw-2rem)] max-w-xl -translate-x-1/2 flex-col overflow-hidden rounded-[var(--radius-lg)] border border-[hsl(var(--border-strong))] bg-[hsl(var(--bg-elevated))] elev-3"
        >
          <DialogPrimitive.Title className="sr-only">Command palette</DialogPrimitive.Title>
          <DialogPrimitive.Description className="sr-only">
            Search videos, channels and actions. Arrow keys move, Enter runs.
          </DialogPrimitive.Description>

          <div className="flex shrink-0 items-center gap-2.5 border-b border-[hsl(var(--border))] px-3.5">
            <Search className="h-4 w-4 shrink-0 text-subtle" aria-hidden />
            <input
              autoFocus
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search videos, channels and actions…"
              aria-label="Search videos, channels and actions"
              className="h-12 min-w-0 flex-1 bg-transparent text-[14px] text-fg outline-none placeholder:text-subtle"
            />
            <span className="tabular shrink-0 text-[11px] text-subtle">{results.length}</span>
          </div>

          <div ref={listRef} className="min-h-0 flex-1 overflow-y-auto py-1.5">
            {results.length === 0 && (
              <p className="px-4 py-8 text-center text-[12.5px] text-subtle">
                Nothing matches “{query}”.
              </p>
            )}
            {results.map((command, index) => {
              const first = index === 0 || results[index - 1]?.group !== command.group
              return (
                <div key={command.id}>
                  {first && (
                    <p className="px-3.5 pb-1 pt-2 text-[10.5px] font-semibold uppercase tracking-wider text-subtle">
                      {command.group}
                    </p>
                  )}
                  <button
                    type="button"
                    data-active={index === active}
                    onMouseMove={() => setActive(index)}
                    onClick={command.run}
                    className={cn(
                      'flex w-full items-center gap-3 px-3.5 py-[7px] text-left transition-colors',
                      index === active
                        ? 'bg-[hsl(var(--accent)/0.14)]'
                        : 'hover:bg-[hsl(var(--bg-hover))]',
                    )}
                  >
                    <span
                      className={cn(
                        'flex h-5 w-11 shrink-0 items-center justify-center',
                        index === active ? 'text-[hsl(var(--accent))]' : 'text-subtle',
                      )}
                    >
                      {command.icon}
                    </span>
                    <span className="min-w-0 flex-1 truncate text-[13px] text-fg">
                      {command.label}
                    </span>
                    {command.hint && (
                      <span className="shrink-0 text-[11px] text-subtle">{command.hint}</span>
                    )}
                    {command.keys && <Kbd keys={command.keys} />}
                    {index === active && !command.keys && (
                      <CornerDownLeft className="h-3.5 w-3.5 shrink-0 text-subtle" aria-hidden />
                    )}
                  </button>
                </div>
              )
            })}
          </div>

          <div className="flex shrink-0 items-center gap-4 border-t border-[hsl(var(--border))] bg-subtle px-3.5 py-1.5 text-[11px] text-subtle">
            <span className="flex items-center gap-1.5">
              <Kbd keys="arrowup" />
              <Kbd keys="arrowdown" />
              navigate
            </span>
            <span className="flex items-center gap-1.5">
              <Kbd keys="enter" />
              open
            </span>
            <span className="ml-auto flex items-center gap-1.5">
              <Kbd keys="escape" />
              dismiss
            </span>
          </div>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}

/* --------------------------------------------------------------- matching */

const GROUP_ORDER = ['Go to', 'Actions', 'Presets', 'Videos', 'Channels']

/**
 * Scores each command against the query and returns the survivors, best first
 * but still clustered by group so the list does not reshuffle wildly as the
 * operator types.
 */
function rank(commands: Command[], query: string): Command[] {
  const trimmed = query.trim().toLowerCase()
  if (!trimmed) {
    return [...commands].sort((a, b) => GROUP_ORDER.indexOf(a.group) - GROUP_ORDER.indexOf(b.group))
  }

  const scored: { command: Command; score: number }[] = []
  for (const command of commands) {
    const haystack = `${command.label} ${command.detail ?? ''}`.toLowerCase()
    const score = subsequenceScore(haystack, trimmed)
    if (score > 0) scored.push({ command, score })
  }

  scored.sort(
    (a, b) =>
      GROUP_ORDER.indexOf(a.command.group) - GROUP_ORDER.indexOf(b.command.group) ||
      b.score - a.score ||
      a.command.label.localeCompare(b.command.label),
  )
  return scored.map((entry) => entry.command)
}

function subsequenceScore(haystack: string, needle: string): number {
  let score = 0
  let cursor = 0
  let previous = -2

  for (const character of needle) {
    const found = haystack.indexOf(character, cursor)
    if (found === -1) return 0
    // Adjacent matches and matches at a word boundary are what people mean.
    if (found === previous + 1) score += 6
    else if (found === 0 || haystack[found - 1] === ' ' || haystack[found - 1] === '-') score += 4
    else score += 1
    previous = found
    cursor = found + 1
  }
  // A short haystack that matched is a tighter match than a long one.
  return score + Math.max(0, 24 - haystack.length) / 8
}
