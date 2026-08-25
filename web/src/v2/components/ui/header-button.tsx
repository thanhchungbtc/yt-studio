import type { LucideIcon } from 'lucide-react'

import { cn } from '../../core/utils'

interface HeaderButtonProps {
  icon: LucideIcon
  label: string
  active?: boolean
  onClick?: () => void
  className?: string
}

/**
 * The quiet, borderless button macOS puts in a titlebar or a toolbar: no chrome
 * of its own until the pointer is on it, and a tinted fill when it is holding
 * something open.
 */
export function HeaderButton({ icon: Icon, label, active, onClick, className }: HeaderButtonProps) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      aria-pressed={active}
      onClick={onClick}
      className={cn(
        'flex size-[22px] shrink-0 items-center justify-center rounded-md transition-colors',
        'text-secondary hover:bg-[var(--hover)] hover:text-primary',
        className,
      )}
      style={
        active ? { backgroundColor: 'var(--idle-selection)', color: 'var(--text)' } : undefined
      }
    >
      <Icon className="size-[15px]" strokeWidth={1.75} />
    </button>
  )
}
