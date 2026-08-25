import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Pause } from 'lucide-react'
import { useState } from 'react'

import { api, qk } from '../../../core/api'
import type { GateKind, Task } from '../../../core/types'
import { Button } from '../../ui/button'

/**
 * The strip that says the pipeline is waiting for you.
 *
 * Both gates use it — `blueprint` before anything is generated, `upload` before
 * anything is published — because they are the same moment twice: nothing more
 * happens until a person says so. One strip, two texts.
 *
 * It sits between the title and the grid rather than floating over either. A
 * gate is not a notification to be dismissed; it is the state the video is in,
 * and it should occupy space for exactly as long as that is true.
 *
 * One line, and it carries no figures. What a person wants before approving —
 * chapters, words, runtime — is on the summary line immediately below, and a
 * strip that repeats the line under it is not emphasis, it is noise.
 *
 * Rejecting asks for a reason inline. A dialog would be the fourth surface in a
 * window that is trying to have three, and the reason is one line of text.
 */

const COPY: Record<GateKind, { title: string; detail: string }> = {
  blueprint: {
    title: 'The blueprint needs approval',
    detail: 'Nothing is generated until you say so.',
  },
  upload: {
    title: 'The upload needs approval',
    detail: 'Nothing is published until you say so.',
  },
}

interface GateStripProps {
  videoRef: string
  videoId: string
  task: Task
}

export function GateStrip({ videoRef, videoId, task }: GateStripProps) {
  const client = useQueryClient()
  const [rejecting, setRejecting] = useState(false)
  const [reason, setReason] = useState('')

  const gate = (task.gate || 'blueprint') as GateKind
  const copy = COPY[gate] ?? COPY.blueprint

  // The stream will carry the consequences — the tasks that unblock, the state
  // the video moves to — so the only thing to invalidate is what no delta
  // covers: the video body behind this strip.
  const settle = () => {
    void client.invalidateQueries({ queryKey: qk.video(videoRef) })
    void client.invalidateQueries({ queryKey: qk.tasks(videoId) })
  }

  const approve = useMutation({
    mutationFn: () => api.approveGate(videoRef, gate),
    onSuccess: settle,
  })
  const reject = useMutation({
    mutationFn: () => api.rejectGate(videoRef, gate, reason.trim()),
    onSuccess: () => {
      setRejecting(false)
      setReason('')
      settle()
    },
  })

  const busy = approve.isPending || reject.isPending
  const failure = approve.error ?? reject.error

  return (
    <div
      className="hairline-b shrink-0 px-4 py-2.5"
      style={{ backgroundColor: 'var(--accent-wash)' }}
    >
      <div className="flex items-center gap-2">
        <Pause className="size-3.5 shrink-0 text-secondary" strokeWidth={2} />
        <span className="shrink-0 text-[13px] font-semibold text-primary">{copy.title}</span>
        <span className="min-w-0 flex-1 truncate text-[12px] text-secondary">{copy.detail}</span>
        {rejecting ? null : (
          <>
            <Button onClick={() => setRejecting(true)} disabled={busy}>
              Reject
            </Button>
            <Button primary onClick={() => approve.mutate()} disabled={busy}>
              Approve
            </Button>
          </>
        )}
      </div>

      {rejecting ? (
        <form
          className="mt-2 flex items-center gap-2 pl-[22px]"
          onSubmit={(event) => {
            event.preventDefault()
            reject.mutate()
          }}
        >
          <input
            autoFocus
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            placeholder="Why is this being sent back?"
            className="h-[22px] min-w-0 flex-1 rounded-[5px] px-2 text-[12px] text-primary outline-none placeholder:text-tertiary"
            style={{
              backgroundColor: 'var(--raised)',
              boxShadow: '0 0 0 0.5px var(--separator-strong)',
            }}
          />
          <Button onClick={() => setRejecting(false)}>Cancel</Button>
          <Button primary disabled={busy || reason.trim().length === 0}>
            Send back
          </Button>
        </form>
      ) : null}

      {failure ? (
        <p className="mt-1.5 pl-[22px] text-[12px] text-[var(--failed)]">
          {(failure as Error).message}
        </p>
      ) : null}
    </div>
  )
}
