import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'

import { Badge } from './ui/controls'
import { Button } from './ui/controls'
import { Field, Input, Select, Textarea } from './ui/controls'
import { ErrorNotice, Modal } from './ui/primitives'
import { useWorkbenchStore } from './lib/store'
import { api, qk } from '@/core/api'

/**
 * The workbench's own new-video dialog. Identical to the shell's apart from the
 * last line: it opens the new video as a document instead of navigating to a
 * route, which is the whole difference between staying in this window and being
 * thrown out of it.
 */
export function CreateVideo({
  open,
  onOpenChange,
  defaultChannel,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  defaultChannel?: string | undefined
}) {
  const queryClient = useQueryClient()
  const openDoc = useWorkbenchStore((s) => s.open)
  const channels = useQuery({ queryKey: qk.channels, queryFn: api.listChannels, enabled: open })

  const [channel, setChannel] = useState(defaultChannel ?? '')
  const [title, setTitle] = useState('')
  const [topic, setTopic] = useState('')
  const [chapterCount, setChapterCount] = useState('50')
  const [durationMinutes, setDurationMinutes] = useState('180')
  const [slidesPerChapter, setSlidesPerChapter] = useState('2')
  const [thumbnailCells, setThumbnailCells] = useState('12')
  const [start, setStart] = useState(true)
  // A stable key per dialog session makes a double submit a no-op.
  const [idempotencyKey, setIdempotencyKey] = useState(() => crypto.randomUUID())

  // Reopening from a different channel should follow the operator's context.
  useEffect(() => {
    if (open && defaultChannel) setChannel(defaultChannel)
  }, [open, defaultChannel])

  const create = useMutation({
    mutationFn: () =>
      api.createVideo(
        {
          channel: channel || channels.data?.[0]?.slug || '',
          title,
          topic,
          chapterCount: Number(chapterCount) || 0,
          targetDurationMinutes: Number(durationMinutes) || 0,
          slidesPerChapter: Number(slidesPerChapter) || 0,
          thumbnailCells: Number(thumbnailCells) || 0,
          start,
        },
        idempotencyKey,
      ),
    onSuccess: (video) => {
      void queryClient.invalidateQueries({ queryKey: ['videos'] })
      onOpenChange(false)
      setTitle('')
      setTopic('')
      setIdempotencyKey(crypto.randomUUID())
      // Pinned, not previewed: a video you just created is one you meant to open.
      openDoc({ kind: 'video', ref: video.ref }, { preview: false })
    },
  })

  const chapters = Number(chapterCount) || 0
  const slides = Number(slidesPerChapter) || 0
  const minutes = Number(durationMinutes) || 0
  const cells = Number(thumbnailCells) || 0
  // Mirrors scheduler.NodeCountFor: seven video-level tasks, four per chapter,
  // one per slide and one icon per thumbnail tile.
  const taskEstimate = 7 + 4 * chapters + chapters * slides + cells

  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title="New video"
      description="The blueprint is generated first, then paused for your review."
      footer={
        <>
          <span className="mr-auto text-[11.5px] text-subtle">
            About <span className="tabular font-medium text-muted">{taskEstimate}</span> tasks
            {minutes > 0 && (
              <>
                {' '}
                over <span className="tabular font-medium text-muted">{minutes}</span> min
              </>
            )}
          </span>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant="primary"
            disabled={!title.trim() || create.isPending}
            onClick={() => create.mutate()}
          >
            {create.isPending ? 'Creating…' : start ? 'Create and start' : 'Create'}
          </Button>
        </>
      }
    >
      <form
        className="space-y-4"
        onSubmit={(event) => {
          event.preventDefault()
          if (title.trim()) create.mutate()
        }}
      >
        <Field label="Channel" hint="Supplies the tone, the voice and the visual style.">
          {(id) => (
            <Select id={id} value={channel} onChange={(e) => setChannel(e.target.value)}>
              {channels.data?.map((c) => (
                <option key={c.id} value={c.slug}>
                  {c.name} ({c.slug})
                </option>
              ))}
            </Select>
          )}
        </Field>

        <Field label="Title">
          {(id) => (
            <Input
              id={id}
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="The Long Winter of the Harbour"
              autoFocus
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

        <div className="grid grid-cols-2 gap-3">
          <Field label="Target duration" hint="Minutes; 0 lets the chapter count decide">
            {(id) => (
              <Input
                id={id}
                type="number"
                min={0}
                max={720}
                value={durationMinutes}
                onChange={(e) => setDurationMinutes(e.target.value)}
              />
            )}
          </Field>
          <Field label="Chapters" hint="Target, 1–500">
            {(id) => (
              <Input
                id={id}
                type="number"
                min={1}
                max={500}
                value={chapterCount}
                onChange={(e) => setChapterCount(e.target.value)}
              />
            )}
          </Field>
          <Field label="Slides per chapter" hint="1–20">
            {(id) => (
              <Input
                id={id}
                type="number"
                min={1}
                max={20}
                value={slidesPerChapter}
                onChange={(e) => setSlidesPerChapter(e.target.value)}
              />
            )}
          </Field>
          {/* Fixed at creation: the DAG gets one icon task per tile and cannot
              change width afterwards. */}
          <Field label="Thumbnail tiles" hint="1–24; twelve is two rows of six">
            {(id) => (
              <Input
                id={id}
                type="number"
                min={1}
                max={24}
                value={thumbnailCells}
                onChange={(e) => setThumbnailCells(e.target.value)}
              />
            )}
          </Field>
        </div>

        <label className="flex cursor-pointer items-center gap-2 rounded-[var(--radius-sm)] border border-[hsl(var(--border))] bg-subtle px-3 py-2 text-[12.5px] text-fg">
          <input
            type="checkbox"
            checked={start}
            onChange={(e) => setStart(e.target.checked)}
            className="h-3.5 w-3.5 accent-[hsl(var(--accent))]"
          />
          Enqueue the DAG immediately
          <Badge tone={start ? 'accent' : 'neutral'} className="ml-auto">
            {start ? 'will start' : 'draft only'}
          </Badge>
        </label>

        {create.isError && <ErrorNotice error={create.error} />}
      </form>
    </Modal>
  )
}
