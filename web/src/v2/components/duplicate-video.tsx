import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { create } from 'zustand'

import { api, qk } from '../core/api'
import { uuid } from '../core/utils'
import type { Channel, Video } from '../core/types'
import { useWorkbench } from '../store/workbench'
import { openDoc } from './editor/dock'
import { Button } from './ui/button'
import { Dialog } from './ui/dialog'

/**
 * Duplicate: a new draft carrying the same brief, and nothing else.
 *
 * The seven fields copied are exactly the ones the create endpoint takes — the
 * channel, the title, the topic and the four numbers that size the graph. That
 * is not a filter written here and kept in step with the video record; it is
 * what going back through the front door gets you. Everything the pipeline
 * produced — the blueprint, the chapters, the scripts, the thumbnail plan and
 * its icons, the render, the listing, the upload — is absent because a new
 * video has never had any of it. A clone that copied those would be claiming
 * work that was done for a different video.
 *
 * There is no dialog. The point is to get a second run at an idea moving in one
 * gesture, and a form asking you to confirm fields you deliberately said to
 * leave alone is the gesture undone. The copy is a draft, so the thing you edit
 * before it costs anything is the video itself.
 *
 * The action is a plain function over a store, the way `openDoc` is, because
 * two callers reach it — the row's menu and ⌘D — and threading a mutation from
 * the sidebar to the keymap would make the keystroke a worse version of the
 * menu item. The host below is what registers the hooks half.
 */

interface DuplicateState {
  /** Registered by the host once it has mounted; null before that. */
  run: ((videos: Video[]) => void) | null
  /** What went wrong, if anything did. Cleared by dismissing the sheet. */
  error: string | null
  setRun: (run: ((videos: Video[]) => void) | null) => void
  setError: (error: string | null) => void
}

const useDuplicate = create<DuplicateState>((set) => ({
  run: null,
  error: null,
  setRun: (run) => set({ run }),
  setError: (error) => set({ error }),
}))

/** Duplicates each video as a fresh draft. Bound to ⌘D and to the row menu. */
export function duplicateVideos(videos: Video[]): void {
  if (videos.length === 0) return
  useDuplicate.getState().run?.(videos)
}

/** What the menu calls this, so the label and the action cannot disagree. */
export function duplicateLabel(count: number): string {
  return count > 1 ? `Duplicate ${count} Videos` : 'Duplicate'
}

/**
 * The hooks half of the action, plus the only thing it can put on screen.
 *
 * Mounted once at the workbench root beside the new-video dialog. It renders
 * nothing until something fails: a duplicate that worked announces itself by
 * the row appearing and the document opening, and a sheet saying so would be a
 * second, worse copy of that news.
 */
export function DuplicateVideoHost() {
  const client = useQueryClient()
  const setRun = useDuplicate((s) => s.setRun)
  const error = useDuplicate((s) => s.error)
  const setError = useDuplicate((s) => s.setError)

  const duplicate = useMutation({
    // One request each, and settled rather than all: five duplications are five
    // chances to fail, and the four that worked should still exist. The same
    // shape the delete beside it uses, for the same reason.
    //
    // A key per request, not per gesture. Idempotency is what stops one
    // duplication becoming two when a response is lost on the way back; it is
    // not meant to stop you pressing ⌘D twice, which is how you ask for two.
    mutationFn: async (videos: Video[]) => {
      const settled = await Promise.allSettled(
        videos.map((video) =>
          api.createVideo(
            {
              // The endpoint resolves a channel by id or slug and tries the id
              // first, so the row's own `channelId` is the whole answer — no
              // lookup, and nothing to be stale about.
              channel: video.channelId,
              title: video.title,
              topic: video.topic,
              chapterCount: video.chapterCount,
              targetDurationMinutes: video.targetDurationMinutes,
              slidesPerChapter: video.slidesPerChapter,
              thumbnailCells: video.thumbnailCells,
              // Never. The copy exists so the brief can be changed before it
              // spends anything, and a run started on the unedited original is
              // the one outcome nobody duplicated a video to get.
              start: false,
            },
            uuid(),
          ),
        ),
      )
      const made: Video[] = []
      const failed: PromiseRejectedResult[] = []
      for (const result of settled) {
        if (result.status === 'fulfilled') made.push(result.value)
        else failed.push(result)
      }
      return { made, failed, asked: videos.length }
    },
    onSuccess: ({ made, failed, asked }) => {
      if (made.length > 0) {
        // Nothing on the stream announces a video appearing — the deltas are
        // about work happening — so the list is asked again.
        void client.invalidateQueries({ queryKey: qk.videos })
        useWorkbench.getState().select(made.map((video) => video.ref))
      }

      // One copy is a thing you want to look at; six is a batch, and opening
      // six tabs is not what "duplicate these" asked for.
      const only = made.length === 1 ? made[0] : undefined
      if (only) {
        // Best-effort, and cosmetic: the seed is what gives the tab its
        // coloured token. Read from the cache rather than fetched, because a
        // cold cache should cost the duplicate nothing.
        const channels = client.getQueryData<Channel[]>(qk.channels) ?? []
        const owner = channels.find((channel) => channel.id === only.channelId)
        openDoc({ kind: 'video', ref: only.ref }, only.title || 'Untitled', {
          ...(owner ? { seed: owner.slug, initial: owner.name } : {}),
        })
      }

      if (failed.length === 0) return
      const first = failed[0]?.reason
      const reason = first instanceof Error ? first.message : 'the server refused'
      setError(
        asked === 1
          ? `Could not duplicate — ${reason}`
          : `${failed.length} of ${asked} could not be duplicated — ${reason}`,
      )
    },
    onError: (err: Error) => setError(`Could not duplicate — ${err.message}`),
  })

  // Registered rather than passed. `mutate` is stable, so this runs once; the
  // cleanup matters only for a remount under StrictMode.
  const { mutate } = duplicate
  useEffect(() => {
    setRun(mutate)
    return () => setRun(null)
  }, [setRun, mutate])

  if (!error) return null
  return (
    <Dialog open onOpenChange={() => setError(null)} width={400}>
      <Dialog.Header title="Duplicate failed" description={error} />
      <Dialog.Footer>
        <span className="mr-auto" />
        <Button primary className="h-[26px] px-3.5" onClick={() => setError(null)}>
          OK
        </Button>
      </Dialog.Footer>
    </Dialog>
  )
}
