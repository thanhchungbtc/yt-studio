import { DragRegion } from '../ui/drag-region'

/**
 * The bottom panel. Deliberately empty, and full width.
 *
 * Full width because it is about the session — the log, the queue, the run in
 * progress — not about whichever document happens to be open above it. Indented
 * to one column it would claim the opposite.
 *
 * The same material as the sidebars, not a shade of its own. Every pane that
 * frames the document is one surface interrupted by seams; giving this one its
 * own tone would make it a third kind of thing for no reason anyone could name.
 */
export function BottomPanel() {
  return (
    <div className="surface-chrome flex h-full flex-col">
      <DragRegion className="flex h-[28px] shrink-0 items-center px-3">
        <span className="min-w-0 flex-1 truncate text-[11px] font-semibold tracking-[0.05em] text-tertiary uppercase">
          Console
        </span>
        <span className="shrink-0 text-[11px] text-tertiary">⌘2</span>
      </DragRegion>
      <div className="min-h-0 flex-1" />
    </div>
  )
}
