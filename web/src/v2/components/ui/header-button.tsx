import type { LucideIcon } from 'lucide-react'

import { cn } from '../../core/utils'
import { Tooltip } from './tooltip'

interface HeaderButtonProps {
  icon: LucideIcon
  label: string
  /** Shown beside the label in the tooltip. */
  shortcut?: string
  active?: boolean
  onClick?: () => void
  className?: string
  tooltipSide?: 'top' | 'bottom' | 'left' | 'right'
}

/**
 * The quiet capsule a toolbar carries: no chrome of its own until the pointer
 * is on it, and a tinted fill when it is holding something open.
 */
export function HeaderButton({
  icon: Icon,
  label,
  shortcut,
  active,
  onClick,
  className,
  tooltipSide = 'bottom',
}: HeaderButtonProps) {
  return (
    <Tooltip content={label} shortcut={shortcut} side={tooltipSide}>
      <button
        type="button"
        aria-label={label}
        aria-pressed={active}
        onClick={onClick}
        className={cn(
          'inline-flex size-7 shrink-0 items-center justify-center rounded-full transition-colors duration-100',
          'text-secondary hover:bg-hover hover:text-primary',
          active && 'bg-active text-primary',
          className,
        )}
      >
        <Icon className="size-4" strokeWidth={1.8} />
      </button>
    </Tooltip>
  )
}
