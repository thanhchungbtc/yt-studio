import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, RotateCcw } from 'lucide-react'
import { memo, useEffect, useMemo, useState } from 'react'

import { PageHeader } from '@/components/app-shell'
import { Button } from '@/components/ui/button'
import { Input, Select } from '@/components/ui/field'
import {
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
  providers: 'Which backend serves each step. Only the mocks are wired up in this version.',
  video: 'Applied to a new video when the request leaves the field blank.',
  scheduler: 'Retry policy for transient provider failures.',
  server: 'Applied live — none of these need a restart.',
  mock: 'Shapes the mock backends so the scheduler can be exercised at realistic pacing.',
}

/**
 * The settings screen is a plain CRUD surface over the settings table, with no
 * privileged file access anywhere (§3, §9).
 */
export function SettingsRoute() {
  const settings = useQuery({ queryKey: qk.settings, queryFn: api.listSettings })

  const groups = useMemo(() => {
    const map = new Map<string, Setting[]>()
    for (const setting of settings.data ?? []) {
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
  }, [settings.data])

  return (
    <>
      <PageHeader
        title="Settings"
        subtitle="Runtime configuration lives in the database, one row per key. Every change applies without restarting the daemon."
      />

      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        {settings.isPending && (
          <div className="space-y-3">
            {Array.from({ length: 3 }, (_, i) => (
              <Skeleton key={i} className="h-40" />
            ))}
          </div>
        )}
        {settings.isError && <ErrorNotice error={settings.error} />}

        <div className="mx-auto max-w-4xl space-y-3">
          {groups.map(([group, rows]) => (
            <Panel key={group}>
              <PanelHeader>
                <div>
                  <PanelTitle className="normal-case tracking-normal text-[13px] text-fg">
                    {GROUP_TITLES[group] ?? group}
                  </PanelTitle>
                  {GROUP_BLURBS[group] && (
                    <p className="mt-0.5 max-w-2xl text-[11.5px] text-subtle">
                      {GROUP_BLURBS[group]}
                    </p>
                  )}
                </div>
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

  return (
    <li className="flex items-start gap-4 px-3 py-2.5">
      <div className="min-w-0 flex-1">
        <label
          htmlFor={`setting-${setting.key}`}
          className="font-mono text-[12px] font-medium text-fg"
        >
          {setting.key}
        </label>
        <p className="mt-0.5 text-[11.5px] text-subtle">{setting.description}</p>
        {save.isError && <ErrorNotice error={save.error} className="mt-1.5" />}
      </div>

      <div className="flex w-[260px] shrink-0 items-center gap-2">
        {setting.type === 'bool' ? (
          <Select
            id={`setting-${setting.key}`}
            value={value}
            onChange={(e) => {
              setValue(e.target.value)
              save.mutate(e.target.value)
            }}
          >
            <option value="true">enabled</option>
            <option value="false">disabled</option>
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
              <Tooltip label="Apply">
                <Button size="icon" variant="primary" onClick={commit} aria-label="Apply">
                  <Check className="h-3.5 w-3.5" />
                </Button>
              </Tooltip>
              <Tooltip label="Revert">
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
