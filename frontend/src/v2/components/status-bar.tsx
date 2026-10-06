import { useQuery } from '@tanstack/react-query'
import {
  Command,
  Moon,
  PanelBottom,
  PanelLeft,
  PanelRight,
  Sun,
  type LucideIcon,
} from 'lucide-react'
import type { ReactNode } from 'react'

import { executeCommand } from '@/kit/commands/registry'
import { cn } from '@/kit/lib/cn'
import { Tooltip } from '@/kit/ui/Tooltip'

import { api, qk } from '../core/api'
import { useScheduler } from '../core/events'
import { toggleTheme, useResolvedTheme } from '../core/theme'
import type { PoolStat } from '../core/types'
import { useWorkbench } from '../store/workbench'
import { openDoc } from './editor/dock'
import { UpdateStatus } from './update-status'

function Item({
  children,
  onClick,
  title,
  command,
}: {
  children: ReactNode
  onClick?: () => void
  title?: string
  command?: string
}) {
  const el = (
    <button
      onClick={onClick}
      className="flex h-5 items-center gap-1 rounded-md px-1.5 transition-colors hover:bg-hover hover:text-fg"
    >
      {children}
    </button>
  )
  return title ? (
    <Tooltip content={title} command={command} side="top">
      {el}
    </Tooltip>
  ) : (
    el
  )
}

function Toggle({
  icon: Icon,
  label,
  command,
  active,
}: {
  icon: LucideIcon
  label: string
  command: string
  active?: boolean
}) {
  return (
    <Tooltip content={label} command={command} side="top">
      <button
        aria-label={label}
        aria-pressed={active}
        onClick={() => void executeCommand(command)}
        className={cn(
          'flex size-6 items-center justify-center rounded-md transition-colors hover:bg-hover hover:text-fg',
          active ? 'text-fg' : 'text-fg-faint',
        )}
      >
        <Icon className="size-3.5" strokeWidth={1.9} />
      </button>
    </Tooltip>
  )
}

function ThemeSwitch() {
  const dark = useResolvedTheme((s) => s.theme) === 'dark'
  return (
    <Tooltip
      content={dark ? 'Switch to light mode' : 'Switch to dark mode'}
      command="workbench.toggleTheme"
      side="top"
    >
      <button
        role="switch"
        aria-checked={dark}
        aria-label="Dark mode"
        onClick={toggleTheme}
        className="relative flex h-5 w-9 items-center rounded-full bg-well p-0.5 shadow-[inset_0_0_0_0.5px_var(--line-strong)] transition-colors hover:bg-active"
      >
        <span
          className={cn(
            'flex size-4 items-center justify-center rounded-full bg-content shadow-[0_1px_2px_rgb(0_0_0/0.18)] transition-transform duration-200 ease-out',
            dark && 'translate-x-4',
          )}
        >
          {dark ? (
            <Moon className="size-2.5 text-fg" strokeWidth={2.4} />
          ) : (
            <Sun className="size-2.5 text-warning" strokeWidth={2.4} />
          )}
        </span>
      </button>
    </Tooltip>
  )
}

function Divider() {
  return <span className="mx-1 h-3 w-px bg-line-strong" />
}

/** One pool: idle grey, working accent, full amber; the queue beside it. */
function Meter({ pool }: { pool: PoolStat }) {
  const full = pool.limit > 0 && pool.inFlight >= pool.limit
  const share = pool.limit > 0 ? Math.min(1, pool.inFlight / pool.limit) : 0
  const tone = pool.inFlight === 0 ? 'var(--fg-faint)' : full ? 'var(--running)' : 'var(--accent)'
  return (
    <Tooltip
      content={`${pool.pool}: ${pool.inFlight} of ${pool.limit} running${pool.queued ? `, ${pool.queued} queued` : ''}`}
      side="top"
    >
      <span className="flex h-5 shrink-0 items-center gap-1.5 rounded-md px-1.5">
        <span className="uppercase">{pool.pool}</span>
        <span className="block h-[3px] w-4 overflow-hidden rounded-full bg-active">
          <span
            className="block h-full rounded-full transition-[width,background-color] duration-200 ease-out"
            style={{ width: `${share * 100}%`, backgroundColor: tone }}
          />
        </span>
        <span className="tabular-nums">
          <span style={{ color: pool.inFlight ? tone : undefined }}>{pool.inFlight}</span>/
          {pool.limit}
          {pool.queued > 0 && <span className="text-fg-faint"> +{pool.queued}</span>}
        </span>
      </span>
    </Tooltip>
  )
}

export function StatusBar() {
  const scheduler = useScheduler()
  const primary = useWorkbench((s) => s.primaryVisible)
  const bottom = useWorkbench((s) => s.bottomVisible)
  const secondary = useWorkbench((s) => s.secondaryVisible)
  const videos = useQuery({ queryKey: qk.videos, queryFn: api.listVideos })
  const running = (videos.data ?? []).filter((v) => v.state === 'running').length
  const waiting = (videos.data ?? []).filter((v) => v.state === 'awaiting_approval')

  return (
    <div className="drag-region flex h-7 shrink-0 items-center gap-0.5 px-2.5 text-2xs text-fg-subtle">
      <div className="flex min-w-0 items-center gap-0.5 overflow-hidden">
        {scheduler?.pools.map((pool) => (
          <Meter key={pool.pool} pool={pool} />
        ))}
      </div>
      <div className="flex-1" />
      <div className="flex shrink-0 items-center gap-0.5">
        {waiting.length > 0 && (
          <Item
            title="Videos waiting for your approval"
            onClick={() => {
              const v = waiting[0]!
              openDoc({ kind: 'video', ref: v.ref }, v.title || 'Untitled')
            }}
          >
            <span className="size-1.5 rounded-full bg-warning" />
            <span className="text-warning">
              {waiting.length} {waiting.length === 1 ? 'needs' : 'need'} you
            </span>
          </Item>
        )}
        {running > 0 && (
          <Item title="Videos in the pipeline">
            <span className="size-1.5 animate-pulse rounded-full bg-accent" />
            {running} running
          </Item>
        )}
        {(waiting.length > 0 || running > 0) && <Divider />}
        <UpdateStatus />
        <Item
          onClick={() => void executeCommand('workbench.commandPalette')}
          title="Command palette"
          command="workbench.commandPalette"
        >
          <Command className="size-3" />
        </Item>
        <Divider />
        <Toggle
          icon={PanelLeft}
          label="Toggle Library"
          command="workbench.togglePrimarySidebar"
          active={primary}
        />
        <Toggle
          icon={PanelBottom}
          label="Toggle Console"
          command="workbench.togglePanel"
          active={bottom}
        />
        <Toggle
          icon={PanelRight}
          label="Toggle Inspector"
          command="workbench.toggleSecondarySidebar"
          active={secondary}
        />
        <Divider />
        <ThemeSwitch />
      </div>
    </div>
  )
}
