import { useQuery } from '@tanstack/react-query'
import { Link, Outlet, useRouterState } from '@tanstack/react-router'
import { Activity, Film, Moon, Radio, Settings as SettingsIcon, Sun, Tv } from 'lucide-react'
import type { ReactNode } from 'react'
import { useCallback, useState } from 'react'

import { api, qk } from '@/lib/api'
import { useEventStream, type ConnectionState } from '@/lib/events'
import { formatDuration } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Tooltip } from '@/components/ui/primitives'

interface NavItem {
  to: string
  label: string
  icon: ReactNode
}

const NAV: NavItem[] = [
  { to: '/videos', label: 'Videos', icon: <Film className="h-[18px] w-[18px]" /> },
  { to: '/channels', label: 'Channels', icon: <Tv className="h-[18px] w-[18px]" /> },
  { to: '/scheduler', label: 'Scheduler', icon: <Activity className="h-[18px] w-[18px]" /> },
  { to: '/settings', label: 'Settings', icon: <SettingsIcon className="h-[18px] w-[18px]" /> },
]

/**
 * The shell: an activity bar on the left and a status bar along the bottom,
 * with the route filling everything between. The layout never scrolls; only
 * the route's own panes do.
 */
export function AppShell() {
  const connection = useEventStream()
  const pathname = useRouterState({ select: (s) => s.location.pathname })

  return (
    <div className="flex h-full w-full flex-col overflow-hidden bg-app">
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

function ActivityBar({ pathname }: { pathname: string }) {
  return (
    <nav
      aria-label="Primary"
      className="flex w-12 shrink-0 flex-col items-center gap-1 border-r border-[hsl(var(--border))] bg-panel py-2"
    >
      <Link
        to="/videos"
        className="mb-2 flex h-8 w-8 items-center justify-center rounded-[var(--radius-sm)] bg-[hsl(var(--accent))] text-[hsl(var(--accent-fg))]"
        aria-label="yt-studio"
      >
        <svg viewBox="0 0 24 24" className="h-4 w-4" fill="currentColor" aria-hidden>
          <path d="M9 7v10l8-5z" />
        </svg>
      </Link>
      {NAV.map((item) => {
        const active = pathname.startsWith(item.to)
        return (
          <Tooltip key={item.to} label={item.label}>
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
      <div className="mt-auto">
        <ThemeToggle />
      </div>
    </nav>
  )
}

function ThemeToggle() {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains('dark'))

  const toggle = useCallback(() => {
    setDark((prev) => {
      const next = !prev
      document.documentElement.classList.toggle('dark', next)
      document.documentElement.classList.toggle('light', !next)
      try {
        localStorage.setItem('yt-studio.theme', next ? 'dark' : 'light')
      } catch {
        // Private browsing; the preference simply does not persist.
      }
      return next
    })
  }, [])

  return (
    <Tooltip label={dark ? 'Switch to light' : 'Switch to dark'}>
      <button
        type="button"
        onClick={toggle}
        aria-label={dark ? 'Switch to light theme' : 'Switch to dark theme'}
        className="flex h-9 w-9 items-center justify-center rounded-[var(--radius-sm)] text-subtle transition-colors hover:bg-[hsl(var(--bg-hover))] hover:text-fg"
      >
        {dark ? <Sun className="h-[17px] w-[17px]" /> : <Moon className="h-[17px] w-[17px]" />}
      </button>
    </Tooltip>
  )
}

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
    <footer className="flex h-6 shrink-0 items-center gap-4 border-t border-[hsl(var(--border))] bg-panel px-3 text-[11px] text-muted">
      <ConnectionPill state={connection} />
      <span className="tabular">
        <span className="text-subtle">running</span> {busy}
      </span>
      <span className="tabular">
        <span className="text-subtle">ready</span> {status?.ready ?? 0}
      </span>
      <span className="tabular">
        <span className="text-subtle">videos</span> {status?.videos ?? 0}
      </span>
      {(status?.failed ?? 0) > 0 && (
        <span className="tabular text-[hsl(var(--danger))]">{status?.failed} failed</span>
      )}
      <span className="ml-auto tabular text-subtle">
        uptime {formatDuration(status?.uptimeSeconds ?? 0)}
      </span>
      <span className="text-subtle">yt-studio {health?.version ?? ''}</span>
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
      <span className="flex items-center gap-1.5">
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
    <header className="flex shrink-0 items-center justify-between gap-4 border-b border-[hsl(var(--border))] bg-subtle px-4 py-2.5">
      <div className="min-w-0">
        <h1 className="truncate text-[14px] font-semibold text-fg">{title}</h1>
        {subtitle && <div className="mt-0.5 truncate text-[12px] text-muted">{subtitle}</div>}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </header>
  )
}
