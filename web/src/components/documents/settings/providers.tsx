import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'

import { backendMeta, portMeta } from './meta'
import { Segmented } from '../../ui/controls'
import { ErrorNotice } from '../../ui/primitives'
import { api, qk } from '@/core/api'
import type { Setting } from '@/core/types'

/**
 * The providers section: who does each job.
 *
 * It is the one section that was actively worse as a table. Every other group
 * asks for a number or a string, where a row of prose beside a control is the
 * right shape; this one asks the operator to *choose between things*, and a
 * dropdown is the control that hides exactly what a choice needs — the options
 * you did not pick.
 *
 * So it keeps that shape and drops the dropdown: the port and what it does on
 * the left, every backend registered for it laid out on the right. A name is
 * all each one needs to carry. What a backend *is* still matters, but it is a
 * thing you want once — on the way to a decision — and not a thing that should
 * cost seven paragraphs of standing furniture, so it waits in the title.
 */
export function Providers({ rows }: { rows: Setting[] }) {
  const ports = rows.filter((row) => row.key.startsWith('provider.') && row.options.length > 0)

  if (ports.length === 0) {
    return <p className="p-6 text-center text-[12px] text-subtle">This build registers no ports.</p>
  }

  return (
    <div className="mx-auto w-full max-w-[1120px] space-y-2 p-5">
      {ports.map((port) => (
        <PortCard key={port.key} port={port} />
      ))}
    </div>
  )
}

/** One port, and every backend registered for it. */
function PortCard({ port }: { port: Setting }) {
  const queryClient = useQueryClient()
  const meta = portMeta(port.key)
  const Icon = meta.icon

  const update = useMutation({
    mutationFn: (value: string) => api.updateSetting(port.key, value),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: qk.settings }),
  })

  // The optimistic value, so the control answers the click rather than the
  // round trip. A failed write puts it back, because the query is the source.
  const selected = update.isPending ? (update.variables ?? port.value) : port.value
  return (
    <section className="rounded-[var(--radius-md)] bg-[hsl(var(--bg-elevated))] elev-1">
      <div className="flex items-center gap-3 px-3.5 py-2.5">
        <span
          aria-hidden
          className="flex h-6 w-6 shrink-0 items-center justify-center rounded-[var(--radius-sm)] bg-[hsl(var(--accent)/0.1)] text-[hsl(var(--accent))]"
        >
          <Icon className="h-3.5 w-3.5" />
        </span>

        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2">
            <h3 className="text-[13px] font-semibold leading-5 text-fg">{meta.title}</h3>
            <code className="font-mono text-[10.5px] text-subtle">{port.key}</code>
            {update.isPending && (
              <Loader2 className="h-3 w-3 animate-spin text-subtle" aria-hidden />
            )}
          </div>
          {port.description && (
            <p className="truncate text-[11.5px] leading-[1.55] text-muted">{port.description}</p>
          )}
        </div>

        <Segmented
          aria-label={port.key}
          className="shrink-0"
          value={selected}
          // A registry with one entry is not a choice, and offering it as one
          // invites a click that can only land where it already is.
          disabled={port.options.length === 1}
          onChange={(next) => update.mutate(next)}
          options={port.options.map((name) => ({
            value: name,
            label: backendMeta(name).title,
            // The sentence the option cards used to carry. It is worth one
            // hover, and not one line each.
            title: backendMeta(name).blurb || undefined,
          }))}
        />
      </div>

      {update.isError && (
        <div className="px-3.5 pb-3">
          <ErrorNotice error={update.error} />
        </div>
      )}
    </section>
  )
}
