import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Check, Copy, Upload } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'

import { api, qk } from '../../core/api'
import type { Video } from '../../core/types'
import { useVideoDoc } from '../../core/queries'
import { useDock } from '../editor/dock'
import { Mark } from '../editor/video/mark'
import { useStageMenu } from '../editor/video/pipeline/regenerate'
import { pipelineStages, type PipelineStage } from '../editor/video/stages'
import { Button } from '../ui/button'
import { Dialog } from '../ui/dialog'
import type { MenuItem } from '../ui/menu'

/**
 * The inspector: the pipeline of whatever document is in front.
 *
 * It is the grid rotated ninety degrees. The table in the editor reads chapters
 * down and stages across, so how far *one stage* has got is a column you have
 * to count; here the stages are the rows and the counting is done. Same tasks,
 * different question — how is chapter 2 doing, against how far along is the
 * video — which is why this is not a second copy of the table.
 *
 * Ten rows, always the same ten, in the order the work happens. A panel whose
 * length grew as the run progressed could not be read at a glance: you would
 * have to work out what was missing before you could see where you were. Fixed,
 * an open ring *is* the information.
 *
 * It follows the front tab rather than the sidebar's selection. Single-clicking
 * a row already opens the document, so binding to the selection would be a
 * second, slower answer to the question the tab strip has already answered.
 */
export function Inspector() {
  const doc = useDock((s) => s.activeDoc)

  if (doc?.kind === 'video') return <VideoPipeline videoRef={doc.ref} />
  return <Note>{doc ? 'Nothing to inspect here yet.' : 'Open a video to see its pipeline.'}</Note>
}

function VideoPipeline({ videoRef }: { videoRef: string }) {
  // The same three keys the editor uses, deliberately. Two panes asking the
  // same questions of one cache is one fetch and one answer; a private key here
  // would be a second copy of the video that drifts from the one on screen.
  const { video, chapters, tasks, ready } = useVideoDoc(videoRef)

  const data = ready ? video.data : undefined
  const stages = useMemo(
    () => (data ? pipelineStages(data, chapters.data ?? [], tasks.data ?? []) : []),
    [data, chapters.data, tasks.data],
  )
  // Hooks run before the early returns below, so the empty id is the one render
  // where there is no video yet — and `menuFor` is never called in it, because
  // there are no stages to call it for.
  const { menuFor, error } = useStageMenu(data?.id ?? '', tasks.data ?? [])
  const [republishing, setRepublishing] = useState(false)

  if (video.error) return <Note>That video could not be loaded.</Note>
  if (!data) return <div className="min-h-0 flex-1" />

  /*
    Upload is the one row whose dot does not offer a re-run, and the reason is
    in `regenerate.ts`: re-running an upload is a republish rather than a
    regeneration. YouTube cannot replace a video's file, so there is no version
    of this that edits what is already published — it can only add a second
    video and abandon the first.

    So the item is here rather than in the stage menu, worded as what it does
    and ending in an ellipsis, because the dialog it opens is the only place
    the abandoned URL is ever shown again.
  */
  const published = data.upload !== undefined && !data.upload.dryRun
  const menuOf = (stage: PipelineStage): MenuItem[] => {
    const items = menuFor(stage)
    if (stage.id !== 'upload' || !published) return items
    return [
      ...items,
      { label: 'Upload Again…', icon: Upload, onSelect: () => setRepublishing(true) },
    ]
  }

  return (
    <div className="scroll-edge min-h-0 flex-1 overflow-y-auto py-1.5">
      {/* What this is the pipeline of.

          The pane follows the front tab and used to say so nowhere, so
          switching tabs changed all ten rows with nothing on screen accounting
          for it. The ref carries that on its own — it is the one label that is
          different for two videos of the same name — and the title follows it
          for the times you know the video by its name and not its number. */}
      <div className="flex items-baseline gap-1.5 px-3 pt-0.5 pb-2 text-xs">
        <span className="shrink-0 font-medium tabular-nums text-fg-muted">{data.ref}</span>
        <span className="min-w-0 truncate text-fg-subtle">{data.title || 'Untitled'}</span>
      </div>

      {/* Above the rows rather than beside the dot that caused it: the menu has
          closed by the time this exists, and the pane is too narrow to hang a
          message off a twelve-pixel target. One at a time, as there is one
          press at a time. */}
      {error ? <p className="px-3 pb-1.5 text-xs text-[var(--failed)]">{error.message}</p> : null}

      {stages.map((stage) => (
        <StageRow key={stage.id} stage={stage} menu={menuOf(stage)} />
      ))}

      {republishing ? (
        <RepublishDialog video={data} onClose={() => setRepublishing(false)} />
      ) : null}
    </div>
  )
}

