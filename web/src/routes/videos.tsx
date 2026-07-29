import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { AlertTriangle, Film, Plus } from 'lucide-react'
import { memo, useMemo, useState } from 'react'

import { PageHeader } from '@/components/app-shell'
import { VideoStateBadge } from '@/components/state-badges'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, Input, Select, Textarea } from '@/components/ui/field'
import {
  EmptyState,
  ErrorNotice,
  Modal,
  Progress,
  Skeleton,
  Tooltip,
} from '@/components/ui/primitives'
import { api, qk } from '@/lib/api'
import { formatRelative, percent, poolLabel } from '@/lib/format'
import type { Channel, PoolStat, Video } from '@/lib/types'
import { cn } from '@/lib/utils'

const STATE_FILTERS: { value: string; label: string }[] = [
  { value: '', label: 'All states' },
  { value: 'draft', label: 'Draft' },
  { value: 'running', label: 'Running' },
  { value: 'awaiting_approval', label: 'Awaiting approval' },
  { value: 'blocked', label: 'Blocked' },
  { value: 'completed', label: 'Completed' },
  { value: 'failed', label: 'Failed' },
  { value: 'cancelled', label: 'Cancelled' },
]

export function VideosRoute() {
  const [state, setState] = useState('')
  const [channelId, setChannelId] = useState('')
  const [creating, setCreating] = useState(false)

  const params = useMemo(
    () => ({ ...(state ? { state } : {}), ...(channelId ? { channelId } : {}) }),
    [state, channelId],
  )

  const videos = useQuery({
    queryKey: qk.videos(params),
    queryFn: () => api.listVideos(params),
  })
  const channels = useQuery({ queryKey: qk.channels, queryFn: api.listChannels })
  const scheduler = useQuery({ queryKey: qk.scheduler, queryFn: api.schedulerStatus })

  const bottleneck = useMemo(() => findBottleneck(scheduler.data?.pools), [scheduler.data])

  return (
    <>
      <PageHeader
        title="Videos"
        subtitle={
          videos.data
            ? `${videos.data.total} video${videos.data.total === 1 ? '' : 's'}${
                bottleneck ? ` · ${poolLabel(bottleneck.pool)} pool is saturated` : ''
              }`
            : 'Loading…'
        }
        actions={
          <>
            <Select
              value={channelId}
              onChange={(e) => setChannelId(e.target.value)}
              aria-label="Filter by channel"
              className="w-44"
            >
              <option value="">All channels</option>
              {channels.data?.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </Select>
            <Select
              value={state}
              onChange={(e) => setState(e.target.value)}
              aria-label="Filter by state"
              className="w-40"
            >
              {STATE_FILTERS.map((f) => (
                <option key={f.value} value={f.value}>
                  {f.label}
                </option>
              ))}
            </Select>
            <Button variant="primary" size="md" onClick={() => setCreating(true)}>
              <Plus className="h-3.5 w-3.5" />
              New video
            </Button>
          </>
        }
      />

      <div className="min-h-0 flex-1 overflow-y-auto">
        {videos.isPending && (
          <div className="space-y-2 p-4">
            {Array.from({ length: 5 }, (_, i) => (
              <Skeleton key={i} className="h-16 w-full" />
            ))}
          </div>
        )}
        {videos.isError && <ErrorNotice error={videos.error} className="m-4" />}
        {videos.data?.videos.length === 0 && (
          <EmptyState
            icon={<Film />}
            title="No videos yet"
            description="A video is a channel, a title and a chapter count. Everything else is generated."
            action={
              <Button variant="primary" onClick={() => setCreating(true)}>
                <Plus className="h-3.5 w-3.5" />
                New video
              </Button>
            }
          />
        )}
        {videos.data && videos.data.videos.length > 0 && (
          <ul className="divide-y divide-[hsl(var(--border))]">
            {videos.data.videos.map((video) => (
              <VideoRow
                key={video.id}
                video={video}
                channel={channels.data?.find((c) => c.id === video.channelId)}
              />
            ))}
          </ul>
        )}
      </div>

      <CreateVideoDialog
        open={creating}
        onOpenChange={setCreating}
        channels={channels.data ?? []}
      />
    </>
  )
}

/** Memoised so a delta for one video re-renders only its own row (§9). */
const VideoRow = memo(function VideoRow({
  video,
  channel,
}: {
  video: Video
  channel: Channel | undefined
}) {
  const done = percent(video.counts.succeeded, video.counts.total)
  return (
    <li>
      <Link
        to="/videos/$ref"
        params={{ ref: video.ref }}
        className="flex items-center gap-4 px-4 py-3 transition-colors hover:bg-[hsl(var(--bg-hover))]"
      >
        <div className="w-[74px] shrink-0">
          <span className="font-mono text-[12px] font-semibold text-[hsl(var(--accent))]">
            {video.ref}
          </span>
        </div>

        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate text-[13px] font-medium text-fg">{video.title}</span>
            {video.counts.failed > 0 && (
              <Tooltip label={`${video.counts.failed} failed task(s)`}>
                <AlertTriangle className="h-3.5 w-3.5 shrink-0 text-[hsl(var(--danger))]" />
              </Tooltip>
            )}
          </div>
          <div className="mt-0.5 flex items-center gap-2 text-[11.5px] text-subtle">
            {channel && <span className="truncate">{channel.name}</span>}
            <span aria-hidden>·</span>
            <span className="tabular">
              {video.chapterCount} chapters × {video.imagesPerChapter} stills
            </span>
            <span aria-hidden>·</span>
            <span>{formatRelative(video.updatedAt)}</span>
          </div>
        </div>

        <div className="w-56 shrink-0">
          <Progress
            value={video.counts.succeeded}
            total={video.counts.total}
            failed={video.counts.failed}
            running={video.state === 'running'}
            aria-label={`${done}% complete`}
          />
          <div className="mt-1 flex justify-between text-[11px] tabular text-subtle">
            <span>
              {video.counts.succeeded}/{video.counts.total}
            </span>
            <span>{done}%</span>
          </div>
        </div>

        <div className="w-40 shrink-0 text-right">
          <VideoStateBadge state={video.state} />
        </div>
      </Link>
    </li>
  )
})

function findBottleneck(pools: PoolStat[] | undefined): PoolStat | undefined {
  if (!pools) return undefined
  let worst: PoolStat | undefined
  for (const pool of pools) {
    if (pool.inFlight >= pool.limit && pool.queued > 0) {
      if (!worst || pool.queued > worst.queued) worst = pool
    }
  }
  return worst
}

function CreateVideoDialog({
  open,
  onOpenChange,
  channels,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  channels: Channel[]
}) {
  const queryClient = useQueryClient()
  const [channel, setChannel] = useState('')
  const [title, setTitle] = useState('')
  const [topic, setTopic] = useState('')
  const [chapterCount, setChapterCount] = useState('50')
  const [imagesPerChapter, setImagesPerChapter] = useState('2')
  const [start, setStart] = useState(true)
  // A stable key per dialog session makes a double submit a no-op (§9).
  const [idempotencyKey, setIdempotencyKey] = useState(() => crypto.randomUUID())

  const create = useMutation({
    mutationFn: () =>
      api.createVideo(
        {
          channel: channel || channels[0]?.slug || '',
          title,
          topic,
          chapterCount: Number(chapterCount) || 0,
          imagesPerChapter: Number(imagesPerChapter) || 0,
          start,
        },
        idempotencyKey,
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['videos'] })
      onOpenChange(false)
      setTitle('')
      setTopic('')
      setIdempotencyKey(crypto.randomUUID())
    },
  })

  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title="New video"
      description="The blueprint is generated first, then paused for your review."
      footer={
        <>
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
        <Field label="Channel">
          {(id) => (
            <Select id={id} value={channel} onChange={(e) => setChannel(e.target.value)}>
              {channels.map((c) => (
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

        <div className="grid grid-cols-2 gap-3">
          <Field label="Chapters" hint="1–500">
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

        <label className="flex cursor-pointer items-center gap-2 text-[12.5px] text-fg">
          <input
            type="checkbox"
            checked={start}
            onChange={(e) => setStart(e.target.checked)}
            className="h-3.5 w-3.5 accent-[hsl(var(--accent))]"
          />
          Enqueue the DAG immediately
        </label>

        <div className={cn('flex items-center gap-2 text-[11.5px] text-subtle')}>
          <Badge tone="neutral">
            {Number(chapterCount) || 0} chapters ·{' '}
            {5 +
              4 * (Number(chapterCount) || 0) +
              (Number(chapterCount) || 0) * (Number(imagesPerChapter) || 0)}{' '}
            tasks
          </Badge>
        </div>

        {create.isError && <ErrorNotice error={create.error} />}
      </form>
    </Modal>
  )
}
