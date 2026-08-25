import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState, type ReactNode } from 'react'
import { create } from 'zustand'

import { api, qk } from '../core/api'
import type { Setting } from '../core/types'
import { cn } from '../core/utils'
import { Button } from './ui/button'
import { Dialog } from './ui/dialog'
import { Checkbox, Input, Select } from './ui/field'

/**
 * Settings.
 *
 * Forty-nine values in ten groups, which is a list long enough that the shape
 * of it is the whole design problem. A sidebar of groups beside one pane is
 * macOS's answer and it is the right one: the groups are a fixed, short,
 * scannable index, and only one of them is ever on screen.
 *
 * Nothing is saved by a button. A setting commits when you leave the field, or
 * the moment you flip it, because there is no draft here worth the ceremony of
 * an Apply — the server is the only copy and every value stands alone.
 */

interface SettingsState {
  open: boolean
  show: () => void
  hide: () => void
}

const useSettings = create<SettingsState>((set) => ({
  open: false,
  show: () => set({ open: true }),
  hide: () => set({ open: false }),
}))

/** Opens Settings. Bound to ⌘, and to the gear in the status bar. */
export function openSettings(): void {
  useSettings.getState().show()
}

const TITLE: Record<string, string> = {
  pools: 'Concurrency',
  providers: 'Providers',
  writing: 'Writing',
  narration: 'Narration',
  slides: 'Slides',
  thumbnail: 'Thumbnail',
  video: 'Video',
  gates: 'Approvals',
  retries: 'Retries',
  server: 'Server',
}

export function SettingsDialog() {
  const open = useSettings((s) => s.open)
  const hide = useSettings((s) => s.hide)
  const [group, setGroup] = useState<string | null>(null)

  const settings = useQuery({ queryKey: qk.settings, queryFn: api.listSettings, enabled: open })
  const rows = useMemo(() => settings.data ?? [], [settings.data])

  // Groups in the order the server lists them, which is the order they matter
  // in — an alphabetical index would put Approvals above Concurrency for no
  // reason anyone could defend.
  const groups = useMemo(() => [...new Set(rows.map((row) => row.group))], [rows])

  /**
   * A setting bound to a backend only matters while that backend is selected.
   *
   * Derived from the provider values rather than from a table mapping concern
   * to backend: the providers *are* the selection, so reading them is exact and
   * stays exact when a new backend is added.
   */
  const chosen = useMemo(
    () => new Set(rows.filter((row) => row.key.startsWith('provider.')).map((row) => row.value)),
    [rows],
  )

  const active = group && groups.includes(group) ? group : (groups[0] ?? null)
  const shown = rows.filter(
    (row) => row.group === active && (row.backend === '' || chosen.has(row.backend)),
  )
  const hidden = rows.filter(
    (row) => row.group === active && row.backend !== '' && !chosen.has(row.backend),
  ).length

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) hide()
      }}
      title="Settings"
      width={680}
      height={560}
      flush
      footer={() => (
        <>
          {hidden > 0 ? (
            <span className="mr-auto text-[11px] text-tertiary">
              {hidden} hidden — they belong to a backend that is not selected
            </span>
          ) : (
            <span className="mr-auto" />
          )}
          <Button primary className="h-[26px] px-4" onClick={hide}>
            Done
          </Button>
        </>
      )}
    >
      {() => (
        <>
          <nav className="surface-band w-[168px] shrink-0 overflow-y-auto py-2">
            {groups.map((name) => {
              const selected = name === active
              return (
                <button
                  key={name}
                  type="button"
                  onClick={() => setGroup(name)}
                  aria-current={selected}
                  className={cn(
                    'mx-2 flex w-[calc(100%-16px)] items-center rounded-[6px] px-2.5 py-[5px]',
                    'text-left text-[12px] transition-colors',
                    selected ? 'text-white' : 'text-primary hover:bg-[var(--hover)]',
                  )}
                  style={selected ? { backgroundColor: 'var(--accent)' } : undefined}
                >
                  {TITLE[name] ?? name}
                </button>
              )
            })}
          </nav>

          <div className="seam-v shrink-0" />

          <div className="min-h-0 flex-1 overflow-y-auto px-5 py-3">
            {settings.error ? (
              <p className="text-[12px] text-[var(--failed)]">
                {(settings.error as Error).message}
              </p>
            ) : null}
            {shown.map((row) => (
              <SettingRow key={row.key} setting={row} />
            ))}
          </div>
        </>
      )}
    </Dialog>
  )
}