/**
 * One stage: the mark, its name, and how far it has got.
 *
 * The mark is the one control, and only on four of the ten rows. This pane used
 * to hold no buttons at all, and the rule it was keeping is narrower than it
 * read: there was an Approve on the gate row, which meant one gate wore two
 * buttons — one here and one in the strip beside the Reject that goes with it —
 * and a pane that can be hidden with ⌘3 is the wrong of the two to make
 * load-bearing. Both halves of that were about Approve. It duplicated a verb
 * the document already had, and it was the only way out of a blocked video.
 *
 * Regenerating a whole stage is neither. It exists nowhere else — the table
 * offers one dot at a time, and only four of these ten stages are columns in
 * it at all — and hiding this pane costs a shortcut rather than an exit, since
 * every one of those presses can still be made cell by cell. A stage is also
 * the only object either pane has that *is* the whole stage, which is what the
 * verb is about. So the gate stays reported and the four settled stages act.
 *
 * The trailing edge holds exactly one thing, in this order: the gate when there
 * is one, then the count, then a percentage. Blueprint and Upload have nothing
 * to count, and Cut is the only stage reporting a percentage, so those never
 * contend. Script is the one that does — it holds a gate *and* a count — and the
 * order is what settles it: a script gate only opens once every chapter has
 * settled, so the count it displaces reads n/n, which the filled disc beside it
 * has already said.
 *
 * The count comes before the percentage deliberately. `12/21` says how much work
 * there is as well as how much is done, which a percentage cannot; a percentage
 * earns its place only where there is a single task and nothing to count.
 */
function StageRow({ stage, menu }: { stage: PipelineStage; menu: MenuItem[] }) {
  const gate = stage.gate
  // Read only while the stage is running. A delta that carries no percent does
  // not clear the last one, so a finished task keeps the figure it stopped at —
  // and 100% next to a settled disc would be the row saying the same thing
  // twice, in the slot kept for whatever is still moving.
  const percent = stage.cell.state === 'running' ? stage.cell.task?.percent : undefined

  const trailing = ((): ReactNode => {
    if (gate) {
      return (
        <span className="shrink-0 text-xs font-medium" style={{ color: 'var(--accent)' }}>
          Needs approval
        </span>
      )
    }
    if (stage.count) return <Figure>{`${stage.count.done}/${stage.count.total}`}</Figure>
    if (percent !== undefined) return <Figure>{`${percent}%`}</Figure>
    return null
  })()

  return (
    <div className="flex h-[27px] items-center gap-2.5 px-3">
      <Mark cell={stage.cell} menu={menu} />
      <span className="min-w-0 flex-1 truncate text-sm text-fg">{stage.label}</span>
      {trailing}
    </div>
  )
}

/** The figure on the trailing edge: tabular, so it does not jitter as it counts. */
function Figure({ children }: { children: ReactNode }) {
  return <span className="shrink-0 text-xs tabular-nums text-fg-subtle">{children}</span>
}

