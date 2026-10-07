import { useQuery } from '@tanstack/react-query'
import type { IDockviewPanelProps } from 'dockview-react'
import { Tv } from 'lucide-react'

import { api, qk } from '../../core/api'
import type { DocPanelParams } from './dock'
import { Placeholder } from './placeholder'
import { EditorShell } from './shell'

const CONNECTION = {
  valid: { label: 'Connected to YouTube', color: 'var(--done)' },
  expired: { label: 'YouTube sign-in expired', color: 'var(--warning)' },
  missing: { label: 'Not connected to YouTube', color: 'var(--warning)' },
} as const

/** The channel editor. A shell for now. */
export function ChannelEditor({ params }: IDockviewPanelProps<DocPanelParams>) {
  const slug = params.doc?.kind === 'channel' ? params.doc.slug : ''
  const channels = useQuery({ queryKey: qk.channels, queryFn: api.listChannels })
  const channel = channels.data?.find((c) => c.slug === slug)
  const connection = channel ? CONNECTION[channel.credentials] : undefined

  return (
    <EditorShell
      title={params.title}
      seed={params.seed}
      initial={params.initial}
      status={connection ? `${slug} · ${connection.label}` : slug}
      statusColor={connection?.color}
    >
      <Placeholder
        icon={Tv}
        title="Channel editor"
        detail="Identity, style and credentials for this channel."
      />
    </EditorShell>
  )
}
