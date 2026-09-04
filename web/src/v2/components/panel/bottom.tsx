import { useLLMStream } from '../../core/llm'
import { DragRegion } from '../ui/drag-region'
import { Console } from './console'

/**
 * The bottom panel. Full width, and about the session.
 *
 * Full width because it is about the session — the log, the queue, the run in
 * progress — not about whichever document happens to be open above it. Indented
 * to one column it would claim the opposite. That is also why the console shows
 * every exchange rather than the open video's: a model working is a fact about
 * the machine, and the pool it is holding is shared by every video at once.
 *
 * The same material as the sidebars, not a shade of its own. Every pane that
 * frames the document is one surface interrupted by seams; giving this one its
 * own tone would make it a third kind of thing for no reason anyone could name.
 *
 * The stream is opened here rather than at the root of the workbench, so it is
 * connected exactly while this panel is on screen. The server retains recent
 * exchanges, so showing the panel again replays what was missed — which makes a
 * hidden panel cost nothing rather than cost a connection nobody reads.
 */
export function BottomPanel() {
  useLLMStream()

  return (
    <div className="surface-chrome flex h-full flex-col">
      <DragRegion className="flex h-[28px] shrink-0 items-center px-3">
        <span className="min-w-0 flex-1 truncate text-[11px] font-semibold tracking-[0.05em] text-tertiary uppercase">
          Console
        </span>
        <span className="shrink-0 text-[11px] text-tertiary">⌘2</span>
      </DragRegion>
      <div className="min-h-0 flex-1">
        <Console />
      </div>
    </div>
  )
}
