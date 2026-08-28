import { Inspector } from './inspector'
import { DragRegion } from '../ui/drag-region'

/**
 * The secondary sidebar: the shell the inspector lives in.
 *
 * Two strips above the content and nothing else. The first is the height of the
 * window's top edge, so this pane lines up with the sidebar that carries the
 * traffic lights; the second is the pane's own label. Both are drag regions —
 * the pane is chrome, and chrome is what the window is picked up by.
 *
 * The label stays `Inspector`, not the name of whatever is being inspected. It
 * says which pane this is and which key hides it; what is in front of you is the
 * content's job to say, and the tab strip has already said it.
 */
export function SecondarySidebar() {
  return (
    <div className="surface-chrome flex h-full flex-col">
      <DragRegion className="h-[38px] shrink-0" />
      <DragRegion className="flex h-[30px] shrink-0 items-center px-3">
        <span className="min-w-0 flex-1 truncate text-[11px] font-semibold tracking-[0.06em] text-tertiary uppercase">
          Inspector
        </span>
        <span className="shrink-0 text-[11px] text-tertiary">⌘3</span>
      </DragRegion>
      <Inspector />
    </div>
  )
}
