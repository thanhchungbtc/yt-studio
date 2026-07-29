import { useQuery } from '@tanstack/react-query'
import { Link, Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import {
  Activity,
  Film,
  Keyboard,
  Moon,
  Radio,
  Search,
  Settings as SettingsIcon,
  Sun,
  Tv,
} from 'lucide-react'
import type { ReactNode } from 'react'

import { PoolChip } from '@/components/pool-chip'
import { Divider, Kbd, Tooltip } from '@/components/ui/primitives'
import { WorkspaceProvider } from '@/components/workspace'
import { api, qk } from '@/lib/api'
import { useAppCommands } from '@/lib/app-commands'
import { useEventStream, type ConnectionState } from '@/lib/events'
import { formatDuration } from '@/lib/format'
import { useHotkeys } from '@/lib/hotkeys'
import { useTheme } from '@/lib/workspace'
import { cn } from '@/lib/utils'

interface NavItem {
  to: string
  label: string
  keys: string
  icon: ReactNode
}

const NAV: NavItem[] = [
  { to: '/videos', label: 'Videos', keys: 'mod+1', icon: <Film className="h-[18px] w-[18px]" /> },
  { to: '/channels', label: 'Channels', keys: 'mod+2', icon: <Tv className="h-[18px] w-[18px]" /> },
  {
    to: '/scheduler',
    label: 'Scheduler',
    keys: 'mod+3',
    icon: <Activity className="h-[18px] w-[18px]" />,
  },
  {
    to: '/settings',
    label: 'Settings',
    keys: 'mod+4',
    icon: <SettingsIcon className="h-[18px] w-[18px]" />,
  },
]

/**
 * The window.
 *
 * A title bar across the top, an activity bar down the left, a status bar along
 * the bottom, and the route filling everything between. Nothing here scrolls;
 * only the panes inside a route do, which is what keeps the chrome fixed the
 * way a native window's is.
 */
export function AppShell() {
  return (
    <WorkspaceProvider>
      <Shell />
    </WorkspaceProvider>
  )
}

function Shell() {
  const connection = useEventStream()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const navigate = useNavigate()

  useHotkeys(
    NAV.map((item) => ({
      keys: item.keys,
      label: item.label,
      group: 'Navigation',
      run: () => void navigate({ to: item.to }),
    })),
  )

  return (
    <div className="flex h-full w-full flex-col overflow-hidden bg-app">
      <TitleBar pathname={pathname} />
      <div className="flex min-h-0 flex-1">
        <ActivityBar pathname={pathname} />
        <main className="flex min-w-0 flex-1 flex-col overflow-hidden">
          <Outlet />
        </main>
      </div>
      <StatusBar connection={connection} />
    </div>
  )
}

/* ------------------------------------------------------------------ chrome */

const SECTION_LABEL: { prefix: string; label: string }[] = [
  { prefix: '/videos', label: 'Videos' },
  { prefix: '/channels', label: 'Channels' },
  { prefix: '/scheduler', label: 'Scheduler' },
  { prefix: '/settings', label: 'Settings' },
]

/**
 * The title bar carries the one control that has to be reachable from every
 * screen: the palette. It is drawn as a search field because that is what it
 * is — everything else on it is identity.
 */
function TitleBar({ pathname }: { pathname: string }) {
  const { openPalette } = useAppCommands()
  const section = SECTION_LABEL.find((entry) => pathname.startsWith(entry.prefix))?.label

  return (
    <header className="flex h-9 shrink-0 items-center gap-3 border-b border-[hsl(var(--border))] bg-chrome px-3 no-select">
      <div className="flex w-40 shrink-0 items-center gap-2">
        <span className="flex h-[18px] w-[18px] items-center justify-center rounded-[var(--radius-xs)] bg-[hsl(var(--accent))] text-[hsl(var(--accent-fg))]">
          <svg viewBox="0 0 24 24" className="h-3 w-3" fill="currentColor" aria-hidden>
            <path d="M9 7v10l8-5z" />
          </svg>
        </span>
        <span className="text-[12px] font-semibold tracking-[-0.01em] text-fg">yt-studio</span>
        {section && (
          <>
            <span className="text-subtle" aria-hidden>
              ›
            </span>
            <span className="truncate text-[11.5px] text-muted">{section}</span>
          </>
        )}
      </div>

      <button
        type="button"
        onClick={openPalette}
        className="mx-auto flex h-[22px] w-full max-w-md items-center gap-2 rounded-[var(--radius-sm)] border border-[hsl(var(--border))] bg-[hsl(var(--bg))] px-2 text-[11.5px] text-subtle transition-colors hover:border-[hsl(var(--border-strong))] hover:text-muted"
      >
        <Search className="h-3 w-3 shrink-0" aria-hidden />
        <span className="truncate">Search videos, channels and actions</span>
        <Kbd keys="mod+k" className="ml-auto" />
      </button>

      <div className="flex w-40 shrink-0 items-center justify-end gap-0.5">
        <ShortcutsButton />
        <ThemeToggle />
      </div>
    </header>
  )
}

function ActivityBar({ pathname }: { pathname: string }) {
  return (
    <nav
      aria-label="Primary"
      className="flex w-12 shrink-0 flex-col items-center gap-1 border-r border-[hsl(var(--border))] bg-panel py-2 no-select"
    >
      {NAV.map((item) => {
        const active = pathname.startsWith(item.to)
        return (
          <Tooltip key={item.to} label={item.label} keys={item.keys} side="right">
            <Link
              to={item.to}
              aria-label={item.label}
              aria-current={active ? 'page' : undefined}
              className={cn(
                'relative flex h-9 w-9 items-center justify-center rounded-[var(--radius-sm)] transition-colors',
                active
                  ? 'bg-[hsl(var(--bg-active))] text-fg'
                  : 'text-subtle hover:bg-[hsl(var(--bg-hover))] hover:text-fg',
              )}
            >
              {active && (
                <span className="absolute left-[-6px] h-5 w-[2px] rounded-full bg-[hsl(var(--accent))]" />
              )}
              {item.icon}
            </Link>
          </Tooltip>
        )
      })}
    </nav>
  )
}

function ShortcutsButton() {
  const { openShortcuts } = useAppCommands()
  return (
    <Tooltip label="Keyboard shortcuts" keys="shift+?" side="bottom">
      <button
        type="button"
        onClick={openShortcuts}
        aria-label="Keyboard shortcuts"
        className="flex h-6 w-6 items-center justify-center rounded-[var(--radius-xs)] text-subtle transition-colors hover:bg-[hsl(var(--bg-hover))] hover:text-fg"
      >
        <Keyboard className="h-3.5 w-3.5" />
      </button>
    </Tooltip>
  )
}

function ThemeToggle() {
  const [theme, toggle] = useTheme()
  const dark = theme === 'dark'

  return (
    <Tooltip label={dark ? 'Switch to light' : 'Switch to dark'} side="bottom">
      <button
        type="button"
        onClick={toggle}
        aria-label={dark ? 'Switch to light theme' : 'Switch to dark theme'}
        className="flex h-6 w-6 items-center justify-center rounded-[var(--radius-xs)] text-subtle transition-colors hover:bg-[hsl(var(--bg-hover))] hover:text-fg"
      >
        {dark ? <Sun className="h-3.5 w-3.5" /> : <Moon className="h-3.5 w-3.5" />}
      </button>
    </Tooltip>
  )
}

/* -------------------------------------------------------------- status bar */

function StatusBar({ connection }: { connection: ConnectionState }) {
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

  const busy = status?.pools.reduce((sum, p) => sum + p.inFlight, 0) ?? 0

  return (
    <footer className="flex h-6 shrink-0 items-center gap-3 border-t border-[hsl(var(--border))] bg-panel px-3 text-[11px] text-muted no-select">
      <ConnectionPill state={connection} />
      <Divider />

      {/* Capacity is the binding constraint of the whole system (§5), so it is
          on screen at all times rather than only on the operator console. */}
      <div className="flex min-w-0 items-center gap-1 overflow-hidden">
        {status?.pools.map((pool) => (
          <PoolChip key={pool.pool} stat={pool} />
        ))}
      </div>

      <Divider className="ml-1" />
      <span className="tabular shrink-0">
        <span className="text-subtle">running</span> {busy}
      </span>
      <span className="tabular shrink-0">
        <span className="text-subtle">ready</span> {status?.ready ?? 0}
      </span>
      <span className="tabular hidden shrink-0 md:inline">
        <span className="text-subtle">videos</span> {status?.videos ?? 0}
      </span>
      {(status?.failed ?? 0) > 0 && (
        <Link
          to="/scheduler"
          className="tabular shrink-0 text-[hsl(var(--danger))] hover:underline"
        >
          {status?.failed} failed
        </Link>
      )}

      <span className="ml-auto shrink-0 tabular text-subtle">
        uptime {formatDuration(status?.uptimeSeconds ?? 0)}
      </span>
      <span className="shrink-0 text-subtle">{health?.version ?? ''}</span>
    </footer>
  )
}

const CONNECTION_LABEL: Record<ConnectionState, string> = {
  live: 'Live',
  connecting: 'Reconnecting',
  offline: 'Offline',
}

function ConnectionPill({ state }: { state: ConnectionState }) {
  return (
    <Tooltip
      label={
        state === 'live'
          ? 'Streaming updates from the daemon'
          : 'The event stream is down; EventSource will reconnect and resume'
      }
    >
      <span className="flex shrink-0 items-center gap-1.5">
        <span
          className={cn(
            'h-1.5 w-1.5 rounded-full',
            state === 'live'
              ? 'bg-[hsl(var(--success))]'
              : state === 'connecting'
                ? 'bg-[hsl(var(--warning))] pulse-live'
                : 'bg-[hsl(var(--danger))]',
          )}
        />
        <Radio className="h-3 w-3 text-subtle" aria-hidden />
        <span>{CONNECTION_LABEL[state]}</span>
      </span>
    </Tooltip>
  )
}

/* ------------------------------------------------------------ page header */

/**
 * The header for a route that owns its whole pane — Channels, Scheduler,
 * Settings. It is the same height as the detail pane's toolbar, so moving
 * between sections never shifts the content beneath it.
 */
export function PageHeader({
  title,
  subtitle,
  actions,
}: {
  title: string
  subtitle?: ReactNode
  actions?: ReactNode
}) {
  return (
    <header className="flex min-h-11 shrink-0 items-center justify-between gap-4 border-b border-[hsl(var(--border))] bg-subtle px-4 py-2">
      <div className="min-w-0">
        <h1 className="truncate text-[13.5px] font-semibold text-fg">{title}</h1>
        {subtitle && <div className="mt-0.5 truncate text-[11.5px] text-muted">{subtitle}</div>}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </header>
  )
}
