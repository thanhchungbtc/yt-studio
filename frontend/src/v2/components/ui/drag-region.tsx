import type { ReactNode } from 'react'

import { cn } from '../../core/utils'

interface DragRegionProps {
  className?: string
  children?: ReactNode
}

/** A strip that moves the window like a titlebar (see `.drag-region` in styles.css). */
export function DragRegion({ className, children }: DragRegionProps) {
  return <div className={cn('drag-region select-none', className)}>{children}</div>
}
