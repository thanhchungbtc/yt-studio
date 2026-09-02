import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Circle, CircleSlash, CircleX, LoaderCircle, type LucideIcon, Pause } from 'lucide-react'
import { useRef, useState, type ReactNode } from 'react'

import { api, qk } from '../../../core/api'
import { count } from '../../../core/format'
import type { GateKind, Task, Video } from '../../../core/types'
import { cn } from '../../../core/utils'
import { Button } from '../../ui/button'
import { YouTubeAuthDialog } from '../../youtube-auth'

/**
 * What the pipeline is doing, and the one thing you can do about it.
 *
 * Five situations wearing one shape — icon, what, why, what to do — because
 * they are five answers to the same question. A gate is a stop the pipeline
 * made on purpose; a failure, a cancellation and a video that has never started
 * are stops it made for other reasons; and a run in progress is the fifth.
 * Splitting them would be five places to keep saying the same thing.
 *
 * This row is the *only* place a lifecycle verb appears. Start, Resume, Cancel,
 * Approve and Reject all change what the machine is doing, and Cancel used to
 * live a row below beside Edit — a view toggle — at the same size and weight.
 * One row, one kind of control: nothing here changes what you are looking at,
 * and nothing that changes what you are looking at is here.
 *
 * It is still absent when there is nothing to say. A finished video has no verb
 * left, and a strip that never goes away is chrome.
 *
 * The `detail` is different information in each case rather than five ways of
 * saying "stopped". That twenty-nine-of-thirty-one is what tells you resuming
 * is nearly free; the 429 is what tells you to wait a minute before trying. And
 * while it runs, the detail is what is *in flight* — never the done-count, which
 * the title bar above is already showing.
 */

interface Face {
  icon: LucideIcon
  /** The only shape that moves, for the only state that is still moving. */
  spin?: boolean
  tint: string
  iconColor: string
  title: string
  detail: string
  /** The full text, when `detail` had to be cut to fit one line. */
  full?: string
  actions: ReactNode
}

const GATE_COPY: Record<GateKind, { title: string; detail: string }> = {
  blueprint: {
    title: 'The blueprint needs approval',
    detail: 'Nothing is generated until you say so.',
  },
  upload: {
    title: 'The upload needs approval',
    detail: 'Nothing is published until you say so.',
  },
}

/**
 * Which stage a failed task belongs to, in the words the table uses.
 *
 * A chapter's ordinal is included when it has one: "narration, chapter 2" is
 * a place to go and look, where "narration" alone is a column.
 */
function stageOf(task: Task): string {
  const where = task.chapterId ? `, chapter ${task.ordinal}` : ''
  switch (task.kind) {
    case 'blueprint':
      return 'the blueprint'
    case 'prime_slide_prompts':
    case 'slide_prompts':
      return 'the slide prompts'
    case 'script':
      return `a script${where}`
    case 'tts':
      return `narration${where}`
    case 'slide':
      return `a slide${where}`
    case 'clip':
      return `a clip${where}`
    case 'concat':
      return 'the cut'
    case 'metadata':
      return 'the metadata'
    case 'thumbnail_plan':
    case 'thumbnail_icon':
    case 'thumbnail':
      return 'the thumbnail'
    case 'upload':
      return 'the upload'
    default:
      return 'the pipeline'
  }
}

/** `a slide, chapter 2` as the start of a sentence. */
function sentence(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1)
}

