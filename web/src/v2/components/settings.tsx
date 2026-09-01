import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Search } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { create } from 'zustand'

import { api, qk } from '../core/api'
import type { Setting } from '../core/types'
import { cn } from '../core/utils'
import { backendsOf, GROUPS, labelFor } from './settings-meta'
import { Button } from './ui/button'
import { Dialog } from './ui/dialog'
import { Checkbox, Input, Select } from './ui/field'

/**
 * Settings.
 *
 * Forty-eight values in ten groups, which is a list long enough that the shape
 * of it is the whole design problem. A sidebar of groups beside one pane is
 * macOS's answer and it is the right one: the groups are a fixed, short,
 * scannable index, and only one of them is ever on screen.
 *
 * The pane draws them as *cards* — rounded containers with a hairline between
 * rows — rather than as a flat run of label-and-control pairs. That is the one
 * thing that separates a settings screen from a web form, and it also does real
 * work: a card is a boundary, so a group with two rows looks finished instead of
 * looking like the top of a longer list that failed to load.
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

export function SettingsDialog() {
  const open = useSettings((s) => s.open)
  const hide = useSettings((s) => s.hide)
  const [group, setGroup] = useState<string | null>(null)
  const [query, setQuery] = useState('')

  const settings = useQuery({ queryKey: qk.settings, queryFn: api.listSettings, enabled: open })
  const rows = useMemo(() => settings.data ?? [], [settings.data])

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
  const applies = (row: Setting) => row.backend === '' || chosen.has(row.backend)

  /*
    Groups in the order the server lists them, minus the ones with nothing in
    them right now.

    Eleven narration settings all belong to XTTS or Kokoro, so with the sample
    voice selected that whole group is empty — and an entry that leads to a
    blank pane is worse than no entry, because the blankness is only discovered
    after the click and explains nothing when it arrives.
  */
  const groups = useMemo(() => {
    const seen = new Set<string>()
    for (const row of rows) {
      if (row.backend === '' || chosen.has(row.backend)) seen.add(row.group)
    }
    return [...new Set(rows.map((row) => row.group))].filter((name) => seen.has(name))
  }, [rows, chosen])

  const searching = query.trim().length > 0
  const active = group && groups.includes(group) ? group : (groups[0] ?? null)

  /*
    Search cuts across the groups, because not knowing which group a setting is
    in is the reason to search in the first place. Matched on the written label,
    the key and the description: the key is what somebody who has read the
    server's config would type.
  */
  const found = useMemo(() => {
    const needle = query.trim().toLowerCase()
    if (!needle) return []
    return rows
      .filter((row) => row.backend === '' || chosen.has(row.backend))
      .filter((row) =>
        `${labelFor(row.key)} ${row.key} ${row.description}`.toLowerCase().includes(needle),
      )
  }, [rows, query, chosen])

  const shown = rows.filter((row) => row.group === active && applies(row))
  const hiddenBackends = backendsOf(
    rows.filter((row) => row.group === active && !applies(row)).map((row) => row.backend),
  )

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) hide()
      }}
      width={720}
      height={600}
    >
      <Dialog.Header title="Settings" />
      <Dialog.Body bare>
        <nav className="surface-band flex w-[196px] shrink-0 flex-col">
          <div className="relative shrink-0 px-2.5 pt-1 pb-2">
            <Search
              className="pointer-events-none absolute top-1/2 left-[18px] size-[13px] -translate-y-1/2 text-tertiary"
              strokeWidth={2}
            />
            <input
              type="search"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search"
              className="control h-[24px] w-full pl-[22px] text-[12px]"
            />
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto pb-2">
            {groups.map((name) => {
              const meta = GROUPS[name]
              const Icon = meta?.icon
              const selected = !searching && name === active
              return (
                <button
                  key={name}
                  type="button"
                  onClick={() => {
                    setQuery('')
                    setGroup(name)
                  }}
                  aria-current={selected}
                  className={cn(
                    'mx-2 flex w-[calc(100%-16px)] items-center gap-2 rounded-[6px] px-2 py-[5px]',
                    'text-left text-[12px] transition-colors',
                    selected ? 'text-white' : 'text-primary hover:bg-[var(--hover)]',
                  )}
                  style={selected ? { backgroundColor: 'var(--accent)' } : undefined}
                >
                  {Icon ? (
                    <Icon
                      className={cn(
                        'size-[15px] shrink-0',
                        selected ? 'text-white' : 'text-tertiary',
                      )}
                      strokeWidth={1.75}
                    />
                  ) : null}
                  <span className="min-w-0 flex-1 truncate">{meta?.title ?? name}</span>
                </button>
              )
            })}
          </div>
        </nav>

        <div className="seam-v shrink-0" />

        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
          {settings.error ? (
            <p className="text-[12px] text-[var(--failed)]">{(settings.error as Error).message}</p>
          ) : null}

          {searching ? (
            <>
              <PaneTitle>{found.length === 1 ? '1 result' : `${found.length} results`}</PaneTitle>
              {found.length > 0 ? (
                <Card>
                  {found.map((row) => (
                    <SettingRow key={row.key} setting={row} where={GROUPS[row.group]?.title} />
                  ))}
                </Card>
              ) : (
                <p className="text-[12px] text-tertiary">Nothing matches “{query.trim()}”.</p>
              )}
            </>
          ) : (
            <>
              <PaneTitle>{active ? (GROUPS[active]?.title ?? active) : 'Settings'}</PaneTitle>
              {shown.length > 0 ? (
                <Card>
                  {shown.map((row) => (
                    <SettingRow key={row.key} setting={row} />
                  ))}
                </Card>
              ) : null}
              {/* Under the card it belongs to, not in the window's footer: it is
                  about this group, and the footer is about the window. */}
              {hiddenBackends ? (
                <p className="mt-2.5 px-1 text-[11px] text-tertiary">
                  More settings appear here when {hiddenBackends} is the selected backend.
                </p>
              ) : null}
            </>
          )}
        </div>
      </Dialog.Body>
      <Dialog.Footer>
        <span className="mr-auto" />
        <Button primary className="h-[26px] px-4" onClick={hide}>
          Done
        </Button>
      </Dialog.Footer>
    </Dialog>
  )
}

