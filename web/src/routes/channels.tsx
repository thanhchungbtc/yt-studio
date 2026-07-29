import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, Plus, Tv } from 'lucide-react'
import { useState } from 'react'

import { PageHeader } from '@/components/app-shell'
import { Badge, type Tone } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, Input, Select, Textarea } from '@/components/ui/field'
import {
  EmptyState,
  ErrorNotice,
  KeyValue,
  Modal,
  Mono,
  Panel,
  PanelHeader,
  PanelTitle,
  Skeleton,
  Tooltip,
} from '@/components/ui/primitives'
import { api, qk } from '@/lib/api'
import { formatAbsolute } from '@/lib/format'
import type { Channel } from '@/lib/types'

const CREDENTIAL_TONES: Record<Channel['credentials'], Tone> = {
  valid: 'success',
  expired: 'warning',
  missing: 'neutral',
}

export function ChannelsRoute() {
  const channels = useQuery({ queryKey: qk.channels, queryFn: api.listChannels })
  const [editing, setEditing] = useState<Channel | null>(null)
  const [creating, setCreating] = useState(false)

  return (
    <>
      <PageHeader
        title="Channels"
        subtitle="Identity, creative direction and upload credentials. The slug is chosen once and never changes."
        actions={
          <Button variant="primary" onClick={() => setCreating(true)}>
            <Plus className="h-3.5 w-3.5" />
            New channel
          </Button>
        }
      />

      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        {channels.isPending && (
          <div className="grid grid-cols-[repeat(auto-fill,minmax(340px,1fr))] gap-3">
            {Array.from({ length: 3 }, (_, i) => (
              <Skeleton key={i} className="h-56" />
            ))}
          </div>
        )}
        {channels.isError && <ErrorNotice error={channels.error} />}
        {channels.data?.length === 0 && (
          <EmptyState
            icon={<Tv />}
            title="No channels"
            description="A channel carries the voice, the visual style and the credentials every video inherits."
            action={
              <Button variant="primary" onClick={() => setCreating(true)}>
                New channel
              </Button>
            }
          />
        )}
        <div className="grid grid-cols-[repeat(auto-fill,minmax(340px,1fr))] gap-3">
          {channels.data?.map((channel) => (
            <Panel key={channel.id} className="flex flex-col">
              <PanelHeader>
                <div className="min-w-0">
                  <PanelTitle className="normal-case tracking-normal text-[13px] text-fg">
                    {channel.name}
                  </PanelTitle>
                  <Mono className="text-subtle">{channel.slug}</Mono>
                </div>
                <Tooltip label={`YouTube credentials: ${channel.credentials}`}>
                  <span>
                    <Badge tone={CREDENTIAL_TONES[channel.credentials]} dot>
                      <KeyRound className="h-3 w-3" />
                      {channel.credentials}
                    </Badge>
                  </span>
                </Tooltip>
              </PanelHeader>

              <div className="flex-1 px-3 py-2">
                {channel.description && (
                  <p className="mb-2 text-[12px] text-muted">{channel.description}</p>
                )}
                <dl>
                  <KeyValue label="Tone">{channel.style.tone || '—'}</KeyValue>
                  <KeyValue label="Voice">{channel.style.voice || '—'}</KeyValue>
                  <KeyValue label="Image style">{channel.style.imageStyle || '—'}</KeyValue>
                  <KeyValue label="Language">{channel.style.language}</KeyValue>
                  <KeyValue label="Words / chapter">{channel.style.wordsPerChapter}</KeyValue>
                  <KeyValue label="Videos minted">{channel.videoSeq}</KeyValue>
                  <KeyValue label="Created">{formatAbsolute(channel.createdAt)}</KeyValue>
                </dl>
              </div>

              <div className="flex justify-end gap-2 border-t border-[hsl(var(--border))] px-3 py-2">
                <Button size="sm" variant="ghost" onClick={() => setEditing(channel)}>
                  Edit
                </Button>
              </div>
            </Panel>
          ))}
        </div>
      </div>

      <ChannelDialog
        open={creating}
        onOpenChange={setCreating}
        onClose={() => setCreating(false)}
      />
      {editing && (
        <ChannelDialog
          open
          channel={editing}
          onOpenChange={(open) => !open && setEditing(null)}
          onClose={() => setEditing(null)}
        />
      )}
    </>
  )
}