/** The label a key earns: its last segment, spaced out and sentence-cased. */
function labelFor(key: string): string {
  const tail = key.split('.').slice(1).join(' ').replace(/[._]/g, ' ')
  const words = (tail || key).trim()
  return words.charAt(0).toUpperCase() + words.slice(1)
}

function SettingRow({ setting }: { setting: Setting }) {
  const client = useQueryClient()
  const [draft, setDraft] = useState(setting.value)

  const save = useMutation({
    mutationFn: (value: string) => api.updateSetting(setting.key, value),
    onSuccess: () => client.invalidateQueries({ queryKey: qk.settings }),
  })

  // Committed on leaving the field rather than on every keystroke: a PUT per
  // character is a race with itself, and the last one to land wins rather than
  // the last one you typed.
  const commit = (value: string) => {
    if (value === setting.value) return
    save.mutate(value)
  }

  const label = labelFor(setting.key)
  const listId = setting.suggestions.length > 0 ? `${setting.key}-suggestions` : undefined

  if (setting.type === 'bool') {
    return (
      <Row label={label} description={setting.description} error={save.error}>
        <Checkbox
          checked={draft === 'true'}
          onChange={(next) => {
            const value = next ? 'true' : 'false'
            setDraft(value)
            commit(value)
          }}
        >
          <span className="text-secondary">{draft === 'true' ? 'On' : 'Off'}</span>
        </Checkbox>
      </Row>
    )
  }

  if (setting.options.length > 0) {
    return (
      <Row label={label} description={setting.description} error={save.error}>
        <Select
          value={draft}
          onChange={(event) => {
            setDraft(event.target.value)
            commit(event.target.value)
          }}
        >
          {setting.options.map((option) => (
            <option key={option} value={option}>
              {option}
            </option>
          ))}
        </Select>
      </Row>
    )
  }

  if (setting.type === 'int' || setting.type === 'float') {
    return (
      <Row label={label} description={setting.description} error={save.error}>
        <Input
          type="number"
          className="w-[88px] text-right tabular-nums"
          value={draft}
          min={setting.min}
          max={setting.max}
          step={setting.type === 'float' ? 0.1 : 1}
          onChange={(event) => setDraft(event.target.value)}
          onBlur={(event) => commit(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') event.currentTarget.blur()
          }}
        />
      </Row>
    )
  }

  return (
    <Row label={label} description={setting.description} error={save.error}>
      <Input
        type={setting.secret ? 'password' : 'text'}
        value={draft}
        list={listId}
        placeholder={setting.secret && setting.configured ? 'Set — type to replace' : ''}
        onChange={(event) => setDraft(event.target.value)}
        onBlur={(event) => commit(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === 'Enter') event.currentTarget.blur()
        }}
      />
      {listId ? (
        <datalist id={listId}>
          {setting.suggestions.map((suggestion) => (
            <option key={suggestion.value} value={suggestion.value} label={suggestion.label} />
          ))}
        </datalist>
      ) : null}
    </Row>
  )
}

function Row({
  label,
  description,
  error,
  children,
}: {
  label: string
  description: string
  error: unknown
  children: ReactNode
}) {
  return (
    <div className="flex gap-3 py-[5px]">
      <span className="w-[136px] shrink-0 pt-[4px] text-right text-[12px] text-secondary">
        {label}
      </span>
      <div className="min-w-0 flex-1">
        {children}
        {description ? (
          <p className="mt-1 text-[11px] leading-snug text-tertiary">{description}</p>
        ) : null}
        {error ? (
          <p className="mt-1 text-[11px] text-[var(--failed)]">{(error as Error).message}</p>
        ) : null}
      </div>
    </div>
  )
}
