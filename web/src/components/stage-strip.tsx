import { memo, useMemo } from 'react'

import { Tooltip } from '@/components/ui/primitives'
import { taskLabel } from '@/lib/format'
import type { Task, TaskKind } from '@/lib/types'
import { cn } from '@/lib/utils'

/**
 * The pipeline, as the scheduler actually runs it.
 *
 * The DAG has no stage barriers — chapter 3's narration can be composing while
 * chapter 40's script is still being written — so this is emphatically not a
 * progress wizard. It is a census: for each kind of work, how much of it is
 * done, running, or has failed. That is the question an operator has when a
 * three-hour render is forty minutes in, and the answer is not a percentage.
 */
const ORDER: TaskKind[] = [
  'blueprint',
  'prime_image_prompts',
  'image_prompts',
  'script',
  'tts',
  'image',
  'clip',
  'concat',
  'metadata',
  'upload',
]

interface Stage {
  kind: TaskKind
  total: number
  succeeded: number
  running: number
  failed: number
  gated: number
  stale: number
}

export const StageStrip = memo(function StageStrip({ tasks }: { tasks: Task[] }) {
  const stages = useMemo(() => summarise(tasks), [tasks])
  if (stages.length === 0) return null

  return (
    <ol className="flex flex-wrap gap-1.5" aria-label="Pipeline stages">
      {stages.map((stage) => {
        const done = stage.total > 0 ? stage.succeeded / stage.total : 0
        const complete = stage.succeeded === stage.total
        return (
          <Tooltip
            key={stage.kind}
            label={
              <span className="block space-y-0.5">
                <span className="block font-medium">{taskLabel(stage.kind)}</span>
                <span className="block tabular">
                  {stage.succeeded} of {stage.total} done
                  {stage.running > 0 ? `, ${stage.running} running` : ''}
                  {stage.gated > 0 ? `, ${stage.gated} gated` : ''}
                  {stage.failed > 0 ? `, ${stage.failed} failed` : ''}
                  {stage.stale > 0 ? `, ${stage.stale} stale` : ''}
                </span>
              </span>
            }
          >
            <li
              className={cn(
                'relative min-w-[92px] flex-1 overflow-hidden rounded-[var(--radius-sm)] border px-2 py-1.5 transition-colors',
                stage.failed > 0
                  ? 'border-[hsl(var(--danger)/0.4)] bg-[hsl(var(--danger-soft))]'
                  : stage.stale > 0 || stage.gated > 0
                    ? 'border-[hsl(var(--warning)/0.4)] bg-[hsl(var(--warning-soft))]'
                    : complete
                      ? 'border-[hsl(var(--success)/0.3)] bg-[hsl(var(--success-soft))]'
                      : 'border-[hsl(var(--border))] bg-[hsl(var(--bg-elevated))]',
              )}
            >
              {/* The fill sits behind the label rather than beside it, so the
                  strip stays legible at ten stages on a narrow pane. */}
              <span
                aria-hidden
                className={cn(
                  'absolute inset-y-0 left-0 transition-[width] duration-500',
                  stage.running > 0 && 'stripes',
                  complete ? 'bg-[hsl(var(--success)/0.16)]' : 'bg-[hsl(var(--accent)/0.14)]',
                )}
                style={{ width: `${done * 100}%` }}
              />
              <span className="relative flex items-baseline justify-between gap-2">
                <span className="truncate text-[11px] font-medium text-fg">
                  {taskLabel(stage.kind)}
                </span>
                <span className="tabular shrink-0 text-[10.5px] text-muted">
                  {stage.total > 1 ? `${stage.succeeded}/${stage.total}` : complete ? '✓' : '—'}
                </span>
              </span>
              {stage.running > 0 && (
                <span className="relative mt-0.5 block text-[10px] text-[hsl(var(--accent))]">
                  {stage.running} running
                </span>
              )}
              {stage.failed > 0 && (
                <span className="relative mt-0.5 block text-[10px] text-[hsl(var(--danger))]">
                  {stage.failed} failed
                </span>
              )}
              {stage.failed === 0 && stage.stale > 0 && (
                <span className="relative mt-0.5 block text-[10px] text-[hsl(var(--warning))]">
                  {stage.stale} stale
                </span>
              )}
              {stage.failed === 0 && stage.stale === 0 && stage.gated > 0 && (
                <span className="relative mt-0.5 block text-[10px] text-[hsl(var(--warning))]">
                  awaiting approval
                </span>
              )}
            </li>
          </Tooltip>
        )
      })}
    </ol>
  )
})

function summarise(tasks: Task[]): Stage[] {
  const byKind = new Map<TaskKind, Stage>()
  for (const task of tasks) {
    const stage = byKind.get(task.kind) ?? {
      kind: task.kind,
      total: 0,
      succeeded: 0,
      running: 0,
      failed: 0,
      gated: 0,
      stale: 0,
    }
    stage.total += 1
    if (task.stale) stage.stale += 1
    if (task.state === 'succeeded') stage.succeeded += 1
    else if (task.state === 'running') stage.running += 1
    else if (task.state === 'failed') stage.failed += 1
    else if (task.state === 'awaiting_approval') stage.gated += 1
    byKind.set(task.kind, stage)
  }
  return ORDER.filter((kind) => byKind.has(kind)).map((kind) => byKind.get(kind) as Stage)
}
