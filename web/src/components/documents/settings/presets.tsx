import { useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertCircle, ArrowRight, Check, Loader2, Wand2 } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'

import { Badge, Button } from '../../ui/controls'
import { ErrorNotice } from '../../ui/primitives'
import { api, qk } from '@/core/api'
import type { Preset, PresetValue, Setting } from '@/core/types'
import { cn } from '@/core/utils'

/**
 * The presets section: one click that moves every provider row at once.
 *
 * Which preset is in force is derived here rather than read from the server. A
 * stored "current preset" would start lying the moment one backend was changed
 * by hand, and this page holds both sides of the comparison already — so a
 * preset is in force exactly when every row it names already holds the value it
 * would write, and the same computation gives the diff to show before applying.
 */
export function Presets({ rows, running }: { rows: Setting[]; running: number }) {
  const queryClient = useQueryClient()
  const presets = useQuery({ queryKey: qk.presets, queryFn: api.listPresets })
  const [pending, setPending] = useState('')
  const [error, setError] = useState<unknown>()

  const current = useMemo(() => new Map(rows.map((row) => [row.key, row.value])), [rows])

  const diffs = useMemo(
    () =>
      (presets.data ?? []).map((preset) => ({
        preset,
        changes: preset.values.filter((value) => current.get(value.key) !== value.value),
      })),
    [presets.data, current],
  )

  const apply = useCallback(
    async (name: string) => {
      setPending(name)
      setError(undefined)
      try {
        const changed = await api.applyPreset(name)
        queryClient.setQueryData<Setting[]>(qk.settings, (prev) =>
          prev?.map((row) => changed.find((next) => next.key === row.key) ?? row),
        )
        void queryClient.invalidateQueries({ queryKey: qk.scheduler })
      } catch (failure) {
        setError(failure)
      } finally {
        setPending('')
      }
    },
    [queryClient],
  )

  if (diffs.length === 0) {
    return <p className="p-6 text-center text-[12px] text-subtle">This build ships no presets.</p>
  }

  return (
    <div className="p-4">
      {running > 0 && (
        <p className="mb-3 flex items-center gap-1.5 text-[11.5px] text-[hsl(var(--warning))]">
          <AlertCircle className="h-3.5 w-3.5 shrink-0" aria-hidden />
          {running} task{running === 1 ? ' is' : 's are'} running. A backend is resolved when a task
          is dispatched, so the work already in flight finishes on the old one and the rest starts
          on the new.
        </p>
      )}

      {error !== undefined && <ErrorNotice error={error} className="mb-3" />}

      <ul className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-3">
        {diffs.map(({ preset, changes }) => (
          <PresetCard
            key={preset.name}
            preset={preset}
            changes={changes}
            current={current}
            saving={pending === preset.name}
            disabled={pending !== '' && pending !== preset.name}
            onApply={() => void apply(preset.name)}
          />
        ))}
      </ul>
    </div>
  )
}

function PresetCard({
  preset,
  changes,
  current,
  saving,
  disabled,
  onApply,
}: {
  preset: Preset
  changes: PresetValue[]
  current: Map<string, string>
  saving: boolean
  disabled: boolean
  onApply: () => void
}) {
  const inForce = changes.length === 0
  return (
    <li
      className={cn(
        'flex flex-col rounded-[var(--radius-md)] border bg-[hsl(var(--bg-elevated))] p-3 elev-1',
        inForce ? 'border-[hsl(var(--accent))]' : 'border-[hsl(var(--border))]',
      )}
    >
      <div className="flex items-center gap-1.5">
        <h3 className="text-[12.5px] font-semibold text-fg">{preset.title}</h3>
        <span className="font-mono text-[11px] text-subtle">{preset.name}</span>
        {inForce && (
          <Badge tone="success" className="ml-auto shrink-0">
            <Check className="h-3 w-3" aria-hidden />
            In force
          </Badge>
        )}
      </div>

      <p className="mt-1 text-[11.5px] leading-[1.5] text-muted">{preset.description}</p>

      {/* The diff before the click: which rows move, and from what. */}
      {!inForce && (
        <ul className="mt-2 space-y-0.5">
          {changes.map((change) => (
            <li key={change.key} className="flex items-center gap-1 font-mono text-[10.5px]">
              <span className="min-w-0 flex-1 truncate text-subtle">{change.key}</span>
              <span className="shrink-0 text-subtle line-through">
                {current.get(change.key) || '—'}
              </span>
              <ArrowRight className="h-3 w-3 shrink-0 text-subtle" aria-hidden />
              <span className="shrink-0 font-medium text-fg">{change.value}</span>
            </li>
          ))}
        </ul>
      )}

      <div className="mt-2.5 flex items-center gap-2 pt-0.5">
        <Button
          variant={inForce ? 'outline' : 'primary'}
          size="sm"
          disabled={inForce || saving || disabled}
          onClick={onApply}
        >
          {saving ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden />
          ) : (
            <Wand2 className="h-3.5 w-3.5" aria-hidden />
          )}
          {inForce ? 'Applied' : `Apply ${changes.length} change${changes.length === 1 ? '' : 's'}`}
        </Button>
      </div>
    </li>
  )
}
