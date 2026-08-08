import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, PanelRightClose, RefreshCw, X } from 'lucide-react'
import { useMemo, useState } from 'react'

import { useCommands } from './lib/keys'
import { Button, IconButton, Textarea } from './ui/controls'
import { ErrorNotice, Modal, PaneHeader, Section, Skeleton, Tooltip } from './ui/primitives'
import { TaskStateDot } from './ui/status'
import { api, qk } from '@/core/api'
import { percent, taskKindRank, taskLabel } from '@/core/format'
import type { GateKind, Task, TaskKind } from '@/core/types'
import { cn } from '@/core/utils'

/**
 * The secondary sidebar's one view, and the point of having a secondary sidebar
 * at all.
 *
 * Run state used to be a tab, which meant the pipeline and the thing it produced
 * could not be looked at together — you watched the tasks, or you read the
 * chapters. Moving it to the right makes that a glance instead of a mode, and
 * takes the gate banner and the stale banner out of the document body, where
 * they used to shove the content down whenever the server changed its mind.
 */
export function RunPanel({ videoRef, onClose }: { videoRef: string; onClose: () => void }) {
  const video = useQuery({ queryKey: qk.video(videoRef), queryFn: () => api.getVideo(videoRef) })
  const videoId = video.data?.id

  const tasks = useQuery({
    queryKey: qk.videoTasks(videoId ?? ''),
    queryFn: () => api.listVideoTasks(videoRef),
    enabled: Boolean(videoId),
  })

  const rows = useMemo(() => tasks.data ?? [], [tasks.data])
  const stages = useMemo(() => stagesOf(rows), [rows])
  const gate = rows.find((t) => t.state === 'awaiting_approval')
  const stale = rows.filter((t) => t.stale)
  const failures = rows.filter((t) => t.state === 'failed')

  const counts = video.data?.counts

  return (
    <div className="flex h-full min-h-0 flex-col bg-panel">
      <PaneHeader title="Run">
        <Tooltip label="Hide the run panel" keys="$mod+Shift+KeyB" side="bottom">
          <IconButton aria-label="Hide the run panel" onClick={onClose}>
            <PanelRightClose className="h-3.5 w-3.5" />
          </IconButton>
        </Tooltip>
      </PaneHeader>

      <div className="min-h-0 flex-1 space-y-4 overflow-y-auto px-3 py-3">
        {counts && (
          <div className="space-y-1.5">
            <div className="flex items-baseline gap-2">
              <span className="tabular text-[18px] font-semibold leading-none text-fg">
                {percent(counts.succeeded, counts.total)}
                <span className="text-[11px] font-normal text-subtle">%</span>
              </span>
              <span className="tabular text-[11px] text-muted">
                {counts.succeeded}/{counts.total} tasks
              </span>
              {counts.running > 0 && (
                <span className="tabular ml-auto text-[11px] text-[hsl(var(--accent))]">
                  {counts.running} running
                </span>
              )}
            </div>
            <div className="h-1 overflow-hidden rounded-full bg-[hsl(var(--bg-hover))]">
              <div
                className="h-full bg-[hsl(var(--accent))] transition-[width] duration-300"
                style={{ width: `${percent(counts.succeeded, counts.total)}%` }}
              />
            </div>
          </div>
        )}

        {gate && <GateCard videoRef={videoRef} videoId={videoId} gate={gate} />}

        {stale.length > 0 && videoId && (
          <StaleCard videoRef={videoRef} videoId={videoId} count={stale.length} />
        )}

        <Section title="Pipeline">
          {tasks.isPending && (
            <div className="space-y-1">
              {Array.from({ length: 6 }, (_, i) => (
                <Skeleton key={i} className="h-5 w-full" />
              ))}
            </div>
          )}
          {!tasks.isPending && stages.length === 0 && (
            <p className="text-[11.5px] text-muted">
              Nothing is enqueued. Start the video to build its DAG.
            </p>
          )}
          <ul className="space-y-px">
            {stages.map((stage) => (
              <StageRow key={stage.kind} stage={stage} />
            ))}
          </ul>
        </Section>

        {failures.length > 0 && videoId && (
          <Section title={`Failures (${failures.length})`}>
            <ul className="space-y-1">
              {failures.slice(0, 12).map((task) => (
                <FailureRow key={task.id} task={task} videoId={videoId} videoRef={videoRef} />
              ))}
            </ul>
            {failures.length > 12 && (
              <p className="text-[11px] text-subtle">
                and {failures.length - 12} more — the console lists them all
              </p>
            )}
          </Section>
        )}
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ stages */

interface Stage {
  kind: TaskKind
  total: number
  succeeded: number
  running: number
  failed: number
  gated: number
  stale: number
}

/**
 * The pipeline as the scheduler actually runs it: a census per kind of work, not
 * a wizard. The DAG has no stage barriers — chapter 3 can be composing while
 * chapter 40's script is written — so a row says how much of a kind is done, not
 * whether the pipeline "is at" that step.
 */
function stagesOf(tasks: Task[]): Stage[] {
  const byKind = new Map<TaskKind, Stage>()
  for (const task of tasks) {
    let stage = byKind.get(task.kind)
    if (!stage) {
      stage = {
        kind: task.kind,
        total: 0,
        succeeded: 0,
        running: 0,
        failed: 0,
        gated: 0,
        stale: 0,
      }
      byKind.set(task.kind, stage)
    }
    stage.total += 1
    if (task.state === 'succeeded') stage.succeeded += 1
    else if (task.state === 'running') stage.running += 1
    else if (task.state === 'failed') stage.failed += 1
    else if (task.state === 'awaiting_approval') stage.gated += 1
    if (task.stale) stage.stale += 1
  }
  return [...byKind.values()].sort((a, b) => taskKindRank(a.kind) - taskKindRank(b.kind))
}

function StageRow({ stage }: { stage: Stage }) {
  const done = percent(stage.succeeded, stage.total)
  const tone = stage.failed
    ? 'bg-[hsl(var(--danger))]'
    : stage.gated
      ? 'bg-[hsl(var(--warning))]'
      : stage.stale
        ? 'bg-[hsl(var(--warning))]'
        : 'bg-[hsl(var(--accent))]'

  return (
    <li className="flex h-[22px] items-center gap-2 rounded-[var(--radius-xs)] px-1 hover:bg-[hsl(var(--bg-hover))]">
      <span
        aria-hidden
        className={cn(
          'h-1.5 w-1.5 shrink-0 rounded-full',
          stage.running > 0 && 'pulse-live',
          stage.failed > 0
            ? 'bg-[hsl(var(--danger))]'
            : stage.gated > 0
              ? 'bg-[hsl(var(--warning))]'
              : stage.succeeded === stage.total
                ? 'bg-[hsl(var(--success))]'
                : stage.running > 0
                  ? 'bg-[hsl(var(--accent))]'
                  : 'bg-[hsl(var(--fg-subtle))] opacity-60',
        )}
      />
      <span className="min-w-0 flex-1 truncate text-[11.5px] text-fg/90">
        {taskLabel(stage.kind)}
      </span>
      {stage.stale > 0 && (
        <Tooltip label={`${stage.stale} stale`}>
          <span className="tabular shrink-0 text-[10px] text-[hsl(var(--warning))]">
            {stage.stale}⚑
          </span>
        </Tooltip>
      )}
      <span className="tabular shrink-0 text-[10.5px] text-subtle">
        {stage.total > 1 ? `${stage.succeeded}/${stage.total}` : ''}
      </span>
      <span className="h-[3px] w-10 shrink-0 overflow-hidden rounded-full bg-[hsl(var(--bg-hover))]">
        <span
          className={cn('block h-full transition-[width] duration-300', tone)}
          style={{ width: `${done}%` }}
        />
      </span>
    </li>
  )
}

/* -------------------------------------------------------------------- gate */

/**
 * The gate, where the banner used to be. A gate is the one moment the pipeline
 * is genuinely waiting on a person, so it sits above the pipeline it is holding
 * up rather than in a strip across the document.
 */
function GateCard({
  videoRef,
  videoId,
  gate,
}: {
  videoRef: string
  videoId: string | undefined
  gate: Task
}) {
  const queryClient = useQueryClient()
  const [rejecting, setRejecting] = useState(false)
  const [reason, setReason] = useState('')

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: qk.video(videoRef) })
    if (videoId) void queryClient.invalidateQueries({ queryKey: qk.videoTasks(videoId) })
    void queryClient.invalidateQueries({ queryKey: ['videos'] })
  }

  const kind = (gate.gate as GateKind) || 'blueprint'
  const approve = useMutation({
    mutationFn: () => api.approveGate(videoRef, kind),
    onSuccess: invalidate,
  })
  const reject = useMutation({
    mutationFn: () => api.rejectGate(videoRef, kind, reason),
    onSuccess: () => {
      setRejecting(false)
      setReason('')
      invalidate()
    },
  })

  useCommands([
    {
      id: 'run.approve',
      label: 'Approve the open gate',
      category: 'Run',
      keys: '$mod+Enter',
      run: () => {
        if (!approve.isPending && !rejecting) approve.mutate()
      },
    },
  ])

  return (
    <div className="space-y-2 rounded-[var(--radius-md)] border border-[hsl(var(--warning)/0.4)] bg-[hsl(var(--warning-soft))] p-2.5">
      <div className="flex items-center gap-1.5">
        <span
          className="h-1.5 w-1.5 rounded-full bg-[hsl(var(--warning))] pulse-live"
          aria-hidden
        />
        <span className="text-[11.5px] font-semibold text-[hsl(var(--warning))]">{kind} gate</span>
      </div>
      <p className="text-[11.5px] leading-snug text-muted">
        The pipeline is holding here until you approve it.
      </p>
      <div className="flex items-center gap-1.5">
        <Tooltip label="Approve and let the pipeline continue" keys="$mod+Enter">
          <Button
            variant="success"
            size="xs"
            className="flex-1"
            disabled={approve.isPending}
            onClick={() => approve.mutate()}
          >
            <Check className="h-3 w-3" />
            Approve
          </Button>
        </Tooltip>
        <Button variant="outline" size="xs" onClick={() => setRejecting(true)}>
          <X className="h-3 w-3" />
          Reject
        </Button>
      </div>
      {approve.isError && <ErrorNotice error={approve.error} />}

      <Modal
        open={rejecting}
        onOpenChange={setRejecting}
        title={`Reject the ${kind} gate`}
        description="The task is marked failed and keeps its reason. Retry it once the input is fixed."
        footer={
          <>
            <Button variant="ghost" onClick={() => setRejecting(false)}>
              Cancel
            </Button>
            <Button variant="danger" disabled={reject.isPending} onClick={() => reject.mutate()}>
              Reject
            </Button>
          </>
        }
      >
        <Textarea
          rows={4}
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          placeholder="The outline repeats itself from chapter 12 onwards."
          aria-label="Rejection reason"
          autoFocus
        />
        {reject.isError && <ErrorNotice error={reject.error} className="mt-3" />}
      </Modal>
    </div>
  )
}

