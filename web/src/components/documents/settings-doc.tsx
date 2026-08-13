import { useQuery } from '@tanstack/react-query'
import { PlugZap, SearchX } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { DocFrame } from '../editor/doc-frame'
import { PRESETS, activeBackends, groupMeta, isDormant, settingTitle } from './settings/meta'
import { Presets } from './settings/presets'
import { SectionRail, type RailSection } from './settings/section-rail'
import { SettingRow } from './settings/setting-row'
import { Button } from '../ui/controls'
import { EmptyState, ErrorNotice, SearchField, Skeleton, Tooltip } from '../ui/primitives'
import { api, qk } from '@/core/api'
import type { Setting } from '@/core/types'
import { cn } from '@/core/utils'

/**
 * A settings table stops being readable well before a wide window runs out of
 * room: past about this, the eye has to cross a field of nothing to get from a
 * description on the left to the control it belongs to on the right. Rows are
 * capped and centred rather than stretched.
 */
const SHELL = 'mx-auto w-full max-w-[1120px]'

/**
 * Settings as a document rather than as a sidebar view.
 *
 * This is where the workbench deliberately departs from "two things in the
 * rail": a settings table is a wide two-column form with its own section rail,
 * and 288 pixels of sidebar would strangle it. The gear at the bottom of the
 * activity bar opens it here instead — which is also what the reference does,
 * and the reason its gear is not a view either.
 *
 * Filtering switches the body from one section to a result list across all of
 * them. A filter that only searched the section already open would be a filter
 * that answers "where is that setting" with "not here", which is the one
 * question a forty-row table is opened to ask.
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
  const presetCount = presets.data?.length ?? 0

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
    return presetCount > 0 ? [{ name: PRESETS, rows: [] as Setting[] }, ...groups] : groups
  }, [rows, presetCount])

  const needle = query.trim().toLowerCase()
  const filtering = needle !== ''

  const matches = useCallback(
    (row: Setting) =>
      !needle ||
      row.key.toLowerCase().includes(needle) ||
      settingTitle(row).toLowerCase().includes(needle) ||
      row.description.toLowerCase().includes(needle) ||
      row.backend.toLowerCase().includes(needle) ||
      groupMeta(row.group).title.toLowerCase().includes(needle) ||
      (!row.secret && row.value.toLowerCase().includes(needle)),
    [needle],
  )

  // A remembered section can name a group this build no longer has, so the
  // fallback is the first one rather than an empty pane.
  const active = sections.find((s) => s.name === view) ?? sections[0]

  useEffect(() => {
    if (active && active.name !== view) onView(active.name)
  }, [active, onView, view])

  /** Every section that still has something once the filter is applied. */
  const results = useMemo(
    () =>
      filtering
        ? sections
            .map((section) => ({ name: section.name, rows: section.rows.filter(matches) }))
            .filter((section) => section.rows.length > 0)
        : [],
    [filtering, matches, sections],
  )

  const hitCount = results.reduce((total, section) => total + section.rows.length, 0)

  const rail: RailSection[] = sections.map((section) => ({
    name: section.name,
    total: section.name === PRESETS ? presetCount : section.rows.length,
    hits: section.rows.filter(matches).length,
  }))

  // Clicking a section while filtering scrolls to it rather than dropping the
  // filter: the rail is showing where the matches are, so it should go there.
  const anchors = useRef(new Map<string, HTMLElement>())
  const select = (name: string) => {
    onView(name)
    if (filtering) anchors.current.get(name)?.scrollIntoView({ block: 'start' })
  }

  const backends = useMemo(() => activeBackends(rows), [rows])
  const dormant = active ? active.rows.filter((row) => isDormant(row, backends)).length : 0
  // Which idle backends this section is waiting on, so the notice can name them
  // and the operator can go and select one.
  const idle = active
    ? [...new Set(active.rows.filter((row) => isDormant(row, backends)).map((row) => row.backend))]
    : []
  const hasProviders = sections.some((section) => section.name === 'providers')
  const meta = groupMeta(active?.name ?? '')
  const SectionIcon = meta.icon

  return (
    <DocFrame
      crumbs={['Settings', filtering ? `“${query.trim()}”` : meta.title]}
      actions={
        <>
          <span className="tabular text-[11px] text-subtle">{rows.length} settings</span>
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
        <SectionRail
          sections={rail}
          active={active?.name ?? ''}
          filtering={filtering}
          loading={settings.isPending}
          onSelect={select}
        />

        <div className="min-h-0 flex-1 overflow-y-auto">
          {settings.isError && <ErrorNotice error={settings.error} className="m-5" />}
          {settings.isPending && <LoadingRows />}

          {filtering ? (
            <>
              <div className="sticky top-0 z-20 border-b border-[hsl(var(--border))] bg-[hsl(var(--bg)/0.85)] backdrop-blur">
                <div className={cn(SHELL, 'flex h-9 items-center gap-2 px-5')}>
                  <p className="min-w-0 flex-1 truncate text-[11.5px] text-muted">
                    <span className="tabular font-medium text-fg">{hitCount}</span>
                    {hitCount === 1 ? ' setting matches ' : ' settings match '}
                    <span className="font-medium text-fg">“{query.trim()}”</span>
                    {results.length > 0 && (
                      <span className="text-subtle">
                        {' '}
                        in {results.length} section{results.length === 1 ? '' : 's'}
                      </span>
                    )}
                  </p>
                  <Button variant="ghost" size="xs" onClick={() => setQuery('')}>
                    Clear
                  </Button>
                </div>
              </div>

              {results.map((section) => {
                const sectionMeta = groupMeta(section.name)
                const Icon = sectionMeta.icon
                return (
                  <section
                    key={section.name}
                    ref={(node) => {
                      if (node) anchors.current.set(section.name, node)
                      else anchors.current.delete(section.name)
                    }}
                  >
                    <h2 className="sticky top-9 z-10 border-y border-[hsl(var(--border))] bg-subtle text-[10.5px] font-semibold uppercase tracking-[0.08em] text-subtle">
                      <span className={cn(SHELL, 'flex items-center gap-2 px-5 py-1.5')}>
                        <Icon className="h-3 w-3" aria-hidden />
                        {sectionMeta.title}
                        <span className="tabular ml-auto font-normal">{section.rows.length}</span>
                      </span>
                    </h2>
                    <ul className={cn(SHELL, 'divide-y divide-[hsl(var(--border))]')}>
                      {section.rows.map((row) => (
                        <SettingRow key={row.key} row={row} dormant={isDormant(row, backends)} />
                      ))}
                    </ul>
                  </section>
                )
              })}

              {hitCount === 0 && !settings.isPending && (
                <EmptyState
                  icon={<SearchX />}
                  title="Nothing matches that"
                  description="The filter reads a setting's name, its key, its description and its value."
                  action={
                    <Button size="sm" onClick={() => setQuery('')}>
                      Clear the filter
                    </Button>
                  }
                />
              )}
            </>
          ) : (
            active && (
              <>
                <header className="sticky top-0 z-10 border-b border-[hsl(var(--border))] bg-[hsl(var(--bg)/0.85)] backdrop-blur">
                  <div className={cn(SHELL, 'flex items-start gap-3 px-5 py-3.5')}>
                    <span
                      aria-hidden
                      className="mt-[1px] flex h-7 w-7 shrink-0 items-center justify-center rounded-[var(--radius-md)] bg-[hsl(var(--accent)/0.1)] text-[hsl(var(--accent))]"
                    >
                      <SectionIcon className="h-4 w-4" />
                    </span>
                    <div className="min-w-0 flex-1">
                      <h2 className="text-[13.5px] font-semibold leading-5 text-fg">
                        {meta.title}
                      </h2>
                      {meta.blurb && (
                        <p className="mt-0.5 max-w-prose text-[11.5px] leading-[1.55] text-muted">
                          {meta.blurb}
                        </p>
                      )}
                    </div>
                    {active.name !== PRESETS && (
                      <p className="tabular flex shrink-0 items-center gap-1.5 pt-0.5 text-[11px] text-subtle">
                        <span>
                          {active.rows.length} setting{active.rows.length === 1 ? '' : 's'}
                        </span>
                        {dormant > 0 && (
                          <Tooltip
                            label={`${dormant} of these belong to a backend no provider is set to. They still save — nothing reads them until that backend is selected.`}
                          >
                            <span className="cursor-default rounded-full bg-[hsl(var(--fg)/0.07)] px-1.5 leading-[16px]">
                              {dormant} idle
                            </span>
                          </Tooltip>
                        )}
                      </p>
                    )}
                  </div>
                </header>

                {/* Said once here rather than on every row: a whole section of
                    "idle" badges is noise, and this one can offer the fix. */}
                {idle.length > 0 && (
                  <div className={cn(SHELL, 'px-5 pt-3.5')}>
                    <div className="flex items-center gap-2 rounded-[var(--radius-sm)] border border-[hsl(var(--border))] bg-subtle px-3 py-1.5 text-[11px] leading-[1.5] text-muted">
                      <PlugZap className="h-3.5 w-3.5 shrink-0 text-subtle" aria-hidden />
                      <p className="min-w-0 flex-1">
                        Nothing reads{' '}
                        {dormant === active.rows.length ? 'these' : `${dormant} of these`}: no
                        provider is set to{' '}
                        <span className="font-medium text-fg">{idle.join(' or ')}</span>. They still
                        save.
                      </p>
                      {hasProviders && (
                        <Button
                          variant="outline"
                          size="xs"
                          className="shrink-0"
                          onClick={() => select('providers')}
                        >
                          Providers
                        </Button>
                      )}
                    </div>
                  </div>
                )}

                {active.name === PRESETS ? (
                  <Presets rows={rows} running={scheduler.data?.running ?? 0} />
                ) : (
                  <ul className={cn(SHELL, 'divide-y divide-[hsl(var(--border))]')}>
                    {active.rows.map((row) => (
                      <SettingRow key={row.key} row={row} dormant={isDormant(row, backends)} />
                    ))}
                  </ul>
                )}
              </>
            )
          )}
        </div>
      </div>
    </DocFrame>
  )
}

/** The shape of the table before it arrives, so the pane does not flash empty. */
function LoadingRows() {
  return (
    <div className={cn(SHELL, 'divide-y divide-[hsl(var(--border))]')}>
      {Array.from({ length: 6 }, (_, i) => (
        <div key={i} className="grid grid-cols-[minmax(0,1fr)_268px] gap-8 px-5 py-3.5">
          <div className="space-y-1.5">
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-3 w-full max-w-md" />
            <Skeleton className="h-3 w-28" />
          </div>
          <Skeleton className="h-8 w-full" />
        </div>
      ))}
    </div>
  )
}
