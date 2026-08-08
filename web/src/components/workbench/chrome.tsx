import { useQuery } from '@tanstack/react-query'
import {
  FolderTree,
  Moon,
  PanelBottom,
  PanelLeft,
  PanelRight,
  Radio,
  Search,
  Settings as SettingsIcon,
  Sun,
} from 'lucide-react'
import type { ReactNode } from 'react'

import { useActiveTab, useWorkbenchStore } from './lib/store'
import { IconButton } from './ui/controls'
import { Divider, Kbd, Tooltip } from './ui/primitives'
import { PoolChip } from './ui/status'
import { api, qk } from '@/core/api'
import type { ConnectionState } from '@/core/events'
import { useTheme } from '@/core/theme'
import { cn } from '@/core/utils'

/* -------------------------------------------------------------- title bar */

/**
 * 32 pixels of window chrome carrying what this is, the one control reachable
 * from every document, and the panel toggles.
 *
 * There is no breadcrumb up here any more — the document draws its own, beneath
 * its tab, where it belongs once more than one document can be open.
 */
export function TitleBar({ onOpenPalette }: { onOpenPalette: () => void }) {
  const explorerVisible = useWorkbenchStore((s) => s.explorerVisible)
  const toggleExplorer = useWorkbenchStore((s) => s.toggleExplorer)
  const asideVisible = useWorkbenchStore((s) => s.asideVisible)
  const toggleAside = useWorkbenchStore((s) => s.toggleAside)
  const bottomVisible = useWorkbenchStore((s) => s.bottomVisible)
  const toggleBottom = useWorkbenchStore((s) => s.toggleBottom)

  const tab = useActiveTab()
  const asideAvailable = tab?.doc.kind === 'video'

  return (
    <header className="flex h-8 shrink-0 items-center gap-3 border-b border-[hsl(var(--border))] bg-chrome px-2 no-select">
      <div className="flex flex-1 items-center gap-2">
        <span className="flex h-[17px] w-[17px] shrink-0 items-center justify-center rounded-[var(--radius-xs)] bg-[hsl(var(--accent))] text-[hsl(var(--accent-fg))]">
          <svg viewBox="0 0 24 24" className="h-2.5 w-2.5" fill="currentColor" aria-hidden>
            <path d="M9 7v10l8-5z" />
          </svg>
        </span>
        <span className="shrink-0 text-[11.5px] font-semibold tracking-[-0.01em] text-fg">
          yt-studio
        </span>
      </div>

      <button
        type="button"
        onClick={onOpenPalette}
        className="flex h-[22px] w-full max-w-sm shrink-0 items-center gap-2 rounded-[var(--radius-sm)] border border-[hsl(var(--border))] bg-[hsl(var(--bg))] px-2 text-[11.5px] text-subtle transition-colors hover:border-[hsl(var(--border-strong))] hover:text-muted"
      >
        <Search className="h-3 w-3 shrink-0" aria-hidden />
        <span className="truncate">Go to a video or channel</span>
        <Kbd keys="$mod+KeyP" className="ml-auto" />
      </button>

      <div className="flex flex-1 shrink-0 items-center justify-end gap-0.5">
        <Tooltip
          label={explorerVisible ? 'Hide the explorer' : 'Show the explorer'}
          keys="$mod+KeyB"
          side="bottom"
        >
          <IconButton
            aria-label="Toggle the explorer"
            active={explorerVisible}
            onClick={toggleExplorer}
          >
            <PanelLeft className="h-3.5 w-3.5" />
          </IconButton>
        </Tooltip>
        <Tooltip
          label={bottomVisible ? 'Hide the panel' : 'Show the panel'}
          keys="$mod+KeyJ"
          side="bottom"
        >
          <IconButton aria-label="Toggle the panel" active={bottomVisible} onClick={toggleBottom}>
            <PanelBottom className="h-3.5 w-3.5" />
          </IconButton>
        </Tooltip>
        <Tooltip
          label={asideVisible ? 'Hide the run panel' : 'Show the run panel'}
          keys="$mod+Shift+KeyB"
          side="bottom"
        >
          <IconButton
            aria-label="Toggle the run panel"
            active={asideVisible && asideAvailable}
            disabled={!asideAvailable}
            onClick={toggleAside}
          >
            <PanelRight className="h-3.5 w-3.5" />
          </IconButton>
        </Tooltip>
        <ThemeToggle />
      </div>
    </header>
  )
}

function ThemeToggle() {
  const [theme, toggle] = useTheme()
  const dark = theme === 'dark'

  return (
    <Tooltip label={dark ? 'Switch to light' : 'Switch to dark'} side="bottom">
      <IconButton
        aria-label={dark ? 'Switch to light theme' : 'Switch to dark theme'}
        onClick={toggle}
      >
        {dark ? <Sun className="h-3.5 w-3.5" /> : <Moon className="h-3.5 w-3.5" />}
      </IconButton>
    </Tooltip>
  )
}

/* ----------------------------------------------------------- activity bar */

/**
 * The rail. Views at the top, the gear pinned to the bottom — the arrangement
 * the reference uses, and the reason settings is not a view: it is a document
 * that wants the whole width, not a tree that fits in 288 pixels.
 */
