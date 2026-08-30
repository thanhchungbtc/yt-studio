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
 * The frame every editor is built in: a translucent title strip over one opaque
 * document surface.
 *
 * The nesting matters and is the whole visual argument of the window. The strip
 * sits *outside* the opaque surface, so the material shows through it; the
 * document does not, so the thing being worked on is the one solid object on
 * screen.
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
      <div className="surface-content min-h-0 flex-1">{children}</div>
    </div>
  )
}
