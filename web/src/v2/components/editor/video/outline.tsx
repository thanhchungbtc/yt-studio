import type { Chapter } from '../../../core/types'
import { cn } from '../../../core/utils'

/**
 * The table of contents, floating in the margin.
 *
 * A card rather than a column, and the difference is the whole design. A column
 * divides the view and claims the full height whether or not it has anything to
 * put there; seven chapters is a short list, and a short list stretched down a
 * window is mostly empty rule. This is sized to what it holds and sits in space
 * the centred reading column was already leaving blank.
 *
 * It is outside the scroller, so it does not move at all while the content runs
 * past it — no sticking, no catching up. And it clears the pinned chapter band
 * above it rather than overlapping: the band is translucent, a card over it goes
 * muddy, and the two are answering different questions anyway. The band is the
 * chapter you are *in*; this is the map of all of them.
 *
 * The reader makes room for it rather than the card being squeezed into whatever
 * margin happens to be left over. Centring a 40rem column in a 900px reader
 * leaves 130px a side, and a 188px card dropped into that sits on the words — so
 * the scroller is padded by the card's footprint at the same width the card
 * appears at, and the column then centres inside what remains. The card still
 * floats; it is the text that steps aside.
 */
export function ChapterOutline({
  chapters,
  activeId,
  onJump,
}: {
  chapters: Chapter[]
  /** Whichever chapter is currently in view; the bar follows it. */
  activeId: string | null
  onJump: (id: string) => void
}) {
  return (
    <nav
      // Below the pinned band, and hidden until there is a margin to sit in.
      className={cn(
        'absolute top-[38px] left-3 z-10 hidden w-[188px] overflow-y-auto rounded-[8px] py-1',
        'max-h-[60%] @[54rem]:block',
      )}
      // Raised rather than banded. `--band` is four percent black — fine over the
      // chrome it was made for, and over a document it is a pane of glass with
      // the script legible straight through it.
      style={{
        backgroundColor: 'var(--raised)',
        boxShadow: '0 0 0 0.5px var(--separator-strong), 0 4px 14px -6px rgb(0 0 0 / 0.22)',
      }}
    >
      {chapters.map((chapter) => {
        const active = chapter.id === activeId
        return (
          <button
            key={chapter.id}
            type="button"
            onClick={() => onJump(chapter.id)}
            className={cn(
              'relative flex w-full items-baseline gap-2 py-[3px] pr-2.5 pl-3 text-left',
              'text-[12px] transition-colors',
              active ? 'text-primary' : 'text-secondary hover:text-primary',
            )}
          >
            {/* Two pixels on the leading edge. A filled pill would say *you
                chose this*; the outline is reporting where you are. */}
            {active ? (
              <span
                className="absolute top-[3px] bottom-[3px] left-0 w-[2px] rounded-r-full"
                style={{ backgroundColor: 'var(--accent)' }}
              />
            ) : null}
            <span className="shrink-0 tabular-nums text-tertiary">{chapter.ordinal}</span>
            <span className="min-w-0 flex-1 truncate">{chapter.title}</span>
          </button>
        )
      })}
    </nav>
  )
}