/* ------------------------------------------------------------------- stale */

function StaleCard({
  videoRef,
  videoId,
  count,
}: {
  videoRef: string
  videoId: string
  count: number
}) {
  const queryClient = useQueryClient()

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: qk.videoTasks(videoId) })
    void queryClient.invalidateQueries({ queryKey: qk.video(videoRef) })
    void queryClient.invalidateQueries({ queryKey: ['videos'] })
  }

  const run = useMutation({ mutationFn: () => api.runStale(videoRef), onSuccess: invalidate })
  const accept = useMutation({ mutationFn: () => api.acceptStale(videoRef), onSuccess: invalidate })
  const busy = run.isPending || accept.isPending

  return (
    <div className="space-y-2 rounded-[var(--radius-md)] border border-[hsl(var(--border))] bg-[hsl(var(--bg-elevated))] p-2.5">
      <p className="text-[11.5px] leading-snug text-muted">
        <span className="font-semibold text-[hsl(var(--warning))]">{count} stale</span> — an input
        changed after they ran. The artifacts are intact but unverified.
      </p>
      <div className="flex items-center gap-1.5">
        <Button
          variant="outline"
          size="xs"
          className="flex-1"
          disabled={busy}
          onClick={() => run.mutate()}
        >
          <RefreshCw className="h-3 w-3" />
          Re-run
        </Button>
        <Button variant="ghost" size="xs" disabled={busy} onClick={() => accept.mutate()}>
          Accept
        </Button>
      </div>
      {run.isError && <ErrorNotice error={run.error} />}
      {accept.isError && <ErrorNotice error={accept.error} />}
    </div>
  )
}

