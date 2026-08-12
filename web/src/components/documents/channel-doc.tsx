import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, Plus, Tv } from 'lucide-react'
import { useEffect, useState } from 'react'

import { DocFrame } from '../editor/doc-frame'
import { useWorkbenchStore } from '../lib/store'
import { Badge, Button, Field, Input, Textarea, type Tone } from '../ui/controls'
import {
  EmptyState,
  ErrorNotice,
  KeyValue,
  Mono,
  Section,
  Skeleton,
  Tooltip,
} from '../ui/primitives'
import { VideoStateDot } from '../ui/status'
import { api, qk } from '@/core/api'
import { formatAbsolute, formatRelative, videoStateLabel } from '@/core/format'
import type { Channel } from '@/core/types'

const CREDENTIAL_TONES: Record<Channel['credentials'], Tone> = {
  valid: 'success',
  expired: 'warning',
  missing: 'neutral',
}

/**
 * The channel document — what the `/channels` page used to be, minus the grid of
 * cards. The explorer already lists every channel, so a page whose whole job was
 * to list them again was duplicating the sidebar.
 */
export function ChannelDoc({
  slug,
  onNewVideo,
  onDirty,
}: {
  slug: string
  onNewVideo: (slug: string) => void
  /** Reported upward so the tab pins itself and refuses to close silently. */
  onDirty: (dirty: boolean) => void
}) {
  const open = useWorkbenchStore((s) => s.open)
  const queryClient = useQueryClient()

  const channel = useQuery({ queryKey: qk.channel(slug), queryFn: () => api.getChannel(slug) })
  const videos = useQuery({ queryKey: qk.videos({}), queryFn: () => api.listVideos({}) })

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')

  // Seeded once the server answers, and re-seeded when the document moves to a
  // different channel: the form is a draft over one row.
  useEffect(() => {
    if (!channel.data) return
    setName(channel.data.name)
    setDescription(channel.data.description)
  }, [channel.data])

  const dirty = Boolean(
    channel.data && (name !== channel.data.name || description !== channel.data.description),
  )
  useEffect(() => {
    onDirty(dirty)
    // Leaving the document must not leave the tab marked dirty forever.
    return () => onDirty(false)
  }, [dirty, onDirty])

  const save = useMutation({
    mutationFn: () => api.updateChannel(slug, { name, description, style: {} }),
    onSuccess: (updated) => {
      queryClient.setQueryData(qk.channel(slug), updated)
      void queryClient.invalidateQueries({ queryKey: qk.channels })
    },
  })

  if (channel.isPending) {
    return (
      <DocFrame crumbs={['Channels', slug]}>
        <div className="space-y-3 p-4">
          <Skeleton className="h-24 w-full" />
        </div>
      </DocFrame>
    )
  }
  if (channel.isError || !channel.data) {
    return (
      <DocFrame crumbs={['Channels', slug]}>
        <ErrorNotice error={channel.error ?? new Error('channel not found')} className="m-4" />
      </DocFrame>
    )
  }

  const c = channel.data
  const mine = (videos.data?.videos ?? []).filter((v) => v.channelId === c.id)

  return (
    <DocFrame
      crumbs={['Channels', c.name]}
      actions={
        <>
          <Tooltip
            label={
              c.credentials === 'valid'
                ? 'Upload credentials are good'
                : c.credentials === 'expired'
                  ? 'Credentials have expired — re-authorise before the upload gate'
                  : 'No upload credentials on file'
            }
          >
            <span>
              <Badge tone={CREDENTIAL_TONES[c.credentials]} dot>
                <KeyRound className="h-3 w-3" aria-hidden />
                {c.credentials}
              </Badge>
            </span>
          </Tooltip>
          <Button variant="ghost" size="xs" onClick={() => onNewVideo(c.slug)}>
            <Plus className="h-3 w-3" />
            New video
          </Button>
          {dirty && (
            <Button
              variant="primary"
              size="xs"
              disabled={save.isPending}
              onClick={() => save.mutate()}
            >
              {save.isPending ? 'Saving…' : 'Save'}
            </Button>
          )}
        </>
      }
    >
      <div className="h-full overflow-y-auto p-4">
        <div className="mx-auto max-w-3xl space-y-5">
          <Section title="Identity">
            <div className="space-y-3">
              <Field label="Name">
                {(id) => (
                  <Input id={id} value={name} onChange={(event) => setName(event.target.value)} />
                )}
              </Field>
              <Field
                label="Description"
                hint="Steers the blueprint and the visual direction of every video on this channel."
              >
                {(id) => (
                  <Textarea
                    id={id}
                    rows={4}
                    value={description}
                    onChange={(event) => setDescription(event.target.value)}
                  />
                )}
              </Field>
              {save.isError && <ErrorNotice error={save.error} />}
            </div>
          </Section>

          <Section title="Facts">
            <dl className="surface divide-y divide-[hsl(var(--border))] px-3">
              {/* The slug is chosen once and never changes: it is the prefix of
                  every ref this channel has issued. */}
              <KeyValue label="Slug">
                <Mono>{c.slug}</Mono>
              </KeyValue>
              <KeyValue label="Videos">{mine.length}</KeyValue>
              <KeyValue label="Next ref">
                <Mono>{`${c.slug.toUpperCase()}-${String(c.videoSeq + 1).padStart(3, '0')}`}</Mono>
              </KeyValue>
              <KeyValue label="Created">{formatAbsolute(c.createdAt)}</KeyValue>
              <KeyValue label="Updated">{formatAbsolute(c.updatedAt)}</KeyValue>
            </dl>
          </Section>

          <Section title={`Videos (${mine.length})`}>
            {mine.length === 0 ? (
              <EmptyState
                icon={<Tv />}
                title="Nothing on this channel yet"
                action={
                  <Button variant="primary" size="sm" onClick={() => onNewVideo(c.slug)}>
                    <Plus className="h-3.5 w-3.5" />
                    New video
                  </Button>
                }
              />
            ) : (
              <ul className="surface divide-y divide-[hsl(var(--border))]">
                {[...mine]
                  .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
                  .map((video) => (
                    <li key={video.id}>
                      <button
                        type="button"
                        onClick={() => open({ kind: 'video', ref: video.ref }, { preview: false })}
                        className="flex w-full items-center gap-2.5 px-3 py-1.5 text-left transition-colors hover:bg-[hsl(var(--bg-hover))]"
                      >
                        <VideoStateDot state={video.state} />
                        <span className="shrink-0 font-mono text-[10.5px] font-semibold text-subtle">
                          {video.ref}
                        </span>
                        <span className="min-w-0 flex-1 truncate text-[12.5px] text-fg/90">
                          {video.title}
                        </span>
                        <span className="shrink-0 text-[11px] text-subtle">
                          {videoStateLabel(video.state)}
                        </span>
                        <span className="tabular w-16 shrink-0 text-right text-[10.5px] text-subtle">
                          {formatRelative(video.updatedAt)}
                        </span>
                      </button>
                    </li>
                  ))}
              </ul>
            )}
          </Section>
        </div>
      </div>
    </DocFrame>
  )
}
