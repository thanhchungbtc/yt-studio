import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, RotateCcw, Search, X } from 'lucide-react'
import { memo, useEffect, useMemo, useRef, useState } from 'react'

import { PageHeader } from '@/components/app-shell'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input, Select } from '@/components/ui/field'
import {
  EmptyState,
  ErrorNotice,
  Panel,
  PanelHeader,
  PanelTitle,
  Skeleton,
  Tooltip,
} from '@/components/ui/primitives'
import { api, qk } from '@/lib/api'
import type { Setting } from '@/lib/types'
import { cn } from '@/lib/utils'

const GROUP_TITLES: Record<string, string> = {
  pools: 'Concurrency pools',
  gates: 'Approval gates',
  providers: 'Providers',
  video: 'Video defaults',
  scheduler: 'Scheduler',
  server: 'Server',
  mock: 'Mock backends',
}

const GROUP_BLURBS: Record<string, string> = {
  pools:
    'Enforced across every video and channel. Lowering a limit takes effect as running tasks finish; a running provider call is not preemptible.',
  gates: 'Where the pipeline pauses for a human. A gate costs nothing while it is open.',
  providers:
    'Which backend serves each step. Each list holds the backends this build registered; a change applies to the next task.',
  video: 'Applied to a new video when the request leaves the field blank.',
  scheduler: 'Retry policy for transient provider failures.',
  server: 'Applied live — none of these need a restart.',
  mock: 'Shapes the mock backends so the scheduler can be exercised at realistic pacing.',
}

/**
 * The settings screen is a plain CRUD surface over the settings table, with no
 * privileged file access anywhere (§3, §9).
 *
 * The group rail on the left is what turns forty rows into a preferences
 * window: it is a table of contents that scrolls the pane rather than a filter
 * that hides the rest, so the operator never loses their place.
 */
export function SettingsRoute() {
  const settings = useQuery({ queryKey: qk.settings, queryFn: api.listSettings })
  const [query, setQuery] = useState('')
  const scrollRef = useRef<HTMLDivElement>(null)

  const groups = useMemo(() => {
    const needle = query.trim().toLowerCase()
    const map = new Map<string, Setting[]>()
    for (const setting of settings.data ?? []) {
      if (
        needle &&
        !setting.key.toLowerCase().includes(needle) &&
        !setting.description.toLowerCase().includes(needle)
      ) {
        continue
      }
      const list = map.get(setting.group)
      if (list) list.push(setting)
      else map.set(setting.group, [setting])
    }
    const order = Object.keys(GROUP_TITLES)
    return [...map.entries()].sort(
      (a, b) =>
        (order.indexOf(a[0]) + 1 || 99) - (order.indexOf(b[0]) + 1 || 99) ||
        a[0].localeCompare(b[0]),
    )
  }, [settings.data, query])

  const total = settings.data?.length ?? 0
  const shown = groups.reduce((sum, [, rows]) => sum + rows.length, 0)

  return (
    <>
      <PageHeader
        title="Settings"
        subtitle="Runtime configuration lives in the database, one row per key. Every change applies without restarting the daemon."
        actions={
          <div className="relative">
            <Search
              className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-subtle"
              aria-hidden
            />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Filter settings"
              aria-label="Filter settings"
              className="h-7 w-56 pl-7 pr-7 text-[12px]"
            />
            {query && (
              <button
                type="button"
                onClick={() => setQuery('')}
                aria-label="Clear the filter"
                className="absolute right-1.5 top-1/2 -translate-y-1/2 rounded-[var(--radius-xs)] p-0.5 text-subtle hover:bg-[hsl(var(--bg-hover))] hover:text-fg"
              >
                <X className="h-3 w-3" />
              </button>
            )}
          </div>
        }
      />

      <div ref={scrollRef} className="min-h-0 flex-1 overflow-y-auto p-4">
        {settings.isPending && (
          <div className="space-y-3">
            {Array.from({ length: 3 }, (_, i) => (
              <Skeleton key={i} className="h-40" />
            ))}
          </div>
        )}
        {settings.isError && <ErrorNotice error={settings.error} />}

        {!settings.isPending && groups.length === 0 && (
          <EmptyState
            title="Nothing matches"
            description={`None of the ${total} settings mention “${query}”.`}
            action={
              <Button variant="outline" onClick={() => setQuery('')}>
                Clear the filter
              </Button>
            }
          />
        )}

        {groups.length > 0 && (
          <div className="mx-auto grid max-w-5xl gap-5 lg:grid-cols-[168px_minmax(0,1fr)]">
            <nav aria-label="Setting groups" className="hidden lg:block">
              <div className="sticky top-0 space-y-0.5">
                <p className="mb-1.5 px-2 text-[10.5px] uppercase tracking-wider text-subtle">
                  {shown === total ? `${total} settings` : `${shown} of ${total}`}
                </p>
                {groups.map(([group, rows]) => (
                  <button
                    key={group}
                    type="button"
                    onClick={() =>
                      document
                        .getElementById(`settings-${group}`)
                        ?.scrollIntoView({ behavior: 'smooth', block: 'start' })
                    }
                    className="flex w-full items-center gap-2 rounded-[var(--radius-sm)] px-2 py-1 text-left text-[12px] text-muted transition-colors hover:bg-[hsl(var(--bg-hover))] hover:text-fg"
                  >
                    <span className="min-w-0 flex-1 truncate">{GROUP_TITLES[group] ?? group}</span>
                    <span className="tabular text-[10.5px] text-subtle">{rows.length}</span>
                  </button>
                ))}
              </div>
            </nav>

            <div className="min-w-0 space-y-3">
              {groups.map(([group, rows]) => (
                <Panel key={group} id={`settings-${group}`} className="scroll-mt-4">
                  <PanelHeader>
                    <div className="min-w-0">
                      <PanelTitle className="text-[13px] normal-case tracking-normal text-fg">
                        {GROUP_TITLES[group] ?? group}
                      </PanelTitle>
                      {GROUP_BLURBS[group] && (
                        <p className="mt-0.5 max-w-2xl text-[11.5px] text-subtle">
                          {GROUP_BLURBS[group]}
                        </p>
                      )}
                    </div>
                    <Badge tone="neutral">{rows.length}</Badge>
                  </PanelHeader>
                  <ul className="divide-y divide-[hsl(var(--border))]">
                    {rows.map((setting) => (
                      <SettingRow key={setting.key} setting={setting} />
                    ))}
                  </ul>
                </Panel>
              ))}
            </div>
          </div>
        )}
      </div>
    </>
  )
}

