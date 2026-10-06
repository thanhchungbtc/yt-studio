import { SquareTerminal } from 'lucide-react'

import { useLLMStream } from '../../core/llm'
import { PaneHeader } from '../ui/pane-header'
import { Console } from './console'

/**
 * The bottom panel. Full width, and about the session.
 *
 * Full width because it is about the session — the log, the queue, the run in
 * progress — not about whichever document happens to be open above it. That is
 * also why the console shows every exchange rather than the open video's: a
 * model working is a fact about the machine, and the pool it is holding is
 * shared by every video at once.
 *
 * The stream is opened here rather than at the root of the workbench, so it is
 * connected exactly while this panel is on screen. The server retains recent
 * exchanges, so showing the panel again replays what was missed — which makes a
 * hidden panel cost nothing rather than cost a connection nobody reads.
 */
export function BottomPanel() {
  useLLMStream()

  return (
    <div className="flex h-full flex-col">
      <PaneHeader
        title="Console"
        icon={SquareTerminal}
        command="workbench.togglePanel"
        side="top"
      />
      <div className="min-h-0 flex-1">
        <Console />
      </div>
    </div>
  )
}
