import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'

import { DocFrame } from '../editor/doc-frame'
import { Presets } from './settings/presets'
import { Input, Select, Switch } from '../ui/controls'
import { ErrorNotice, SearchField, Skeleton } from '../ui/primitives'
import { api, qk } from '@/core/api'
import type { Setting } from '@/core/types'
import { cn } from '@/core/utils'

/** The section that owns no settings rows, only the buttons that move them. */
const PRESETS = 'presets'

/**
 * Settings as a document rather than as a sidebar view.
 *
 * This is where the workbench deliberately departs from "two things in the
 * rail": a settings table is a wide two-column form with its own section rail,
 * and 288 pixels of sidebar would strangle it. The gear at the bottom of the
 * activity bar opens it here instead — which is also what the reference does,
 * and the reason its gear is not a view either.
 */
export function SettingsDoc({
  view,
  onView,
}: {
  view: string | undefined
  onView: (view: string) => void
}) {
  const settings = useQuery({ queryKey: qk.settings, queryFn: api.listSettings })
  const presets = useQuery({ queryKey: qk.presets, queryFn: api.listPresets })
  // Free from cache — the status bar keeps this warm — and it turns an abstract
  // switch into what that switch is currently interrupting.
  const scheduler = useQuery({ queryKey: qk.scheduler, queryFn: api.schedulerStatus })
  const [query, setQuery] = useState('')

  const rows = useMemo(() => settings.data ?? [], [settings.data])
  const hasPresets = (presets.data?.length ?? 0) > 0

  /**
   * Sections in the order the server lists their rows. That order is meaningful
   * — it is the order the pipeline reads them in — so it is kept rather than
   * sorted alphabetically.
   */
  const sections = useMemo(() => {
    const map = new Map<string, Setting[]>()
    for (const row of rows) {
      const group = row.group || 'other'
      const list = map.get(group)
      if (list) list.push(row)
      else map.set(group, [row])
    }
    const groups = [...map.entries()].map(([name, list]) => ({ name, rows: list }))
    // Owns no rows of its own; listed when the server has any presets at all.
    return hasPresets ? [{ name: PRESETS, rows: [] }, ...groups] : groups
  }, [rows, hasPresets])

  const needle = query.trim().toLowerCase()
  const matches = (row: Setting) =>
    !needle ||
    row.key.toLowerCase().includes(needle) ||
    row.description.toLowerCase().includes(needle) ||
    row.value.toLowerCase().includes(needle)

  // A remembered section can name a group this build no longer has, so the
  // fallback is the first one rather than an empty pane.
  const active = sections.find((s) => s.name === view) ?? sections[0]

  useEffect(() => {
    if (active && active.name !== view) onView(active.name)
  }, [active, onView, view])

  const visible = active?.rows.filter(matches) ?? []

  return (
    <DocFrame
      crumbs={['Settings', active?.name ?? '']}
      actions={
        <>
          <span className="tabular text-[11px] text-subtle">{rows.length} rows</span>
          <SearchField
            value={query}
            onChange={setQuery}
            placeholder="Filter settings"
            className="w-56"
          />
        </>
      }
    >
      <div className="flex h-full min-h-0">
        {/* The document's own rail. It is navigation inside one document, which
            is why it lives here and not in the primary sidebar. */}
        <nav
          aria-label="Settings sections"
          className="w-48 shrink-0 overflow-y-auto border-r border-[hsl(var(--border))] bg-subtle py-2"
        >
          {settings.isPending && (
            <div className="space-y-1 px-2">
              {Array.from({ length: 6 }, (_, i) => (
                <Skeleton key={i} className="h-6 w-full" />
              ))}
            </div>
          )}
          {sections.map((section) => {
            const hits = needle ? section.rows.filter(matches).length : 0
            return (
              <button
                key={section.name}
                type="button"
                onClick={() => onView(section.name)}
                aria-current={active?.name === section.name ? 'page' : undefined}
                className={cn(
                  'flex w-full items-center gap-2 px-3 py-1 text-left text-[12px] transition-colors',
                  active?.name === section.name
                    ? 'bg-[hsl(var(--bg-active))] font-medium text-fg'
                    : 'text-muted hover:bg-[hsl(var(--bg-hover))] hover:text-fg',
                )}
              >
                <span className="min-w-0 flex-1 truncate capitalize">
                  {section.name.replace(/[._-]/g, ' ')}
                </span>
                <span
                  className={cn(
                    'tabular shrink-0 text-[10.5px]',
                    needle && hits > 0 ? 'text-[hsl(var(--accent))]' : 'text-subtle',
                  )}
                >
                  {section.name === PRESETS
                    ? (presets.data?.length ?? 0)
                    : needle
                      ? hits
                      : section.rows.length}
                </span>
              </button>
            )
          })}
        </nav>

        <div className="min-h-0 flex-1 overflow-y-auto">
          {settings.isError && <ErrorNotice error={settings.error} className="m-4" />}
          {active?.name === PRESETS ? (
            <Presets rows={rows} running={scheduler.data?.running ?? 0} />
          ) : (
            <>
              <ul className="divide-y divide-[hsl(var(--border))]">
                {visible.map((row) => (
                  <SettingRow key={row.key} row={row} />
                ))}
              </ul>
              {active && visible.length === 0 && (
                <p className="p-6 text-center text-[12px] text-subtle">
                  Nothing in this section matches. The rail counts where else it does.
                </p>
              )}
            </>
          )}
        </div>
      </div>
    </DocFrame>
  )
}

