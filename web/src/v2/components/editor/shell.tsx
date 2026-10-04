import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'

import { EditorTitleBar } from './title-bar'

interface EditorShellProps {
  title: string
  seed?: string
  initial?: string
  icon?: LucideIcon
  status?: ReactNode
  statusColor?: string
  /** What the strip carries on its trailing edge; see the title bar. */
  actions?: ReactNode
  children: ReactNode
}

/**
 * The frame every editor is built in: a title strip over the document, both on
 * the group's card. The card is the surface; the strip and the document are
 * two regions of it, not two layers.
 */
export function EditorShell({
  title,
  seed,
  initial,
  icon,
  status,
  statusColor,
  actions,
  children,
}: EditorShellProps) {
  return (
    <div className="flex h-full flex-col">
      <EditorTitleBar
        title={title}
        seed={seed}
        initial={initial}
        icon={icon}
        status={status}
        statusColor={statusColor}
        actions={actions}
      />
      {/* No entrance animation. A tab's content is hidden and shown again as
          tabs switch, which can restart a CSS animation on it — so a fade here
          played on every tab switch, and made switching feel like loading. A
          document comes forward the way a window does: at once. */}
      <div className="min-h-0 flex-1">{children}</div>
    </div>
  )
}
