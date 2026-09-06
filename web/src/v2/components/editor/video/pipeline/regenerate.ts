import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Check, RefreshCw, RotateCcw } from 'lucide-react'
import { useCallback } from 'react'

import { api } from '../../../../core/api'
import type { MenuItem } from '../../../ui/menu'
import { cellAction, type Cell } from '../stages'

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
      /*
        A plan edit commits on blur, and the blur that opened this menu is part
        of the same gesture that is now asking for a re-run. The two race, and
        the losing side is silent: the script gets rewritten from the summary
        the operator has just replaced, and nothing on screen says so.

        Against a local server the window is a few milliseconds. It is also a
        few lines to close, and the failure it prevents is one nobody would
        think to look for.
      */
      while (client.isMutating({ mutationKey: PLAN_SAVE }) > 0) {
        await new Promise((resolve) => setTimeout(resolve, 25))
      }
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
