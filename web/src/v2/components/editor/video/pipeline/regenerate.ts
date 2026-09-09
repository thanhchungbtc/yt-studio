import { useMutation, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { Check, RefreshCw, RotateCcw } from 'lucide-react'
import { useCallback } from 'react'

import { api } from '../../../../core/api'
import type { Task } from '../../../../core/types'
import type { MenuItem } from '../../../ui/menu'
import { cellAction, STAGE_KINDS, type Cell, type PipelineStage, type StageId } from '../stages'

/**
 * The key a chapter's plan edit saves under.
 *
 * It exists so that the thing which *waits* for those saves can name them. The
 * table is the only writer and this module is the only reader, which is why it
 * lives here rather than beside the query keys: it is one half of a handshake,
 * not part of the cache's vocabulary.
 */
export const PLAN_SAVE = ['v2', 'chapter-plan'] as const

/** One press: which task, and what to do to it. */
interface Job {
  action: 'rerun' | 'retry' | 'accept'
  taskId: string
}

/*
  A plan edit commits on blur, and the blur that opened a menu is part of the
  same gesture that is now asking for a re-run. The two race, and the losing
  side is silent: the script gets rewritten from the summary the operator has
  just replaced, and nothing on screen says so.

  Against a local server the window is a few milliseconds. It is also a few
  lines to close, and the failure it prevents is one nobody would think to look
  for.
*/
async function afterPlanSave(client: QueryClient): Promise<void> {
  while (client.isMutating({ mutationKey: PLAN_SAVE }) > 0) {
    await new Promise((resolve) => setTimeout(resolve, 25))
  }
}

/**
 * The three things a dot can do, as one mutation and a menu builder.
 *
 * One hook per table rather than one per dot. A video is eighty cells and a
 * hook in each is eighty mutation subscriptions to render a grid that changes
 * on the event stream anyway; the menu is built on demand from the cell it is
 * for, and the press it produces is the same press wherever it came from.
 *
 * Nothing is invalidated on success. The scheduler records every task it
 * touches — the seed it reset and each dependent it flagged — and those arrive
 * as deltas, so the grid is already being told. A refetch here would ask for
 * what is on its way.
 */
export function useCellMenu(videoId: string) {
  const client = useQueryClient()

  const run = useMutation({
    mutationFn: async (job: Job) => {
      await afterPlanSave(client)
      switch (job.action) {
        case 'retry':
          return api.retryTask(job.taskId)
        case 'rerun':
          return api.rerunTasks(videoId, [job.taskId])
        case 'accept':
          return api.acceptStale(videoId, [job.taskId])
      }
    },
  })

  const { mutate, reset } = run

  /**
   * The menu for one cell, named after the thing it holds.
   *
   * The noun is passed in rather than derived from the task's kind, because the
   * column already knows it and the words the table uses are the words this
   * should use. Naming it matters more than it looks: the target is twelve
   * pixels, and "Regenerate Narration" is how you find out you missed.
   */
  const menuFor = useCallback(
    (cell: Cell, noun: string): MenuItem[] => {
      const action = cellAction(cell)
      if (!action || !cell.task) return []
      const taskId = cell.task.id

      const items: MenuItem[] = [
        {
          label: action === 'retry' ? `Retry ${noun}` : `Regenerate ${noun}`,
          icon: action === 'retry' ? RotateCcw : RefreshCw,
          onSelect: () => {
            reset()
            mutate({ action, taskId })
          },
        },
      ]

      // Only on a flagged dot. Staleness says an input moved, not that the
      // output is wrong, and without this the only way to settle the ring is to
      // pay for a generation nobody asked for — so the second row is what makes
      // the first one safe to press.
      if (cell.stale) {
        items.push({
          label: 'Keep As Is',
          icon: Check,
          onSelect: () => {
            reset()
            mutate({ action: 'accept', taskId })
          },
        })
      }
      return items
    },
    [mutate, reset],
  )

  return { menuFor, error: run.error as Error | null }
}

/**
 * The stages the inspector can redo whole, and what the item says.
 *
 * Four of the ten, and the four are the serial tail: narration, the clips built
 * from it, the cut built from those, and the listing written off the cut. That
 * is the chain you walk down after changing something, one press per rung, and
 * it is the reason this is a short list rather than a permission model. The six
 * absent ones are absent for a reason each — a blueprint cannot be rolled twice
 * because expansion is one-way, re-running an upload is a republish rather than
 * a regeneration, and the rest are simply not asked for yet.
 *
 * "all" only where there is more than one. Cut and Metadata are single tasks,
 * and offering to regenerate all of the one of them is the menu misreading its
 * own subject.
 */
const STAGE_LABELS: Partial<Record<StageId, string>> = {
  narration: 'Regenerate all Narration',
  clips: 'Regenerate all Clips',
  cut: 'Regenerate Cut',
  metadata: 'Regenerate Metadata',
}

/**
 * The one thing a stage row can do, as one mutation and a menu builder.
 *
 * The sibling of `useCellMenu`, one level up: same verb, same endpoint, scope
 * widened from a single task to every task of the stage's kinds. It is a
 * re-run and deliberately not a cascade — everything below keeps its artifact
 * and is flagged stale — so redoing narration leaves the clips holding the old
 * audio until the next rung down is pressed. That is the same bargain the cell
 * menu strikes, and the reason the four form a chain worth walking.
 *
 * Only `done` and `failed` offer anything, exactly as `cellAction` decides for
 * a cell: a dot you can press is a dot with a result you might disagree with.
 * A failed stage takes the same press, because a re-run resets the tasks it
 * names whatever state they are in, and the ones under a failure never ran and
 * so are never flagged.
 */
export function useStageMenu(videoId: string, tasks: Task[]) {
  const client = useQueryClient()

  const run = useMutation({
    mutationFn: async (taskIds: string[]) => {
      await afterPlanSave(client)
      return api.rerunTasks(videoId, taskIds)
    },
  })

  const { mutate, reset } = run

  const menuFor = useCallback(
    (stage: PipelineStage): MenuItem[] => {
      const label = STAGE_LABELS[stage.id]
      if (!label) return []
      if (stage.cell.state !== 'done' && stage.cell.state !== 'failed') return []

      const kinds = STAGE_KINDS[stage.id]
      const taskIds = tasks.filter((task) => kinds.includes(task.kind)).map((task) => task.id)
      // A stage can read `done` off its artifacts before the graph holding the
      // tasks has been expanded, and a press with nothing to name is an error
      // the operator did not ask a question to get.
      if (taskIds.length === 0) return []

      return [
        {
          label,
          icon: RefreshCw,
          onSelect: () => {
            reset()
            mutate(taskIds)
          },
        },
      ]
    },
    [tasks, mutate, reset],
  )

  return { menuFor, error: run.error as Error | null }
}
