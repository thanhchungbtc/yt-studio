import type { IDockviewPanelProps } from 'dockview-react'
import { Plus, Tv, Clapperboard } from 'lucide-react'

import type { DocPanelParams } from './dock'
import { Placeholder } from './placeholder'
import { EditorShell } from './shell'

/**
 * Creating a video or a channel.
 *
 * A document rather than a modal, because creating one is the beginning of
 * working on it — and because a sheet would be the only thing in this window
 * you cannot leave open while you look at something else.
 */
export function NewEditor({ params }: IDockviewPanelProps<DocPanelParams>) {
  const of = params.doc?.kind === 'new' ? params.doc.of : 'video'
  const video = of === 'video'

  return (
    <EditorShell title={params.title} seed={`new-${of}`} icon={Plus} status="Not created yet">
      <Placeholder
        icon={video ? Clapperboard : Tv}
        title={video ? 'New video' : 'New channel'}
        detail={
          video
            ? 'Topic, channel and shape — the form lands here.'
            : 'Name, style and credentials — the form lands here.'
        }
      />
    </EditorShell>
  )
}