const SettingRow = memo(function SettingRow({ setting }: { setting: Setting }) {
  const queryClient = useQueryClient()
  const [value, setValue] = useState(setting.value)
  const [saved, setSaved] = useState(false)

  // A change made elsewhere (or by another client) wins over a stale draft.
  useEffect(() => setValue(setting.value), [setting.value])

  const save = useMutation({
    mutationFn: (next: string) => api.updateSetting(setting.key, next),
    onSuccess: (updated) => {
      queryClient.setQueryData<Setting[]>(qk.settings, (prev) =>
        prev?.map((s) => (s.key === updated.key ? updated : s)),
      )
      void queryClient.invalidateQueries({ queryKey: qk.scheduler })
      setSaved(true)
      window.setTimeout(() => setSaved(false), 1500)
    },
  })

  const dirty = value !== setting.value
  const commit = () => {
    if (dirty) save.mutate(value)
  }

  // A setting with a fixed set of values is a dropdown, whether that set comes
  // from the type (a boolean) or from the daemon (the backends it registered).
  // Anything else is free-form text.
  const choices = useMemo(() => {
    if (setting.options.length > 0) {
      return setting.options.map((option) => ({ value: option, label: option }))
    }
    if (setting.type === 'bool') {
      return [
        { value: 'true', label: 'enabled' },
        { value: 'false', label: 'disabled' },
      ]
    }
    return null
  }, [setting.options, setting.type])

  return (
    <li
      className={cn(
        'flex items-start gap-4 px-3 py-2.5 transition-colors',
        dirty && 'bg-[hsl(var(--accent)/0.05)]',
      )}
    >
      <div className="min-w-0 flex-1">
        <label
          htmlFor={`setting-${setting.key}`}
          className="font-mono text-[12px] font-medium text-fg"
        >
          {setting.key}
        </label>
        <p className="mt-0.5 text-[11.5px] text-subtle">{setting.description}</p>
        {setting.type === 'int' && setting.min !== setting.max && (
          <p className="mt-0.5 text-[11px] tabular text-subtle">
            range {setting.min}–{setting.max}
          </p>
        )}
        {save.isError && <ErrorNotice error={save.error} className="mt-1.5" />}
      </div>

      <div className="flex w-[260px] shrink-0 items-center gap-2">
        {choices ? (
          <Select
            id={`setting-${setting.key}`}
            value={value}
            onChange={(e) => {
              setValue(e.target.value)
              save.mutate(e.target.value)
            }}
          >
            {choices.map((choice) => (
              <option key={choice.value} value={choice.value}>
                {choice.label}
              </option>
            ))}
          </Select>
        ) : (
          <Input
            id={`setting-${setting.key}`}
            value={value}
            type={setting.type === 'int' ? 'number' : 'text'}
            {...(setting.type === 'int' && setting.min !== setting.max
              ? { min: setting.min, max: setting.max }
              : {})}
            onChange={(e) => setValue(e.target.value)}
            onBlur={commit}
            onKeyDown={(e) => {
              if (e.key === 'Enter') commit()
              if (e.key === 'Escape') setValue(setting.value)
            }}
            className={cn(dirty && 'border-[hsl(var(--accent))]')}
          />
        )}

        <div className="flex w-14 justify-end gap-1">
          {saved && (
            <Tooltip label="Applied">
              <Check className="h-4 w-4 text-[hsl(var(--success))]" />
            </Tooltip>
          )}
          {dirty && (
            <>
              <Tooltip label="Apply" keys="enter">
                <Button size="icon" variant="primary" onClick={commit} aria-label="Apply">
                  <Check className="h-3.5 w-3.5" />
                </Button>
              </Tooltip>
              <Tooltip label="Revert" keys="escape">
                <Button
                  size="icon"
                  variant="ghost"
                  onClick={() => setValue(setting.value)}
                  aria-label="Revert"
                >
                  <RotateCcw className="h-3.5 w-3.5" />
                </Button>
              </Tooltip>
            </>
          )}
        </div>
      </div>
    </li>
  )
})
