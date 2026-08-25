import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { create } from 'zustand'

import { api, qk } from '../core/api'
import { count } from '../core/format'
import { openDoc } from './editor/dock'
import { Button } from './ui/button'
import {
  Checkbox,
  Field,
  FieldDivider,
  INDENT,
  Input,
  NumberField,
  Select,
  Textarea,
} from './ui/field'
import { Dialog } from './ui/dialog'

/**
 * The new-video dialog.
 *
 * A dialog rather than a document, because creating a video is a question with
 * an answer — you fill it in and it is over — where a document is a place you
 * come back to. The video it produces is the document.
 *
 * Two groups, one rule between them: what the video is *about*, and how big it
 * is. The first three fields are prose and want the width; the last four are
 * numbers and want none of it. That is the whole layout, and it is why the
 * numeric fields are sized to three digits with their unit beside them rather
 * than stretched across the dialog.
 *
 * The store is here rather than in `store/workbench.ts` for the same reason
 * `openDoc` lives beside the dock: whoever opens this — a menu, a keystroke, a
 * channel row — should not have to hold a boolean for it, and an open dialog is
 * not layout, so it has no business being persisted.
 */

interface NewVideoState {
  open: boolean
  /** The channel the request came from, if it came from one. */
  channel: string | undefined
  show: (channel?: string) => void
  hide: () => void
}

const useNewVideo = create<NewVideoState>((set) => ({
  open: false,
  channel: undefined,
  show: (channel) => set({ open: true, channel }),
  hide: () => set({ open: false }),
}))

/** Opens the dialog. Bound to ⌘N and to the sidebar's create menu. */
export function newVideo(channel?: string): void {
  useNewVideo.getState().show(channel)
}

export function NewVideoDialog() {
  const open = useNewVideo((s) => s.open)
  const from = useNewVideo((s) => s.channel)
  const hide = useNewVideo((s) => s.hide)

  const client = useQueryClient()
  const channels = useQuery({ queryKey: qk.channels, queryFn: api.listChannels, enabled: open })

  const [channel, setChannel] = useState('')
  const [title, setTitle] = useState('')
  const [topic, setTopic] = useState('')
  const [chapterCount, setChapterCount] = useState('50')
  const [durationMinutes, setDurationMinutes] = useState('180')
  const [slidesPerChapter, setSlidesPerChapter] = useState('2')
  const [thumbnailCells, setThumbnailCells] = useState('12')
  const [start, setStart] = useState(true)
  // Minted per dialog session, so a double submit is a no-op rather than a
  // second video — including the resubmit of a request that timed out on the
  // way back after the server had already made one.
  const [idempotencyKey, setIdempotencyKey] = useState(() => crypto.randomUUID())

  // Opening from a channel row should follow the operator there.
  useEffect(() => {
    if (open && from) setChannel(from)
  }, [open, from])

  const submit = useMutation({
    mutationFn: () =>
      api.createVideo(
        {
          channel: channel || channels.data?.[0]?.slug || '',
          title: title.trim(),
          topic: topic.trim(),
          chapterCount: Number(chapterCount) || 0,
          targetDurationMinutes: Number(durationMinutes) || 0,
          slidesPerChapter: Number(slidesPerChapter) || 0,
          thumbnailCells: Number(thumbnailCells) || 0,
          start,
        },
        idempotencyKey,
      ),
    onSuccess: (video) => {
      void client.invalidateQueries({ queryKey: qk.videos })
      const owner = (channels.data ?? []).find((c) => c.id === video.channelId)
      hide()
      setTitle('')
      setTopic('')
      setIdempotencyKey(crypto.randomUUID())
      // Pinned, not previewed: a video you just created is one you meant to open.
      openDoc({ kind: 'video', ref: video.ref }, video.title || video.ref, {
        seed: owner?.slug,
        initial: owner?.name,
      })
    },
  })

  const chapters = Number(chapterCount) || 0
  const slides = Number(slidesPerChapter) || 0
  const minutes = Number(durationMinutes) || 0
  const cells = Number(thumbnailCells) || 0
  // Mirrors scheduler.NodeCountFor: seven video-level tasks, four per chapter,
  // one per slide, and one icon per thumbnail tile.
  const tasks = 7 + 4 * chapters + chapters * slides + cells

  const ready = title.trim().length > 0 && !submit.isPending

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) hide()
      }}
      title="New video"
      description="The blueprint is written first, then paused for your review."
      footer={(formId) => (
        <>
          <span className="mr-auto text-[11px] text-tertiary">
            About <span className="font-medium tabular-nums">{count(tasks)}</span> tasks
            {minutes > 0 ? (
              <>
                {' '}
                over <span className="font-medium tabular-nums">{minutes}</span> min
              </>
            ) : null}
          </span>
          <Button className="h-[26px] px-3.5" onClick={hide}>
            Cancel
          </Button>
          <Button primary form={formId} type="submit" className="h-[26px] px-3.5" disabled={!ready}>
            {submit.isPending ? 'Creating…' : start ? 'Create and Start' : 'Create'}
          </Button>
        </>
      )}
    >
      {(formId) => (
        <form
          id={formId}
          onSubmit={(event) => {
            event.preventDefault()
            if (ready) submit.mutate()
          }}
        >
          <Field label="Channel" hint="Supplies the tone, the voice and the visual style.">
            {(id) => (
              <Select id={id} value={channel} onChange={(e) => setChannel(e.target.value)}>
                {(channels.data ?? []).map((c) => (
                  <option key={c.id} value={c.slug}>
                    {c.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>

          <Field label="Title">
            {(id) => (
              <Input
                id={id}
                data-autofocus
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                placeholder="The Long Winter of the Harbour"
              />
            )}
          </Field>

          <Field label="Topic" hint="Steers the blueprint, the scripts and the slide prompts.">
            {(id) => (
              <Textarea
                id={id}
                rows={3}
                value={topic}
                onChange={(e) => setTopic(e.target.value)}
                placeholder="A northern port town over one winter, told through its shipping ledgers."
              />
            )}
          </Field>

          <FieldDivider />

          <NumberField
            label="Duration"
            unit={minutes > 0 ? 'minutes' : 'minutes — the chapter count decides'}
            value={durationMinutes}
            onChange={setDurationMinutes}
            min={0}
            max={720}
          />
          <NumberField
            label="Chapters"
            unit="a target, 1–500"
            value={chapterCount}
            onChange={setChapterCount}
            min={1}
            max={500}
          />
          <NumberField
            label="Slides"
            unit="per chapter, 1–20"
            value={slidesPerChapter}
            onChange={setSlidesPerChapter}
            min={1}
            max={20}
          />
          {/* Fixed at creation: the DAG gets one icon task per tile and cannot
              change width afterwards. */}
          <NumberField
            label="Thumbnail"
            unit="tiles, 1–24 — twelve is two rows of six"
            value={thumbnailCells}
            onChange={setThumbnailCells}
            min={1}
            max={24}
          />

          <FieldDivider />

          <div className={INDENT}>
            <Checkbox checked={start} onChange={setStart}>
              Start the pipeline immediately
            </Checkbox>
          </div>

          {submit.error ? (
            <p className={`${INDENT} pt-2 text-[12px] text-[var(--failed)]`}>
              {(submit.error as Error).message}
            </p>
          ) : null}
        </form>
      )}
    </Dialog>
  )
}
