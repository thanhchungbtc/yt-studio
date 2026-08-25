import type { IDockviewPanelProps } from 'dockview-react'
import { Image } from 'lucide-react'

import type { DocPanelParams } from './dock'
import { Placeholder } from './placeholder'
import { EditorShell } from './shell'

/** The thumbnail editor. A shell for now. */
export function ThumbnailEditor({ params }: IDockviewPanelProps<DocPanelParams>) {
  const ref = params.doc?.kind === 'thumbnail' ? params.doc.ref : ''

  return (
    <EditorShell title={params.title} seed={params.seed} initial={params.initial} status={ref}>
      <Placeholder
        icon={Image}
        title="Thumbnail editor"
        detail="The grid, the icons and the caption are composed here."
      />
    </EditorShell>
  )
}
