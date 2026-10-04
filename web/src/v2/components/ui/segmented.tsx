import type { LucideIcon } from 'lucide-react'

import { cn } from '../../core/utils'
import { SlidingThumb } from './sliding-thumb'
import { Tooltip } from './tooltip'

export interface Segment<T extends string> {
  value: T
  label: string
  icon?: LucideIcon
  shortcut?: string
}

interface SegmentedProps<T extends string> {
  segments: readonly Segment<T>[]
  value: T
  onChange: (value: T) => void
  iconOnly?: boolean
  className?: string
}

/**
 * A segmented control: one recessed capsule, and the selection as a glass
 * thumb that glides between options.
 *
 * Arrow keys move the selection, as they do in a native radio group.
 */
export function Segmented<T extends string>({
  segments,
  value,
  onChange,
  iconOnly,
  className,
}: SegmentedProps<T>) {
  const step = (direction: 1 | -1) => {
    const index = segments.findIndex((segment) => segment.value === value)
    const next = segments[(index + direction + segments.length) % segments.length]
    if (next) onChange(next.value)
  }

  return (
    <div
      role="radiogroup"
      className={cn('relative inline-flex items-center rounded-full bg-[var(--well)] p-0.5', className)}
      onKeyDown={(event) => {
        if (event.key === 'ArrowRight' || event.key === 'ArrowDown') {
          event.preventDefault()
          step(1)
        } else if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') {
          event.preventDefault()
          step(-1)
        }
      }}
    >
      <SlidingThumb active={value} />
      {segments.map((segment) => {
        const selected = segment.value === value
        const Icon = segment.icon
        const button = (
          <button
            key={segment.value}
            type="button"
            role="radio"
            aria-checked={selected}
            aria-label={iconOnly ? segment.label : undefined}
            tabIndex={selected ? 0 : -1}
            data-thumb-key={segment.value}
            onClick={() => onChange(segment.value)}
            className={cn(
              'relative flex h-6 flex-1 items-center justify-center gap-1.5 rounded-full',
              'text-[12px] font-medium whitespace-nowrap transition-colors duration-150',
              iconOnly ? 'min-w-7 px-1.5' : 'px-2.5',
              selected ? 'text-primary' : 'text-tertiary hover:text-primary',
            )}
          >
            {Icon ? <Icon className="size-[15px] shrink-0" strokeWidth={1.8} /> : null}
            {iconOnly ? null : <span className="truncate">{segment.label}</span>}
          </button>
        )
        return iconOnly || segment.shortcut ? (
          <Tooltip key={segment.value} content={segment.label} shortcut={segment.shortcut}>
            {button}
          </Tooltip>
        ) : (
          button
        )
      })}
    </div>
  )
}
