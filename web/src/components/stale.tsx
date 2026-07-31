import { useMutation, useQueryClient } from '@tanstack/react-query'
import { AlertCircle, Check, Play, RefreshCw } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ErrorNotice, Modal, Tooltip } from '@/components/ui/primitives'
import { api, qk } from '@/lib/api'
import { taskLabel } from '@/lib/format'
import type { Task, Video } from '@/lib/types'
import { cn } from '@/lib/utils'

/* -------------------------------------------------------------- the mark */

/**
 * The stale mark. Amber rather than red: nothing has gone wrong, an input just
 * moved and this output has not been checked against it.
 */
export function StaleBadge({ className }: { className?: string }) {
  return (
    <Tooltip label="An input changed after this ran. The artifact is intact — it just has not been checked against the new input.">
      <span>
        <Badge tone="warning" className={cn('gap-1', className)}>
          <AlertCircle className="h-3 w-3" />
          stale
        </Badge>
      </span>
    </Tooltip>
  )
}

/** A dot for rows too dense to carry the badge. */
export function StaleDot({ className }: { className?: string }) {
  return (
    <Tooltip label="Stale — an input changed after this ran">
      <span
        className={cn('h-[7px] w-[7px] shrink-0 rounded-full bg-[hsl(var(--warning))]', className)}
        aria-label="stale"
      />
    </Tooltip>
  )
}

/* ------------------------------------------------------------- the banner */

/**
 * The stale set, and the two ways out of it. Both exits carry equal weight:
 * staleness records that an input changed, not that the output is wrong, so
 * accepting is as legitimate as re-running.
 */
export function StaleBanner({ video, tasks }: { video: Video; tasks: Task[] }) {
  const queryClient = useQueryClient()
  const [reviewing, setReviewing] = useState(false)

  const stale = useMemo(() => tasks.filter((t) => t.stale), [tasks])

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: qk.videoTasks(video.id) })
    void queryClient.invalidateQueries({ queryKey: qk.video(video.ref) })
    void queryClient.invalidateQueries({ queryKey: ['videos'] })
  }

  const run = useMutation({
    mutationFn: (ids: string[]) => api.runStale(video.ref, ids),
    onSuccess: () => {
      setReviewing(false)
      invalidate()
    },
  })
  const accept = useMutation({
    mutationFn: (ids: string[]) => api.acceptStale(video.ref, ids),
    onSuccess: () => {
      setReviewing(false)
      invalidate()
    },
  })

  if (stale.length === 0) return null

  const busy = run.isPending || accept.isPending

  return (
    <>
      <div className="flex shrink-0 items-center gap-3 border-b border-[hsl(var(--warning)/0.35)] bg-[hsl(var(--warning-soft))] px-4 py-2.5">
        <Badge tone="warning" dot className="shrink-0">
          {stale.length} stale
        </Badge>
        <p className="min-w-0 flex-1 text-[12px] text-fg">
          {stale.length === 1 ? 'One task was' : `${stale.length} tasks were`} built from an input
          that has since changed. Their artifacts are still here and may still be right — nothing
          re-runs until you say so.
        </p>
        <div className="flex shrink-0 items-center gap-2">
          <Button size="sm" variant="ghost" onClick={() => setReviewing(true)}>
            Review
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={busy}
            onClick={() => accept.mutate([])}
            title="Clear the flag without re-running anything"
          >
            <Check className="h-3.5 w-3.5" />
            Keep all
          </Button>
          <Button size="sm" variant="primary" disabled={busy} onClick={() => run.mutate([])}>
            <RefreshCw className={cn('h-3.5 w-3.5', run.isPending && 'animate-spin')} />
            Re-run all
          </Button>
        </div>
      </div>

      <StaleReviewDialog
        open={reviewing}
        onOpenChange={setReviewing}
        stale={stale}
        busy={busy}
        error={run.error ?? accept.error}
        onRun={(ids) => run.mutate(ids)}
        onAccept={(ids) => accept.mutate(ids)}
      />
    </>
  )
}

function StaleReviewDialog({
  open,
  onOpenChange,
  stale,
  busy,
  error,
  onRun,
  onAccept,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  stale: Task[]
  busy: boolean
  error: unknown
  onRun: (ids: string[]) => void
  onAccept: (ids: string[]) => void
}) {
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title="Stale work"
      description="Each of these ran before one of its inputs changed. Re-run it, or keep what is already there."
      wide
      footer={
        <>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Close
          </Button>
          <Button variant="outline" disabled={busy} onClick={() => onAccept([])}>
            Keep all
          </Button>
          <Button variant="primary" disabled={busy} onClick={() => onRun([])}>
            Re-run all
          </Button>
        </>
      }
    >
      <ul className="divide-y divide-[hsl(var(--border))]">
        {stale.map((task) => (
          <li key={task.id} className="flex items-center gap-3 py-2">
            <StaleDot />
            <span className="min-w-0 flex-1">
              <span className="text-[12.5px] text-fg">
                {taskLabel(task.kind)}
                {task.index >= 0 && <span className="text-subtle"> #{task.index + 1}</span>}
              </span>
              <span className="ml-2 text-[11px] text-subtle">
                {task.ordinal > 0 ? `chapter ${task.ordinal}` : 'whole video'}
              </span>
            </span>
            <div className="flex shrink-0 gap-1">
              <Button size="xs" variant="ghost" disabled={busy} onClick={() => onAccept([task.id])}>
                <Check className="h-3 w-3" />
                Keep
              </Button>
              <Button size="xs" variant="outline" disabled={busy} onClick={() => onRun([task.id])}>
                <Play className="h-3 w-3" />
                Re-run
              </Button>
            </div>
          </li>
        ))}
      </ul>
      {error != null && <ErrorNotice error={error} className="mt-3" />}
    </Modal>
  )
}

