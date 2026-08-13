import { useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, ArrowRight, Check, Loader2, Wand2 } from 'lucide-react'
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
    <div className="mx-auto w-full max-w-[1120px] p-5">
      {running > 0 && (
        <p className="mb-3 flex items-start gap-2 rounded-[var(--radius-md)] border border-[hsl(var(--warning)/0.35)] bg-[hsl(var(--warning-soft))] px-3 py-2 text-[11.5px] leading-[1.55] text-[hsl(var(--warning))]">
          <AlertTriangle className="mt-[2px] h-3.5 w-3.5 shrink-0" aria-hidden />
          <span>
            {running} task{running === 1 ? ' is' : 's are'} running. A backend is resolved when a
            task is dispatched, so the work already in flight finishes on the old one and the rest
            starts on the new.
          </span>
        </p>
      )}

      {error !== undefined && <ErrorNotice error={error} className="mb-3" />}

      <ul className="grid gap-3 lg:grid-cols-2 2xl:grid-cols-3">
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
        'flex flex-col rounded-[var(--radius-md)] border bg-[hsl(var(--bg-elevated))] transition-shadow elev-1 hover:elev-2',
        inForce
          ? 'border-[hsl(var(--accent))] ring-1 ring-[hsl(var(--accent)/0.25)]'
          : 'border-[hsl(var(--border))]',
      )}
    >
      <div className="flex items-start gap-2 px-3.5 pb-2 pt-3">
        <div className="min-w-0 flex-1">
          <h3 className="truncate text-[12.5px] font-semibold leading-5 text-fg">{preset.title}</h3>
          <code className="font-mono text-[10.5px] text-subtle">{preset.name}</code>
        </div>
        {inForce && (
          <Badge tone="success" className="shrink-0 px-1.5 text-[10px] leading-[15px]">
            <Check className="h-2.5 w-2.5" aria-hidden />
            In force
          </Badge>
        )}
      </div>

      <p className="px-3.5 text-[11.5px] leading-[1.55] text-muted">{preset.description}</p>

      {/* The diff before the click: which rows move, and from what. */}
      {!inForce && (
        <div className="mx-3.5 mt-2.5 rounded-[var(--radius-sm)] border border-[hsl(var(--border))] bg-subtle px-2 py-1.5">
          <ul className="space-y-1">
            {changes.map((change) => (
              <li key={change.key} className="flex items-center gap-1.5 font-mono text-[10.5px]">
                <span className="min-w-0 flex-1 truncate text-subtle" title={change.key}>
                  {change.key}
                </span>
                <span className="shrink-0 text-subtle line-through">
                  {current.get(change.key) || '—'}
                </span>
                <ArrowRight className="h-3 w-3 shrink-0 text-subtle" aria-hidden />
                <span className="shrink-0 font-medium text-fg">{change.value}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="mt-auto px-3.5 pb-3.5 pt-3">
        <Button
          variant={inForce ? 'outline' : 'primary'}
          size="sm"
          className="w-full justify-center"
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
