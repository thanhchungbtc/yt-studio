import { Inspector } from './inspector'
import { PaneHeader } from '../ui/pane-header'

/**
 * The secondary sidebar: the card the inspector lives in.
 *
 * One strip above the content, the same height as every other card's, so the
 * three columns line up along the top of the window. The label stays
 * `Inspector`, not the name of whatever is being inspected: it says which pane
 * this is and which key hides it; what is in front of you is the content's job
 * to say, and the tab strip has already said it.
 */
export function SecondarySidebar() {
  return (
    <div className="flex h-full flex-col">
      <PaneHeader title="Inspector" shortcut="⌘3" />
      <Inspector />
    </div>
  )
}