/* -------------------------------------------------------- the re-run gate */

/**
 * The confirmation in front of re-running a step. It answers one question
 * first: what else does this touch? Re-running one chapter's script also
 * invalidates the concat, the metadata, the thumbnail and the upload.
 *
 * Re-running the step alone is the default and the only thing the primary
 * button does. Rebuilding everything below it is a second, separately-worded
 * choice, offered only once the preview has said what "everything below"
 * actually is — a cascade nobody asked for is how a reviewed artifact gets
 * thrown away without anyone noticing.
 */
export function RerunDialog({
  open,
  onOpenChange,
  videoRef,
  videoId,
  taskIds,
  what,
  onCascade,
  cascadePending,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  videoRef: string
  videoId: string
  taskIds: string[]
  /** What the operator asked to redo, in their words. */
  what: string
  /**
   * Rebuilds everything downstream as well. Omitted where there is no single
   * command for it; the dialog then offers the safe path alone.
   */
  onCascade?: () => void
  cascadePending?: boolean
}) {
  const queryClient = useQueryClient()

  // The dry run is the preview: it computes the closure and changes nothing.
  const preview = useMutation({
    mutationFn: () => api.rerunTasks(videoRef, taskIds, true),
  })
  const commit = useMutation({
    mutationFn: () => api.rerunTasks(videoRef, taskIds, false),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: qk.videoTasks(videoId) })
      void queryClient.invalidateQueries({ queryKey: qk.video(videoRef) })
      void queryClient.invalidateQueries({ queryKey: ['videos'] })
      onOpenChange(false)
    },
  })

  // Run the preview once per opening. Keyed on the ids so reopening the dialog
  // for a different task does not show the previous answer.
  const key = taskIds.join(',')
  const previewRef = useRef(preview)
  previewRef.current = preview
  useEffect(() => {
    if (open) previewRef.current.mutate()
  }, [open, key])

  const stale = preview.data?.stale ?? []

  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title={`Re-run ${what}`}
      description="Only this step runs. Anything below it keeps its artifact and is flagged for you to decide on."
      footer={
        <>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          {onCascade && stale.length > 0 && (
            <Button
              variant="ghost"
              disabled={commit.isPending || cascadePending}
              onClick={onCascade}
              className="text-[hsl(var(--danger))]"
            >
              {cascadePending ? 'Starting…' : `Rebuild all ${stale.length} below`}
            </Button>
          )}
          <Button variant="primary" disabled={commit.isPending} onClick={() => commit.mutate()}>
            {commit.isPending ? 'Starting…' : 'Re-run this step'}
          </Button>
        </>
      }
    >
      <div className="space-y-3 text-[12.5px]">
        <p className="text-muted">
          <strong className="text-fg">{what}</strong> will run again now.
        </p>

        {preview.isPending && <p className="text-subtle">Working out what this affects…</p>}
        {preview.isError && <ErrorNotice error={preview.error} />}

        {preview.data && stale.length === 0 && (
          <p className="text-muted">Nothing downstream has run yet, so nothing else is affected.</p>
        )}

        {stale.length > 0 && (
          <div className="rounded-[var(--radius-sm)] border border-[hsl(var(--warning)/0.4)] bg-[hsl(var(--warning-soft))] px-3 py-2.5">
            <p className="mb-2 text-fg">
              <strong>{stale.length}</strong>{' '}
              {stale.length === 1 ? 'task keeps its artifact' : 'tasks keep their artifacts'} and
              {stale.length === 1 ? ' is' : ' are'} flagged stale. Nothing below re-runs until you
              ask it to{onCascade ? ', here or from the banner' : ''}.
            </p>
            <ul className="flex flex-wrap gap-1.5">
              {stale.map((task) => (
                <li key={task.id}>
                  <Badge tone="warning">
                    {taskLabel(task.kind)}
                    {task.ordinal > 0 && <span className="opacity-70"> {task.ordinal}</span>}
                  </Badge>
                </li>
              ))}
            </ul>
          </div>
        )}

        {commit.isError && <ErrorNotice error={commit.error} />}
      </div>
    </Modal>
  )
}