/* ---------------------------------------------------------------- failures */

function FailureRow({
  task,
  videoId,
  videoRef,
}: {
  task: Task
  videoId: string
  videoRef: string
}) {
  const queryClient = useQueryClient()
  const retry = useMutation({
    mutationFn: () => api.retryTask(task.id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: qk.videoTasks(videoId) })
      void queryClient.invalidateQueries({ queryKey: qk.video(videoRef) })
    },
  })

  return (
    <li className="rounded-[var(--radius-sm)] border border-[hsl(var(--danger)/0.35)] bg-[hsl(var(--danger-soft))] px-2 py-1.5">
      <div className="flex items-center gap-1.5">
        <TaskStateDot state={task.state} />
        <span className="min-w-0 flex-1 truncate text-[11.5px] font-medium text-fg">
          {taskLabel(task.kind)}
          {task.ordinal > 0 && <span className="tabular text-subtle"> #{task.ordinal}</span>}
        </span>
        <Tooltip label="Retry this task">
          <button
            type="button"
            aria-label="Retry this task"
            disabled={retry.isPending}
            onClick={() => retry.mutate()}
            className="flex h-5 w-5 shrink-0 items-center justify-center rounded-[var(--radius-xs)] text-subtle hover:bg-[hsl(var(--bg-hover))] hover:text-fg disabled:opacity-50"
          >
            <RefreshCw className={cn('h-3 w-3', retry.isPending && 'animate-spin')} />
          </button>
        </Tooltip>
      </div>
      {task.error && (
        <p className="mt-0.5 line-clamp-2 text-[10.5px] leading-snug text-[hsl(var(--danger))]">
          {task.error}
        </p>
      )}
    </li>
  )
}
