import { Eye, RefreshCw } from 'lucide-react'
import { memo, useMemo, useState } from 'react'

import { useAssetViewer } from '@/components/asset-viewer'
import { RerunDialog } from '@/components/stale'
import { ContextMenu, ContextMenuItem, ContextMenuLabel } from '@/components/ui/menu'
import { Tooltip } from '@/components/ui/primitives'
import type { ViewerItem } from '@/lib/assets'
import { taskLabel } from '@/lib/format'
import type { Task, TaskKind } from '@/lib/types'
import { cn } from '@/lib/utils'

/**
 * The pipeline, as the scheduler actually runs it. The DAG has no stage barriers
 * — chapter 3 can be composing while chapter 40's script is written — so this is
 * a census rather than a wizard: per kind of work, how much is done, running or
 * failed.
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
  'thumbnail_plan',
  'thumbnail_icon',
  'thumbnail',
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
  /** Every task of this kind, which is what re-running the stage seeds. */
  ids: string[]
  /** Nothing here has run yet, so there is nothing to run again. */
  pending: boolean
}

export const StageStrip = memo(function StageStrip({
  tasks,
  videoRef,
  videoId,
  artifacts,
}: {
  tasks: Task[]
  videoRef: string
  videoId: string
  /** What each stage produced, for the viewer the context menu opens. */
  artifacts?: Map<TaskKind, ViewerItem[]>
}) {
  const stages = useMemo(() => summarise(tasks), [tasks])
  const [rerunning, setRerunning] = useState<Stage | null>(null)
  const openViewer = useAssetViewer()
  if (stages.length === 0) return null

  return (
    <>
      <ol className="flex flex-wrap gap-1.5" aria-label="Pipeline stages">
        {stages.map((stage) => {
          const done = stage.total > 0 ? stage.succeeded / stage.total : 0
          const complete = stage.succeeded === stage.total
          // The blueprint is the one stage that cannot be re-run: the whole DAG
          // below it was built from the chapters it produced, and that expansion
          // is one-way. Rejecting the outline is the way back.
          const locked = stage.kind === 'blueprint' && tasks.length > 1
          const runnable = !locked && !stage.pending
          const produced = artifacts?.get(stage.kind) ?? []
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
                  <span className="block text-subtle">
                    {locked
                      ? 'The pipeline below is built from this outline — reject it to go back'
                      : stage.pending
                        ? 'Nothing here has run yet'
                        : `Click to run ${stage.total === 1 ? 'it' : `all ${stage.total}`} again`}
                  </span>
                  {produced.length > 0 && (
                    <span className="block text-subtle">Right-click to see what it produced</span>
                  )}
                </span>
              }
            >
              <li className="flex min-w-[92px] flex-1">
                <ContextMenu
                  items={
                    <>
                      <ContextMenuLabel>{taskLabel(stage.kind)}</ContextMenuLabel>
                      {produced.length > 0 && (
                        <ContextMenuItem onSelect={() => openViewer(produced, 0)}>
                          <Eye className="h-3.5 w-3.5" />
                          View {produced.length === 1 ? 'artifact' : `${produced.length} artifacts`}
                        </ContextMenuItem>
                      )}
                      {runnable && (
                        <ContextMenuItem onSelect={() => setRerunning(stage)}>
                          <RefreshCw className="h-3.5 w-3.5" />
                          Re-run this step
                        </ContextMenuItem>
                      )}
                    </>
                  }
                >
                  {/*
                  The whole tile is the button. Thirteen stages have no room for
                  thirteen icons beside their counts, and the dialog behind this
                  is a preview rather than an action — an accidental click costs
                  a glance at what would change, not a re-run.
                */}
                  <button
                    type="button"
                    /*
                    aria-disabled rather than disabled: a disabled button stops
                    receiving pointer events, which would take the tooltip with
                    it — and the tooltip is where the reason lives.
                  */
                    aria-disabled={!runnable}
                    onClick={() => runnable && setRerunning(stage)}
                    aria-label={`Re-run ${taskLabel(stage.kind)}`}
                    className={cn(
                      'relative w-full overflow-hidden rounded-[var(--radius-sm)] border px-2 py-1.5 text-left transition-colors',
                      runnable &&
                        'cursor-pointer hover:border-[hsl(var(--border-strong))] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[hsl(var(--accent))]',
                      !runnable && 'cursor-default',
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
                        {stage.total > 1
                          ? `${stage.succeeded}/${stage.total}`
                          : complete
                            ? '✓'
                            : '—'}
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
                  </button>
                </ContextMenu>
              </li>
            </Tooltip>
          )
        })}
      </ol>

      {rerunning && (
        <RerunDialog
          open
          onOpenChange={(open) => !open && setRerunning(null)}
          videoRef={videoRef}
          videoId={videoId}
          taskIds={rerunning.ids}
          what={taskLabel(rerunning.kind).toLowerCase()}
        />
      )}
    </>
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
      ids: [],
      pending: true,
    }
    stage.total += 1
    stage.ids.push(task.id)
    if (task.stale) stage.stale += 1
    if (task.state === 'succeeded') stage.succeeded += 1
    else if (task.state === 'running') stage.running += 1
    else if (task.state === 'failed') stage.failed += 1
    else if (task.state === 'awaiting_approval') stage.gated += 1
    // A stage nobody has reached yet has nothing to run again; one that is
    // running is already doing it.
    if (task.state !== 'blocked' && task.state !== 'running') stage.pending = false
    byKind.set(task.kind, stage)
  }
  return ORDER.filter((kind) => byKind.has(kind)).map((kind) => byKind.get(kind) as Stage)
}
