import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, Input, Select, Textarea } from '@/components/ui/field'
import { ErrorNotice, Modal } from '@/components/ui/primitives'
import { api, qk } from '@/lib/api'

/**
 * Creating a video is the one destructive-ish action in the application, so it
 * is a dialog rather than an inline form: it states what it is about to enqueue
 * before it enqueues it, and it opens the new video the moment it exists.
 */
export function CreateVideoDialog({
  open,
  onOpenChange,
  defaultChannel,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Pre-selects the channel the operator was looking at. */
  defaultChannel?: string
}) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const channels = useQuery({ queryKey: qk.channels, queryFn: api.listChannels, enabled: open })

  const [channel, setChannel] = useState(defaultChannel ?? '')
  const [title, setTitle] = useState('')
  const [topic, setTopic] = useState('')
  const [chapterCount, setChapterCount] = useState('50')
  const [durationMinutes, setDurationMinutes] = useState('180')
  const [imagesPerChapter, setImagesPerChapter] = useState('2')
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
          imagesPerChapter: Number(imagesPerChapter) || 0,
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
      void navigate({ to: '/videos/$ref', params: { ref: video.ref } })
    },
  })

  const chapters = Number(chapterCount) || 0
  const stills = Number(imagesPerChapter) || 0
  const minutes = Number(durationMinutes) || 0
  const taskEstimate = 5 + 4 * chapters + chapters * stills

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
            , settled when you approve the blueprint
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
        onSubmit={(e) => {
          e.preventDefault()
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

        <Field label="Topic" hint="Steers the blueprint, the scripts and the image prompts.">
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

        <Field
          label="Target duration"
          hint="Minutes. The blueprint budgets words to fill it; 0 lets the chapter count decide."
        >
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

        <div className="grid grid-cols-2 gap-3">
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
          <Field label="Stills per chapter" hint="1–20">
            {(id) => (
              <Input
                id={id}
                type="number"
                min={1}
                max={20}
                value={imagesPerChapter}
                onChange={(e) => setImagesPerChapter(e.target.value)}
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
