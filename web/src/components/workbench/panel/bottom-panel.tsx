import { X } from 'lucide-react'

import { ConsoleView } from './console-view'
import { OutputView } from './output-view'
import { useWorkbenchStore, type BottomView } from '../lib/store'
import { IconButton } from '../ui/controls'
import { Kbd, Tooltip } from '../ui/primitives'
import { cn } from '@/core/utils'

const TABS: { id: BottomView; label: string }[] = [
  { id: 'console', label: 'Console' },
  { id: 'output', label: 'Output' },
]

/**
 * The bottom panel. Views about the machine rather than about a document —
 * capacity, the queue, the raw event stream — so they sit under the editor
 * instead of taking its place, and both can be watched at once.
 */
export function BottomPanel() {
  const view = useWorkbenchStore((s) => s.bottomView)
  const showBottom = useWorkbenchStore((s) => s.showBottom)
  const toggleBottom = useWorkbenchStore((s) => s.toggleBottom)

  return (
    <div className="flex h-full min-h-0 flex-col border-t border-[hsl(var(--border))] bg-panel">
      <div className="flex h-8 shrink-0 items-center gap-3 border-b border-[hsl(var(--border))] px-2.5 no-select">
        {TABS.map((tab) => (
          <button
            key={tab.id}
            type="button"
            onClick={() => showBottom(tab.id)}
            aria-selected={view === tab.id}
            role="tab"
            className={cn(
              'relative h-full text-[11px] font-medium uppercase tracking-[0.06em] transition-colors',
              view === tab.id ? 'text-fg' : 'text-subtle hover:text-muted',
            )}
          >
            {tab.label}
            {view === tab.id && (
              <span
                aria-hidden
                className="absolute inset-x-0 bottom-0 h-[2px] bg-[hsl(var(--accent))]"
              />
            )}
          </button>
        ))}

        <div className="ml-auto flex items-center gap-1">
          <Kbd keys="$mod+Digit2" className="opacity-60" />
          <Tooltip label="Hide the panel" keys="$mod+Digit2" side="top">
            <IconButton aria-label="Hide the panel" onClick={toggleBottom}>
              <X className="h-3.5 w-3.5" />
            </IconButton>
          </Tooltip>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-hidden">
        {view === 'console' ? <ConsoleView /> : <OutputView />}
      </div>
    </div>
  )
}
