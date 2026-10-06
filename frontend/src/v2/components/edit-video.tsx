import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useId, useState } from 'react'
import { create } from 'zustand'
import { toast } from 'sonner'

import { api, qk } from '../core/api'
import type { Video } from '../core/types'
import { Button } from './ui/button'
import { Dialog } from './ui/dialog'
import { INDENT } from './ui/field'
import { BLANK_BRIEF, BriefFields, briefFrom, briefReady, requestFrom } from './video-brief'

/**
 * The edit-video dialog.
 *
 * The same six fields the new-video dialog asks for, prefilled, because they
 * are the same question asked a second time — a brief typed once, in a hurry,
 * that turned out to be wrong. A dialog rather than a document for the same
 * reason creating one is a dialog: you fill it in and it is over.
 *
 * No state gating. These are the video's information, not the pipeline's
 * controls: a chapter count on a graph that has already been laid out is a
 * record of what was planned, and an operator correcting it is not asking for a
 * rebuild. What each field reaches is whatever has not run yet.
 *
 * The store is here rather than in `store/workbench.ts` for the same reason the
 * new-video dialog's is: whoever opens this should not have to hold a boolean
 * for it, and an open dialog is not layout, so it has no business being
 * persisted.
 */

interface EditVideoState {
  /** The video being edited, and the whole of whether the dialog is open. */
  video: Video | null
  show: (video: Video) => void
  hide: () => void
}

const useEditVideo = create<EditVideoState>((set) => ({
  video: null,
  show: (video) => set({ video }),
  hide: () => set({ video: null }),
}))

/** Opens the dialog on a video. Bound to the sidebar row's Edit item. */
export function editVideo(video: Video): void {
  useEditVideo.getState().show(video)
}

export function EditVideoDialog() {
  // The dialog does not know about forms. This screen has both the form and the
  // button that submits it, so the id that associates them across the two
  // subtrees is minted here.
  const formId = useId()
  const video = useEditVideo((s) => s.video)
  const hide = useEditVideo((s) => s.hide)
  const client = useQueryClient()

  const [brief, setBrief] = useState(BLANK_BRIEF)

  // Refilled whenever the dialog is opened on a video, so a second edit starts
  // from what is saved rather than from the last draft typed.
  useEffect(() => {
    if (video) setBrief(briefFrom(video))
  }, [video])

  const save = useMutation({
    mutationFn: () => api.updateVideo(video?.ref ?? '', requestFrom(brief)),
    onSuccess: (saved) => {
      // The response is the whole row, so the open document is patched from it
      // rather than refetched; the list is asked again because nothing on the
      // event stream announces an edit.
      client.setQueryData(qk.video(saved.ref), saved)
      toast.success('Brief saved')
      void client.invalidateQueries({ queryKey: qk.videos })
      hide()
    },
  })

  const ready = briefReady(brief) && !save.isPending

  return (
    <Dialog
      open={Boolean(video)}
      onOpenChange={(next) => {
        if (!next) hide()
      }}
      width={640}
    >
      <Dialog.Header
        title="Edit video"
        description="Nothing is re-run. Each field reaches whatever has not happened yet."
      />
      <Dialog.Body>
        <form
          id={formId}
          onSubmit={(event) => {
            event.preventDefault()
            if (ready) save.mutate()
          }}
        >
          <BriefFields brief={brief} onChange={setBrief} />

          {save.error ? (
            <p className={`${INDENT} pt-3 text-sm text-[var(--failed)]`}>
              {(save.error as Error).message}
            </p>
          ) : null}
        </form>
      </Dialog.Body>
      <Dialog.Footer>
        {/* Which video this is. The title is in the field being edited — and may
            be being changed — so the ref is the only label here that still says
            what is about to be written. */}
        <span className="mr-auto text-xs tabular-nums text-fg-subtle">{video?.ref}</span>
        <Button className="h-[26px] px-3.5" onClick={hide}>
          Cancel
        </Button>
        <Button primary form={formId} type="submit" className="h-[26px] px-3.5" disabled={!ready}>
          {save.isPending ? 'Saving…' : 'Save'}
        </Button>
      </Dialog.Footer>
    </Dialog>
  )
}
