import { cn } from '../../core/utils'

export interface Segment<T extends string> {
  value: T
  label: string
}

interface SegmentedProps<T extends string> {
  segments: readonly Segment<T>[]
  value: T
  onChange: (value: T) => void
}

/**
 * A macOS segmented control: one recessed track, and the selected segment
 * raised out of it on a white cap. The cap is a real element rather than a
 * pseudo-selector so it can carry the shadow AppKit gives the knob.
 */
export function Segmented<T extends string>({ segments, value, onChange }: SegmentedProps<T>) {
  return (
    <div
      className="flex items-center gap-0.5 rounded-md p-0.5"
      style={{ backgroundColor: 'var(--idle-selection)' }}
    >
      {segments.map((segment) => {
        const selected = segment.value === value
        return (
          <button
            key={segment.value}
            type="button"
            aria-pressed={selected}
            onClick={() => onChange(segment.value)}
            className={cn(
              'flex h-[21px] flex-1 items-center justify-center rounded-[5px] px-2',
              'text-[12px] font-medium transition-colors',
              selected ? 'text-primary' : 'text-secondary hover:text-primary',
            )}
            style={
              selected
                ? {
                    backgroundColor: 'var(--raised)',
                    boxShadow: '0 1px 2px rgb(0 0 0 / 0.14)',
                  }
                : undefined
            }
          >
            {segment.label}
          </button>
        )
      })}
    </div>
  )
}
