import type { IDockviewPanelProps } from 'dockview-react'
import { Clapperboard } from 'lucide-react'

import type { DocPanelParams } from './dock'
import { Placeholder } from './placeholder'
import { EditorShell } from './shell'

/** The video editor. A shell for now: title strip, and the body it will fill. */
export function VideoEditor({ params }: IDockviewPanelProps<DocPanelParams>) {
  const ref = params.doc?.kind === 'video' ? params.doc.ref : ''

  return (
    <EditorShell title={params.title} seed={params.seed} initial={params.initial} status={ref}>
      <Placeholder
        icon={Clapperboard}
        title="Video editor"
        detail="The script, the chapters and the timeline land here."
      />
    </EditorShell>
  )
}
