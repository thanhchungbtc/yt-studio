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
      {/* No entrance animation: tab switches would replay it. */}
      <div className="min-h-0 flex-1">{children}</div>
    </div>
  )
}