/** Which group you are in, said in the pane rather than only in the sidebar. */
function PaneTitle({ children }: { children: ReactNode }) {
  return <h2 className="mb-2.5 px-1 text-[14px] font-semibold text-primary">{children}</h2>
}

/**
 * The rounded container a group's rows sit in.
 *
 * The separator belongs *between* two rows, which is why it is drawn from the
 * second child on rather than under every one: a rule under the last row is the
 * line that makes a card look like it was cut off mid-list.
 */
function Card({ children }: { children: ReactNode }) {
  return (
    <div
      className="settings-card overflow-hidden rounded-[9px]"
      style={{
        backgroundColor: 'var(--raised)',
        boxShadow: '0 0 0 0.5px var(--separator-strong)',
      }}
    >
      {children}
    </div>
  )
}

function SettingRow({ setting, where }: { setting: Setting; where?: string }) {
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

  const control = (): ReactNode => {
    if (setting.type === 'bool') {
      return (
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
      )
    }

    if (setting.options.length > 0) {
      return (
        <Select
          className="w-[180px]"
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
      )
    }

    if (setting.type === 'int' || setting.type === 'float') {
      return (
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
      )
    }

    const listId = setting.suggestions.length > 0 ? `${setting.key}-suggestions` : undefined
    return (
      <>
        <Input
          type={setting.secret ? 'password' : 'text'}
          className="w-[220px]"
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
      </>
    )
  }

  return (
    // Label leading, control trailing, description under the label — the shape
    // macOS has used since the sheets went to cards. The old right-aligned label
    // gutter put a column of ragged text down the middle of the pane and left
    // the controls to line up against nothing.
    <div className="flex items-start gap-4 px-3.5 py-2.5">
      <div className="min-w-0 flex-1">
        <div className="text-[12px] text-primary">{labelFor(setting.key)}</div>
        {where ? <div className="mt-px text-[11px] text-tertiary">{where}</div> : null}
        {setting.description ? (
          <p className="mt-1 max-w-[46ch] text-[11px] leading-snug text-tertiary">
            {setting.description}
          </p>
        ) : null}
        {save.error ? (
          <p className="mt-1 text-[11px] text-[var(--failed)]">{(save.error as Error).message}</p>
        ) : null}
      </div>
      <div className="flex shrink-0 items-center pt-[1px]">{control()}</div>
    </div>
  )
}
