import type { ReactNode } from 'react'

import { cn } from '../../core/utils'
import { Avatar } from '../ui/avatar'

interface RowProps {
  /** The document this row opens, as `video:REF` or `channel:slug`. */
  id: string
  title: string
  subtitle: ReactNode
  timestamp?: string
  avatarName: string
  avatarSeed?: string
  /** The state badge on the token; omitted when there is nothing to say. */
  dotColor?: string
  selected: boolean
  onSelect: () => void
  onOpen: () => void
}

/**
 * One row of the source list, laid out the way a Messages conversation is: a
 * token, two lines of text, and the time on the trailing edge of the first.
 *
 * The selected row is a filled, rounded pill inset from both edges rather than
 * a full-bleed band. That inset is most of why the list reads as macOS and not
 * as a table — and it is what the group headers deliberately break, spanning
 * the full width so the two never read as the same kind of thing.
 *
 * The state badge rides on the token rather than sitting in a column of its
 * own. A column of mostly-empty space is a column the eye still has to cross.
 */
export function Row({
  id,
  title,
  subtitle,
  timestamp,
  avatarName,
  avatarSeed,
  dotColor,
  selected,
  onSelect,
  onOpen,
}: RowProps) {
  return (
    <button
      type="button"
      onClick={onSelect}
      onDoubleClick={onOpen}
      aria-current={selected}
      data-row-id={id}
      className={cn(
        'flex w-full items-center gap-2.5 rounded-[9px] px-2 py-1.5 text-left',
        'transition-colors duration-75',
        selected ? 'text-white' : 'hover:bg-[var(--hover)]',
      )}
      style={selected ? { backgroundColor: 'var(--accent)' } : undefined}
    >
      <span className="relative shrink-0">
        <Avatar name={avatarName} seed={avatarSeed} />
        {dotColor ? (
          // The ring punches the badge out of the token underneath it, and is
          // the colour of whatever the row itself is sitting on — so the badge
          // reads the same on a plain row and on the blue pill.
          <span
            className="absolute -top-0.5 -left-0.5 size-[10px] rounded-full border-2"
            style={{
              backgroundColor: selected ? '#ffffff' : dotColor,
              borderColor: selected ? 'var(--accent)' : 'var(--window)',
            }}
          />
        ) : null}
      </span>

      <span className="min-w-0 flex-1">
        <span className="flex items-baseline gap-2">
          <span
            className={cn(
              'min-w-0 flex-1 truncate text-[13px] font-semibold',
              selected ? 'text-white' : 'text-primary',
            )}
          >
            {title}
          </span>
          {timestamp ? (
            <span
              className={cn(
                'shrink-0 text-[11px] tabular-nums',
                selected ? 'text-white/80' : 'text-tertiary',
              )}
            >
              {timestamp}
            </span>
          ) : null}
        </span>
        <span
          className={cn(
            'mt-px block truncate text-[12px]',
            selected ? 'text-white/85' : 'text-secondary',
          )}
        >
          {subtitle}
        </span>
      </span>
    </button>
  )
}