/**
 * What the pane says when there is nothing to inspect.
 *
 * One line of quiet text, not the editors' centred icon-over-two-lines. That
 * empty state is sized for a document; at sidebar width it reads as a screen
 * that failed to load rather than as a pane waiting for a selection.
 */
function Note({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-0 flex-1 items-center justify-center px-6">
      <p className="text-center text-sm text-fg-subtle">{children}</p>
    </div>
  )
}

/**
 * The one irreversible thing this app does, asked for out loud.
 *
 * Not a guard — the operator has already decided, and `PublishVideo`'s own
 * refusal is what this walks through deliberately. It is here because pressing
 * it destroys the only record of where the current video lives: the app keeps
 * one upload receipt, the new one overwrites it, and nothing else remembers
 * the old URL. So the dialog's real job is to put that URL on screen, with a
 * copy button, at the last moment anything can.
 *
 * What it does not claim: that the old video is removed. YouTube has no way to
 * replace a video's file, so the first one stays up exactly as it is, and
 * tidying it is a job for YouTube rather than for this.
 */
function RepublishDialog({ video, onClose }: { video: Video; onClose: () => void }) {
  const client = useQueryClient()
  const [copied, setCopied] = useState(false)
  const url = video.upload?.url ?? ''

  const republish = useMutation({
    mutationFn: () => api.republishVideo(video.ref),
    onSuccess: () => {
      // The receipt is gone and the upload task has been reset; both are on the
      // video and its tasks, which every other pane reads.
      void client.invalidateQueries({ queryKey: qk.video(video.ref) })
      void client.invalidateQueries({ queryKey: qk.tasks(video.ref) })
      onClose()
    },
  })

  return (
    <Dialog open onOpenChange={(next) => (next ? undefined : onClose())} width={460}>
      <Dialog.Header
        title="Upload again?"
        description="This sends a new video to YouTube. The one already there is not replaced."
      />
      <Dialog.Body>
        <p className="text-sm leading-relaxed text-fg-muted">
          {video.ref} is published. YouTube cannot replace a video&rsquo;s file, so uploading again
          adds a second video and leaves the first one up — still public, still at its own address.
          This app will stop tracking it, so copy the link now if you mean to take it down.
        </p>
        <div
          className="mt-3 flex items-center gap-2 rounded-[7px] px-2.5 py-2"
          style={{ backgroundColor: 'var(--band)' }}
        >
          <span className="min-w-0 flex-1 truncate font-mono text-xs text-fg">
            {url || 'No URL was recorded'}
          </span>
          {url ? (
            <button
              type="button"
              aria-label={copied ? 'Copied' : 'Copy the link'}
              title={copied ? 'Copied' : 'Copy the link'}
              onClick={() => {
                void navigator.clipboard.writeText(url).then(() => {
                  setCopied(true)
                  setTimeout(() => setCopied(false), 1400)
                })
              }}
              className="flex size-[22px] shrink-0 items-center justify-center rounded-[6px] text-fg-subtle transition-colors hover:bg-[var(--hover)] hover:text-fg"
              style={copied ? { color: 'var(--done)' } : undefined}
            >
              {copied ? (
                <Check className="size-[13px]" strokeWidth={2.5} />
              ) : (
                <Copy className="size-[13px]" strokeWidth={2} />
              )}
            </button>
          ) : null}
        </div>
        {republish.error ? (
          <p className="mt-2 text-sm" style={{ color: 'var(--failed)' }}>
            {(republish.error as Error).message}
          </p>
        ) : null}
      </Dialog.Body>
      <Dialog.Footer>
        <Button
          className="ml-auto h-[26px] px-3.5"
          onClick={onClose}
          disabled={republish.isPending}
        >
          Cancel
        </Button>
        <Button
          primary
          className="h-[26px] px-3.5"
          onClick={() => republish.mutate()}
          disabled={republish.isPending}
        >
          {republish.isPending ? 'Uploading…' : 'Upload Again'}
        </Button>
      </Dialog.Footer>
    </Dialog>
  )
}