function ChannelDialog({
  open,
  channel,
  onOpenChange,
  onClose,
}: {
  open: boolean
  channel?: Channel
  onOpenChange: (open: boolean) => void
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const [name, setName] = useState(channel?.name ?? '')
  const [slug, setSlug] = useState(channel?.slug ?? '')
  const [description, setDescription] = useState(channel?.description ?? '')
  const [tone, setTone] = useState(channel?.style.tone ?? '')
  const [voice, setVoice] = useState(channel?.style.voice ?? '')
  const [imageStyle, setImageStyle] = useState(channel?.style.imageStyle ?? '')
  const [language, setLanguage] = useState(channel?.style.language ?? 'en-US')
  const [words, setWords] = useState(String(channel?.style.wordsPerChapter ?? 450))
  const [credentials, setCredentials] = useState(channel?.credentials ?? 'missing')

  const save = useMutation({
    mutationFn: () => {
      const style = {
        tone,
        voice,
        imageStyle,
        language,
        wordsPerChapter: Number(words) || 0,
      }
      return channel
        ? api.updateChannel(channel.slug, { name, description, style, credentials })
        : api.createChannel({ ...(slug ? { slug } : {}), name, description, style })
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: qk.channels })
      onClose()
    },
  })

  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title={channel ? `Edit ${channel.name}` : 'New channel'}
      description={
        channel
          ? 'The slug is immutable — renaming the display name does not touch it.'
          : 'A blank slug is derived from the name.'
      }
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            disabled={!name.trim() || save.isPending}
            onClick={() => save.mutate()}
          >
            {save.isPending ? 'Saving…' : 'Save'}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="Name">
          {(id) => (
            <Input id={id} value={name} onChange={(e) => setName(e.target.value)} autoFocus />
          )}
        </Field>

        {!channel && (
          <Field label="Slug" hint="Lowercase kebab-case; chosen once and immutable.">
            {(id) => (
              <Input
                id={id}
                value={slug}
                onChange={(e) => setSlug(e.target.value)}
                placeholder="deep-sleep-stories"
              />
            )}
          </Field>
        )}

        <Field label="Description">
          {(id) => (
            <Textarea
              id={id}
              rows={2}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          )}
        </Field>

        <Field label="Tone" hint="Handed to the LLM with every script request.">
          {(id) => (
            <Input
              id={id}
              value={tone}
              onChange={(e) => setTone(e.target.value)}
              placeholder="calm, measured, nocturnal"
            />
          )}
        </Field>

        <div className="grid grid-cols-2 gap-3">
          <Field label="Voice">
            {(id) => <Input id={id} value={voice} onChange={(e) => setVoice(e.target.value)} />}
          </Field>
          <Field label="Language">
            {(id) => (
              <Input id={id} value={language} onChange={(e) => setLanguage(e.target.value)} />
            )}
          </Field>
        </div>

        <Field label="Image style">
          {(id) => (
            <Input
              id={id}
              value={imageStyle}
              onChange={(e) => setImageStyle(e.target.value)}
              placeholder="muted watercolour, wide shot"
            />
          )}
        </Field>

        <div className="grid grid-cols-2 gap-3">
          <Field label="Words per chapter">
            {(id) => (
              <Input
                id={id}
                type="number"
                min={50}
                max={5000}
                value={words}
                onChange={(e) => setWords(e.target.value)}
              />
            )}
          </Field>
          {channel && (
            <Field label="Credentials">
              {(id) => (
                <Select
                  id={id}
                  value={credentials}
                  onChange={(e) => setCredentials(e.target.value as Channel['credentials'])}
                >
                  <option value="missing">missing</option>
                  <option value="valid">valid</option>
                  <option value="expired">expired</option>
                </Select>
              )}
            </Field>
          )}
        </div>

        {save.isError && <ErrorNotice error={save.error} />}
      </div>
    </Modal>
  )
}
