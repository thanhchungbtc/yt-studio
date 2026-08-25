import { DragRegion } from '../ui/drag-region'

/**
 * The secondary sidebar. Deliberately empty.
 *
 * It exists now so the shell it lives in — the drag region, the hairline, the
 * resize behaviour, the keystroke that shows it — is settled before anything
 * has to be built inside it. What goes here is the inspector for whatever the
 * editor has open, and that arrives in its own step.
 */
export function SecondarySidebar() {
  return (
    <div className="surface-chrome hairline-l flex h-full flex-col">
      <DragRegion className="h-[38px] shrink-0" />
      <DragRegion className="flex h-[30px] shrink-0 items-center px-3">
        <span className="min-w-0 flex-1 truncate text-[11px] font-semibold tracking-[0.06em] text-tertiary uppercase">
          Inspector
        </span>
        <span className="shrink-0 text-[11px] text-tertiary">⌘3</span>
      </DragRegion>
      <div className="min-h-0 flex-1" />
    </div>
  )
}