/* --------------------------------------------------------------------- row */

/**
 * One setting. A control that carries its own value — a switch, a select —
 * commits the moment it changes; a free-text field waits for blur or Enter, so
 * a half-typed number is never sent.
 */
function SettingRow({ row }: { row: Setting }) {
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState(row.value)
  const [saved, setSaved] = useState(false)

  // Another client — or the server itself — can move a value under us.
  useEffect(() => setDraft(row.value), [row.value])

  const update = useMutation({
    mutationFn: (value: string) => api.updateSetting(row.key, value),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: qk.settings })
      setSaved(true)
      window.setTimeout(() => setSaved(false), 1200)
    },
  })

  const commit = (value: string) => {
    if (value === row.value) return
    update.mutate(value)
  }

  return (
    <li className="flex items-start gap-4 px-4 py-2.5 transition-colors hover:bg-[hsl(var(--bg-hover))]">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5">
          <code className="font-mono text-[11.5px] font-medium text-fg">{row.key}</code>
          {row.backend && (
            <span className="rounded-full bg-[hsl(var(--fg)/0.08)] px-1.5 text-[10px] leading-[15px] text-subtle">
              {row.backend}
            </span>
          )}
          {saved && <Check className="h-3 w-3 text-[hsl(var(--success))]" aria-label="Saved" />}
        </div>
        {row.description && (
          <p className="mt-0.5 text-[11.5px] leading-snug text-muted">{row.description}</p>
        )}
        {update.isError && <ErrorNotice error={update.error} className="mt-1.5" />}
      </div>

      <div className="w-64 shrink-0">
        {row.type === 'bool' ? (
          <div className="flex h-7 items-center">
            <Switch
              checked={draft === 'true'}
              onCheckedChange={(next) => {
                const value = next ? 'true' : 'false'
                setDraft(value)
                commit(value)
              }}
              aria-label={row.key}
            />
          </div>
        ) : row.options.length > 0 ? (
          <Select
            value={draft}
            aria-label={row.key}
            onChange={(event) => {
              setDraft(event.target.value)
              commit(event.target.value)
            }}
          >
            {row.options.map((option) => (
              <option key={option} value={option}>
                {option}
              </option>
            ))}
          </Select>
        ) : (
          <>
            <Input
              value={draft}
              aria-label={row.key}
              type={row.type === 'int' || row.type === 'float' ? 'number' : 'text'}
              {...(row.min !== row.max ? { min: row.min, max: row.max } : {})}
              onChange={(event) => setDraft(event.target.value)}
              onBlur={() => commit(draft)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault()
                  commit(draft)
                } else if (event.key === 'Escape') {
                  event.stopPropagation()
                  setDraft(row.value)
                }
              }}
            />
            {/* Advisory: the catalogue these come from lives on someone else's
                server, so the field still takes anything. */}
            {row.suggestions.length > 0 && (
              <div className="mt-1 flex flex-wrap gap-1">
                {row.suggestions.slice(0, 4).map((suggestion) => (
                  <button
                    key={suggestion.value}
                    type="button"
                    onClick={() => {
                      setDraft(suggestion.value)
                      commit(suggestion.value)
                    }}
                    className={cn(
                      'rounded-full border px-1.5 text-[10px] leading-[16px] transition-colors',
                      suggestion.value === draft
                        ? 'border-[hsl(var(--accent))] text-[hsl(var(--accent))]'
                        : 'border-[hsl(var(--border))] text-subtle hover:text-fg',
                    )}
                  >
                    {suggestion.label || suggestion.value}
                  </button>
                ))}
              </div>
            )}
          </>
        )}
      </div>
    </li>
  )
}
