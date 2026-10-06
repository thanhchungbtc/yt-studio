import { useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, ArrowRight, LoaderCircle, RefreshCw, Sparkles } from 'lucide-react'
import { useEffect, useState } from 'react'

import { qk } from '../core/api'
import type { Video } from '../core/types'
import {
  checkForUpdates,
  noteText,
  restartToUpdate,
  useUpdate,
  type UpdateState,
} from '../core/update'
import { Button } from './ui/button'
import { Popover } from './ui/popover'
import { Tooltip } from './ui/tooltip'

/** True once `on` has held for ms, so quick states never flash. */
function useAfter(on: boolean, ms: number): boolean {
  const [after, setAfter] = useState(false)
  useEffect(() => {
    setAfter(false)
    if (!on) return
    const t = setTimeout(() => setAfter(true), ms)
    return () => clearTimeout(t)
  }, [on, ms])
  return on && after
}

function Version({ children, fresh }: { children: string; fresh?: boolean }) {
  return (
    <span
      className={
        fresh
          ? 'rounded-md bg-accent-soft px-1.5 py-px font-medium text-accent-strong'
          : 'rounded-md bg-active px-1.5 py-px text-secondary'
      }
    >
      {children}
    </span>
  )
}

/** The ready version: what's new, and restarting into it. */
function UpdateDetails({ u }: { u: UpdateState }) {
  const notes = u.notes ?? []
  const [restarting, setRestarting] = useState(false)
  const running =
    useQueryClient()
      .getQueryData<Video[]>(qk.videos)
      ?.filter((v) => v.state === 'running').length ?? 0

  return (
    <div className="flex flex-col">
      <div className="flex items-start gap-3 p-3.5 pb-3">
        <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-accent-soft text-accent">
          <Sparkles className="size-4" strokeWidth={2} />
        </span>
        <div className="min-w-0 flex-1 space-y-1">
          <div className="text-[13px] font-semibold text-primary">Update ready</div>
          <div className="flex items-center gap-1.5 text-[11px] tabular-nums">
            {u.current && <Version>{u.current}</Version>}
            {u.current && <ArrowRight className="size-3 text-tertiary" />}
            <Version fresh>{u.version ?? ''}</Version>
          </div>
        </div>
      </div>
      {notes.length > 0 && (
        <div className="border-t border-line px-3.5 py-2.5">
          <div className="mb-1.5 text-[10px] font-semibold tracking-[0.06em] text-tertiary uppercase">
            What's new
          </div>
          <ul className="max-h-48 space-y-1 overflow-y-auto text-[12px] text-secondary">
            {notes.map((n, i) => (
              <li key={i} className="flex gap-2">
                <span className="mt-[7px] size-1 shrink-0 rounded-full bg-[var(--text-tertiary)]" />
                <span className="min-w-0">{noteText(n)}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
      {u.error && (
        <div className="mx-3.5 mb-1 rounded-lg bg-[color-mix(in_srgb,var(--failed)_12%,transparent)] px-2.5 py-1.5 text-[12px] text-primary">
          {u.error}
        </div>
      )}
      <div className="flex items-center gap-3 border-t border-line px-3.5 py-2.5">
        <span className="mr-auto text-[11px] text-tertiary">
          {running > 0
            ? `${running === 1 ? 'A running video picks' : `${running} running videos pick`} up where ${running === 1 ? 'it' : 'they'} left off.`
            : ''}
        </span>
        <Button
          primary
          disabled={restarting}
          onClick={() => {
            setRestarting(true)
            void restartToUpdate().finally(() => setRestarting(false))
          }}
        >
          {restarting ? (
            <LoaderCircle className="size-3.5 animate-spin" />
          ) : (
            <RefreshCw className="size-3.5" strokeWidth={2.2} />
          )}
          Restart Now
        </Button>
      </div>
    </div>
  )
}

/** Updating, in the status bar; nothing when up to date. */
export function UpdateStatus() {
  const u = useUpdate()
  const preparing = useAfter(u.state === 'preparing', 600)

  if (u.state === 'ready') {
    return (
      <Popover
        align="end"
        width={320}
        trigger={
          <button
            type="button"
            aria-label={`Restart to update to ${u.version}`}
            className="animate-slide-up mr-1 flex h-5 items-center gap-1 rounded-full bg-accent-fill pr-2 pl-1.5 text-[11px] font-medium text-accent-fg shadow-[inset_0_1px_0_rgb(255_255_255/0.18),0_1px_2px_rgb(0_0_0/0.12)] transition-colors hover:bg-accent-fill-hover"
          >
            {u.error ? (
              <AlertTriangle className="size-3" strokeWidth={2.2} />
            ) : (
              <RefreshCw className="size-3" strokeWidth={2.2} />
            )}
            Restart to Update
          </button>
        }
      >
        <UpdateDetails u={u} />
      </Popover>
    )
  }
  if (preparing) {
    return (
      <span className="animate-fade-in flex h-5 items-center gap-1 px-1.5 text-tertiary">
        <LoaderCircle className="size-3 animate-spin" />
        Preparing {u.version}…
      </span>
    )
  }
  if (u.state === 'failed') {
    return (
      <Tooltip
        content={u.error ? `${u.error} — click to try again` : 'Click to try again'}
        side="top"
      >
        <button
          type="button"
          onClick={() => void checkForUpdates()}
          className="flex h-5 items-center gap-1 rounded-md px-1.5 text-[var(--running)] transition-colors hover:bg-hover"
        >
          <AlertTriangle className="size-3" />
          Update failed
        </button>
      </Tooltip>
    )
  }
  return null
}

/** The running version and an update check, for the settings footer. */
export function VersionCheck() {
  const u = useUpdate()
  const [checking, setChecking] = useState(false)
  const [said, setSaid] = useState('')

  const check = () => {
    setChecking(true)
    setSaid('')
    void checkForUpdates()
      .then((s) => {
        switch (s.state) {
          case 'ready':
            setSaid(`${s.version} is ready — restart from the status bar.`)
            break
          case 'failed':
            setSaid(s.error ?? "Couldn't check.")
            break
          case 'disabled':
            setSaid("Development builds don't update.")
            break
          default:
            setSaid("You're up to date.")
        }
      })
      .catch(() => setSaid("Couldn't check."))
      .finally(() => setChecking(false))
  }

  return (
    <span className="mr-auto flex min-w-0 items-center gap-2 text-[11px] text-tertiary">
      <span className="tabular-nums">yt-studio {u.current || '…'}</span>
      <button
        type="button"
        disabled={checking}
        onClick={check}
        className="rounded-md px-1.5 py-0.5 text-accent transition-colors hover:bg-hover disabled:opacity-50"
      >
        {checking ? 'Checking…' : 'Check for Updates'}
      </button>
      {said && <span className="animate-fade-in truncate">{said}</span>}
    </span>
  )
}
