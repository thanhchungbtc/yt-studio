import type { IDockviewPanelProps } from 'dockview-react'
import { Plus, Tv } from 'lucide-react'

import type { DocPanelParams } from './dock'
import { Placeholder } from './placeholder'
import { EditorShell } from './shell'

/**
 * Creating a channel.
 *
 * Still a document, unlike a new video, and the asymmetry is the point rather
 * than an oversight: a video is a question with an answer, and a channel is a
 * thing you come back to — its style, its voice, its credentials. When the form
 * exists it belongs where the channel editor is, not in a dialog you dismiss.
 */
export function NewEditor({ params }: IDockviewPanelProps<DocPanelParams>) {
  return (
    <EditorShell title={params.title} seed="new-channel" icon={Plus} status="Not created yet">
      <Placeholder
        icon={Tv}
        title="New channel"
        detail="Name, style and credentials — the form lands here."
      />
    </EditorShell>
  )
}