export function StoppedStrip({ video, tasks }: { video: Video; tasks: Task[] }) {
  const client = useQueryClient()
  const [rejecting, setRejecting] = useState(false)
  const [reason, setReason] = useState('')
  const [authorizing, setAuthorizing] = useState(false)
  /*
    What to do once the grant exists.

    A ref rather than state because the dialog closes and reports success in the
    same tick, and the decision must survive that without depending on which of
    the two happens first.
  */
  const resumption = useRef<'approve' | 'resume'>('approve')

  // The stream carries the consequences — the tasks that unblock, the state the
  // video moves to. What no delta covers is the video body behind this strip.
  const settle = () => {
    void client.invalidateQueries({ queryKey: qk.video(video.ref) })
    void client.invalidateQueries({ queryKey: qk.tasks(video.id) })
    void client.invalidateQueries({ queryKey: qk.videos })
  }

  const gate = tasks.find((task) => task.state === 'awaiting_approval')
  const gateKind = (gate?.gate || 'blueprint') as GateKind

  /*
    The two moments a grant is worth asking about: before a publish is approved,
    and after one has failed for want of it. The second is not the same question
    asked twice — a gate that was approved while the grant was still good is a
    video that is now stopped with no way back on, and Resume alone would walk
    it into the same wall.
  */
  const brokenUpload = tasks.find((task) => task.state === 'failed' && task.kind === 'upload')
  const publishing = (Boolean(gate) && gateKind === 'upload') || Boolean(brokenUpload)

  /*
    Whether publishing this video needs a Google account behind it.

    Which backend publishes, and nothing else. Dry run deliberately does not
    come into it: the YouTube backend proves the grant before it decides whether
    to send, because a rehearsal that skipped the one check that fails in
    practice would rehearse nothing worth knowing. So a dry run needs a grant
    exactly as much as a real publish does — it just stops afterwards.

    What keeps this out of the way of ordinary work is the default backend. An
    installation publishing through `sample` is asked for nothing, which is the
    whole point of having it.
  */
  const settings = useQuery({
    queryKey: qk.settings,
    queryFn: api.listSettings,
    enabled: publishing,
  })
  const setting = (name: string) => settings.data?.find((row) => row.key === name)?.value
  const needsGoogle = setting('provider.uploader') === 'youtube'

  /*
    What the channel can currently publish with.

    Asked whenever a publish is in play, which also repairs a drift nothing else
    can: the task that discovers a dead grant writes that to the credentials on
    disk, and the channel's row goes on saying `valid` until something reads
    them again. This is that read — the server reconciles the row while
    answering it — so the row is corrected by the act of showing the right
    button.
  */
  const auth = useQuery({
    queryKey: qk.channelAuth(video.channelId),
    queryFn: () => api.channelAuth(video.channelId),
    enabled: publishing && needsGoogle,
  })
  /** A stopped upload that will stay stopped until somebody authorizes. */
  const needsGrant = Boolean(brokenUpload) && needsGoogle && auth.data?.authorized === false

  const askToAuthorize = (then: 'approve' | 'resume') => {
    resumption.current = then
    setAuthorizing(true)
  }

  const approve = useMutation({
    mutationFn: () => api.approveGate(video.ref, gateKind),
    onSuccess: settle,
  })

  /*
    Approve, with the one question that has to be answered first.

    A publish that is going to be refused for want of a grant is better refused
    now than after the gate has been released and the task has failed — the
    difference between a dialog and a red strip over a video that has to be
    resumed. The check is a courtesy and not the guard: the task checks again
    when it runs, which is what catches a grant revoked in the seconds between.

    So a preflight that cannot answer approves anyway. Refusing to publish
    because a status call failed would put this screen in the way of a channel
    that is perfectly well authorized.
  */
  const publish = useMutation({
    mutationFn: async () => {
      if (gateKind === 'upload' && needsGoogle) {
        // Through the cache rather than around it, so the answer this decision
        // was made on is the same one the strip is rendering.
        const current = await client
          .fetchQuery({
            queryKey: qk.channelAuth(video.channelId),
            queryFn: () => api.channelAuth(video.channelId),
            staleTime: 0,
          })
          .catch(() => null)
        if (current && !current.authorized) return 'needs-authorization' as const
      }
      await api.approveGate(video.ref, gateKind)
      return 'approved' as const
    },
    onSuccess: (outcome) => {
      if (outcome === 'needs-authorization') {
        askToAuthorize('approve')
        return
      }
      settle()
    },
  })
  const reject = useMutation({
    mutationFn: () => api.rejectGate(video.ref, gateKind, reason.trim()),
    onSuccess: () => {
      setRejecting(false)
      setReason('')
      settle()
    },
  })
  const start = useMutation({ mutationFn: () => api.startVideo(video.ref), onSuccess: settle })
  const cancel = useMutation({ mutationFn: () => api.cancelVideo(video.ref), onSuccess: settle })

  const busy =
    approve.isPending ||
    publish.isPending ||
    reject.isPending ||
    start.isPending ||
    cancel.isPending ||
    // Only while the two rows that decide whether this publishes are still
    // unknown, and only for the gate that reads them. Half a second on the one
    // button whose meaning depends on them.
    (publishing && settings.isPending)
  const failure = approve.error ?? publish.error ?? reject.error ?? start.error ?? cancel.error

  const resume = (label: string) => (
    <Button primary onClick={() => start.mutate()} disabled={busy}>
      {start.isPending ? 'Starting…' : label}
    </Button>
  )

  const face = ((): Face | null => {
    if (gate) {
      const copy = GATE_COPY[gateKind] ?? GATE_COPY.blueprint
      return {
        icon: Pause,
        tint: 'var(--accent-wash)',
        iconColor: 'var(--accent)',
        title: copy.title,
        detail: copy.detail,
        actions: (
          <>
            <Button onClick={() => setRejecting(true)} disabled={busy}>
              Reject
            </Button>
            <Button primary onClick={() => publish.mutate()} disabled={busy}>
              {publish.isPending ? 'Approving…' : 'Approve'}
            </Button>
          </>
        ),
      }
    }

    if (video.state === 'failed' || video.state === 'blocked') {
      const broken = tasks.filter((task) => task.state === 'failed')
      const first = broken[0]
      const error = first?.error || video.error || 'No reason was recorded.'
      const others = broken.length > 1 ? ` · ${broken.length} tasks failed` : ''
      return {
        icon: CircleX,
        tint: 'var(--failed-wash)',
        iconColor: 'var(--failed)',
        title: first ? `Stopped at ${stageOf(first)}` : 'Stopped',
        detail: `${error}${others}`,
        full: error,
        // Resuming an upload the channel is not authorized for would fail in
        // the same way within the second. The verb offered is the one that
        // changes the outcome, and it resumes on its own once it has.
        actions: needsGrant ? (
          <Button primary onClick={() => askToAuthorize('resume')} disabled={busy}>
            Authorize YouTube
          </Button>
        ) : (
          resume('Resume')
        ),
      }
    }

    if (video.state === 'cancelled') {
      const { succeeded, total } = video.counts
      return {
        icon: CircleSlash,
        tint: 'transparent',
        iconColor: 'var(--text-tertiary)',
        title: 'Cancelled',
        detail:
          total > 0
            ? `${count(succeeded)} of ${count(total)} done · resuming picks up where it stopped`
            : 'Nothing had run yet.',
        actions: resume('Resume'),
      }
    }

    if (video.state === 'running') {
      const inFlight = tasks.filter((task) => task.state === 'running')
      const first = inFlight[0]
      const others = inFlight.length > 1 ? ` · ${inFlight.length} running` : ''
      return {
        icon: LoaderCircle,
        spin: true,
        tint: 'transparent',
        iconColor: 'var(--running)',
        title: 'Running',
        // What is in flight, not how much is done. The done-count is on the line
        // above this one, and printing it twice would make this row chrome.
        detail: first ? `${sentence(stageOf(first))}${others}` : 'Waiting for a worker.',
        actions: (
          <Button onClick={() => cancel.mutate()} disabled={busy}>
            {cancel.isPending ? 'Cancelling…' : 'Cancel'}
          </Button>
        ),
      }
    }

    if (video.state === 'draft') {
      return {
        icon: Circle,
        tint: 'transparent',
        iconColor: 'var(--text-tertiary)',
        title: 'Not started',
        detail: 'The blueprint is written first, then paused for your review.',
        actions: resume('Start'),
      }
    }

    return null
  })()

  if (!face) return null

  return (
    <div className="hairline-b shrink-0 px-4 py-2.5" style={{ backgroundColor: face.tint }}>
      <div className="flex items-center gap-2">
        <face.icon
          className={cn('size-3.5 shrink-0', face.spin && 'animate-spin')}
          strokeWidth={2}
          style={{ color: face.iconColor }}
        />
        <span className="shrink-0 text-[13px] font-semibold text-primary">{face.title}</span>
        <span
          className="min-w-0 flex-1 truncate text-[12px] text-secondary"
          title={face.full ?? face.detail}
        >
          {face.detail}
        </span>
        {rejecting ? null : face.actions}
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
            className="control min-w-0 flex-1"
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

      {/*
        Rendered from here rather than from the upload page, because this is the
        row that asked the question: the gate is answered above every mode, and
        an operator who pressed Approve from the pipeline table should not have
        to go and find the dialog somewhere else. Once the grant exists it
        approves for real — one press, one publish, with a detour in between.
      */}
      <YouTubeAuthDialog
        channel={video.channelId}
        open={authorizing}
        onOpenChange={setAuthorizing}
        onAuthorized={() =>
          resumption.current === 'resume' ? start.mutate() : approve.mutate()
        }
        confirmLabel={resumption.current === 'resume' ? 'Authorize and resume' : 'Authorize and publish'}
      />
    </div>
  )
}
