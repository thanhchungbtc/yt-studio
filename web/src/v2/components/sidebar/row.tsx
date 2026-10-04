import type { MouseEvent, ReactNode } from 'react'

import { cn } from '../../core/utils'
import { Avatar } from '../ui/avatar'
import { ContextMenu } from '../ui/context-menu'
import type { MenuItem } from '../ui/menu'

interface RowProps {
  /** The document this row opens, as `video:REF` or `channel:slug`. */
  id: string
  title: string
  subtitle: ReactNode
  timestamp?: string
  avatarName: string
  avatarSeed?: string
  /**
   * The state, as one value that drives both marks.
   *
   * The dot and the ring around the token read the same property, so they can
   * never end up saying different things.
   */
  tone?: 'accent' | 'running' | 'failed'
  /**
   * How the mark moves, which is half of what it says.
   *
   * Named for the state rather than the animation, because the two marks move
   * differently: working sets the ring orbiting and the dot swelling once,
   * attention sets both beating twice. One value, two rhythms, and neither
   * component has to know what the other is drawing.
   *
   * The rhythm is not decoration. At ten pixels hue alone cannot separate
   * "working" from "waiting for you", and with red/green colour blindness it
   * does not separate them at all. Omitted for a state that is simply true
   * rather than happening.
   */
  motion?: 'working' | 'attention'
  /**
   * Over, and needing nothing — so the row steps back.
   *
   * Without this, "finished" is expressed only as the absence of a mark, which
   * makes it indistinguishable from "has not started". Absence is not a signal.
   *
   * The point is not the row, it is the list: a mature library is mostly done,
   * so most of it goes quiet and the eye lands on the few rows that are not.
   * Selection overrides it, because the thing you have just clicked is never
   * the quiet one.
   */
  finished?: boolean
  selected: boolean
  /** The event comes through so the caller can read ⌘ and ⇧ off the click. */
  onSelect: (event: MouseEvent<HTMLButtonElement>) => void
  onOpen: () => void
  /**
   * Fired before the context menu opens, so a right-click on a row outside the
   * selection can collapse the selection onto it first — which is what stops a
   * menu built for three rows from acting on the one you pointed at.
   */
  onContextMenu?: () => void
  /**
   * What the right button offers on this row. Omitted where there is nothing to
   * offer, so a row without a menu does not swallow the gesture and show an
   * empty card.
   */
  menu?: MenuItem[]
  /**
   * The pointer arriving on the row and leaving it again, so the caller can
   * fetch what a click would open while the hand is still on its way.
   */
  onHover?: (hovering: boolean) => void
}

/**
 * One row of the source list, laid out the way a Messages conversation is: a
 * token, two lines of text, and the time on the trailing edge of the first.
 *
 * The selected row is a soft glass capsule inset from both edges rather than a
 * full-bleed band, concentric with the card it sits in. That inset is most of
 * why the list reads as macOS and not as a table.
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
  tone,
  motion,
  finished,
  selected,
  onSelect,
  onOpen,
  onContextMenu,
  menu,
  onHover,
}: RowProps) {
  const quiet = finished && !selected
  const row = (
    <button
      type="button"
      // Selects on the press, the way a Finder or Mail list does, rather than
      // on the release a click waits for — the hand is still on its way down
      // and the row is already the selected one and its document is opening.
      // ⌃-click is the context menu on a Mac, and the right button never
      // selects here.
      onPointerDown={(event) => {
        if (event.button !== 0 || event.ctrlKey) return
        onSelect(event)
      }}
      // Return and Space still press the button; they arrive as a click with
      // no pointer behind it, which is the only click left to act on.
      onClick={(event) => {
        if (event.detail === 0) onSelect(event)
      }}
      onDoubleClick={onOpen}
      onContextMenu={onContextMenu}
      onPointerEnter={onHover ? () => onHover(true) : undefined}
      onPointerLeave={onHover ? () => onHover(false) : undefined}
      aria-current={selected}
      data-row-id={id}
      className={cn(
        // `items-start`, not centred: the token tops with the title, so a row
        // whose second line wraps grows downwards instead of pushing the token
        // out of line with the name it belongs to.
        // No colour transition: a selection that fades in reads as one that
        // is still deciding. Lists on a Mac light up on the press, at once.
        'row-item flex w-full items-start gap-[9px] rounded-[var(--radius-inner)] px-[9px] py-[7px] text-left',
        selected && 'row-selected',
      )}
    >
      {/* The token is the loudest thing in the row — a saturated disc — so
          dimming it is most of the effect for one property. */}
      <span
        className={cn('row-avatar', quiet && 'opacity-[0.42]')}
        data-tone={tone}
        data-motion={motion}
      >
        <Avatar name={avatarName} seed={avatarSeed} className="size-7" />
        {/* No motion means the state is true rather than happening, so the dot
            takes a halo instead of a rhythm. */}
        {tone ? (
          <span className="row-dot" data-motion={motion} data-rest={motion ? undefined : ''} />
        ) : null}
      </span>

      <span className="min-w-0 flex-1">
        <span className="flex items-baseline gap-2">
          <span
            className={cn(
              'min-w-0 flex-1 truncate text-[13px] font-medium',
              quiet ? 'text-tertiary' : 'text-primary',
            )}
          >
            {title}
          </span>
          {timestamp ? (
            <span
              className={cn(
                'shrink-0 text-[10.5px] tabular-nums',
                selected ? 'text-secondary' : 'text-tertiary',
              )}
            >
              {timestamp}
            </span>
          ) : null}
        </span>
        {/* Two lines, then ellipsis. One line meant the end of the subtitle —
            which is where the useful half of it lives — was the part that got
            cut on a narrow sidebar. */}
        <span
          className={cn(
            'mt-px line-clamp-2 block text-[12px] leading-[1.35]',
            quiet ? 'text-tertiary' : 'text-secondary',
          )}
        >
          {subtitle}
        </span>
      </span>
    </button>
  )

  if (!menu?.length) return row
  return <ContextMenu items={menu}>{row}</ContextMenu>
}