export function ActivityBar() {
  const explorerVisible = useWorkbenchStore((s) => s.explorerVisible)
  const toggleExplorer = useWorkbenchStore((s) => s.toggleExplorer)
  const open = useWorkbenchStore((s) => s.open)
  const tab = useActiveTab()

  return (
    <nav
      aria-label="Primary"
      className="flex w-12 shrink-0 flex-col items-center gap-1 border-r border-[hsl(var(--border))] bg-panel py-2 no-select"
    >
      <RailButton
        label="Explorer"
        keys="$mod+Shift+KeyE"
        // Active means "this view is showing", so clicking the lit icon puts the
        // panel away — the toggle every editor rail has.
        active={explorerVisible}
        onClick={toggleExplorer}
      >
        <FolderTree className="h-[18px] w-[18px]" />
      </RailButton>

      <div className="mt-auto" />

      <RailButton
        label="Settings"
        keys="$mod+Comma"
        active={tab?.doc.kind === 'settings'}
        onClick={() => open({ kind: 'settings' }, { preview: false })}
      >
        <SettingsIcon className="h-[18px] w-[18px]" />
      </RailButton>
    </nav>
  )
}

function RailButton({
  label,
  keys,
  active,
  onClick,
  children,
}: {
  label: string
  keys?: string
  active: boolean
  onClick: () => void
  children: ReactNode
}) {
  return (
    <Tooltip label={label} {...(keys ? { keys } : {})} side="right">
      <button
        type="button"
        aria-label={label}
        aria-pressed={active}
        onClick={onClick}
        className={cn(
          'relative flex h-9 w-9 items-center justify-center rounded-[var(--radius-sm)] transition-colors',
          active ? 'text-fg' : 'text-subtle hover:bg-[hsl(var(--bg-hover))] hover:text-fg',
        )}
      >
        {active && (
          <span
            aria-hidden
            className="absolute left-[-6px] h-5 w-[2px] rounded-full bg-[hsl(var(--accent))]"
          />
        )}
        {children}
      </button>
    </Tooltip>
  )
}

/* ------------------------------------------------------------- status bar */

const CONNECTION_LABEL: Record<ConnectionState, string> = {
  live: 'Live',
  connecting: 'Reconnecting',
  offline: 'Offline',
}

/**
 * The status bar, and the reason there is no scheduler in the rail: capacity is
 * on screen at all times anyway, and the pools are the click that opens the
 * console in the bottom panel.
 */
export function StatusBar({ connection }: { connection: ConnectionState }) {
  const showBottom = useWorkbenchStore((s) => s.showBottom)
  const bottomVisible = useWorkbenchStore((s) => s.bottomVisible)
  const bottomView = useWorkbenchStore((s) => s.bottomView)

  const { data: status } = useQuery({
    queryKey: qk.scheduler,
    queryFn: api.schedulerStatus,
    // The SSE stream keeps this current; the interval is only a safety net.
    refetchInterval: 30_000,
  })
  const { data: health } = useQuery({
    queryKey: ['health'],
    queryFn: api.health,
    staleTime: 60_000,
  })

  const running = status?.pools.reduce((sum, p) => sum + p.inFlight, 0) ?? 0
  const failed = status?.failed ?? 0
  const consoleOpen = bottomVisible && bottomView === 'console'

  return (
    <footer className="flex h-6 shrink-0 items-center gap-3 border-t border-[hsl(var(--border))] bg-panel px-3 text-[11px] text-muted no-select">
      <Tooltip
        label={
          connection === 'live'
            ? 'Streaming updates from the server'
            : 'The event stream is down; it will reconnect and resume'
        }
      >
        <button
          type="button"
          onClick={() => showBottom('output')}
          className="flex shrink-0 items-center gap-1.5 rounded-[var(--radius-xs)] px-1 transition-colors hover:bg-[hsl(var(--bg-hover))]"
        >
          <span
            className={cn(
              'h-1.5 w-1.5 rounded-full',
              connection === 'live'
                ? 'bg-[hsl(var(--success))]'
                : connection === 'connecting'
                  ? 'bg-[hsl(var(--warning))] pulse-live'
                  : 'bg-[hsl(var(--danger))]',
            )}
          />
          <Radio className="h-3 w-3 text-subtle" aria-hidden />
          <span>{CONNECTION_LABEL[connection]}</span>
        </button>
      </Tooltip>

      <Divider />

      <button
        type="button"
        onClick={() => showBottom('console')}
        aria-label="Open the console"
        className={cn(
          'flex min-w-0 items-center gap-1 overflow-hidden rounded-[var(--radius-xs)] px-1 py-0.5 transition-colors hover:bg-[hsl(var(--bg-hover))]',
          consoleOpen && 'bg-[hsl(var(--bg-active))]',
        )}
      >
        {status?.pools.map((pool) => (
          <PoolChip key={pool.pool} stat={pool} />
        ))}
      </button>

      <Divider className="ml-1" />
      <span className="tabular shrink-0">
        <span className="text-subtle">running</span> {running}
      </span>
      <span className="tabular shrink-0">
        <span className="text-subtle">ready</span> {status?.ready ?? 0}
      </span>
      <span className="tabular hidden shrink-0 md:inline">
        <span className="text-subtle">videos</span> {status?.videos ?? 0}
      </span>
      {failed > 0 && (
        <button
          type="button"
          onClick={() => showBottom('console')}
          className="tabular shrink-0 text-[hsl(var(--danger))] hover:underline"
        >
          {failed} failed
        </button>
      )}

      <span className="ml-auto shrink-0 text-subtle">{health?.version ?? ''}</span>
    </footer>
  )
}
