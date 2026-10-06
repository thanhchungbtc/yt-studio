import type { IDockviewPanelProps } from 'dockview-react'
import { Tv } from 'lucide-react'

import type { DocPanelParams } from './dock'
import { Placeholder } from './placeholder'
import { EditorShell } from './shell'

/** The channel editor. A shell for now. */
export function ChannelEditor({ params }: IDockviewPanelProps<DocPanelParams>) {
  const slug = params.doc?.kind === 'channel' ? params.doc.slug : ''

  return (
    <EditorShell title={params.title} seed={params.seed} initial={params.initial} status={slug}>
      <Placeholder
        icon={Tv}
        title="Channel editor"
        detail="Identity, style and credentials for this channel."
      />
    </EditorShell>
  )
}
