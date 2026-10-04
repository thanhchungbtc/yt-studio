import { useEffect, useRef } from 'react'

import { count } from '../../../../core/format'
import type { Chapter } from '../../../../core/types'
import { cn } from '../../../../core/utils'

/**
 * The table of contents, as the reader's leading column.
 *
 * It was a card floating in the margin once, sized to what it held. That suited
 * seven chapters and nothing past them: a fifty-chapter video turned the card
 * into a short window on a long list, sitting over a reader that had been
 * narrowed to make room for it — so the margin it lived in was the space the
 * text gave up, and the list still scrolled.
 *
 * A column claims the full height, which for a long video is exactly what the
 * list wants, and it gives the reader everything to its right without anything
 * overlapping anything. It is hidden on a window too narrow for both, where the
 * pinned chapter band alone says where you are.
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
  const list = useRef<HTMLDivElement>(null)

  // The bar follows the reader, and the list follows the bar: on a long video
  // the active row would otherwise scroll out of the column it is reporting in.
  // `nearest`, so a row already in view does not move at all.
  useEffect(() => {
    if (!activeId) return
    list.current
      ?.querySelector<HTMLElement>(`[data-outline="${activeId}"]`)
      ?.scrollIntoView({ block: 'nearest' })
  }, [activeId])

  return (
    <nav
      aria-label="Chapters"
      className="hairline-r hidden w-[220px] shrink-0 flex-col @[54rem]:flex"
    >
      <div className="flex shrink-0 items-baseline gap-2 px-4 pt-2.5 pb-1.5">
        <span className="text-[10px] font-semibold tracking-[0.07em] text-tertiary uppercase">
          Chapters
        </span>
        <span className="ml-auto text-[11px] tabular-nums text-tertiary">
          {count(chapters.length)}
        </span>
      </div>
      <div ref={list} className="min-h-0 flex-1 overflow-y-auto px-2 pb-2">
        {chapters.map((chapter) => {
          const active = chapter.id === activeId
          return (
            <button
              key={chapter.id}
              type="button"
              data-outline={chapter.id}
              aria-current={active ? 'location' : undefined}
              onClick={() => onJump(chapter.id)}
              className={cn(
                'relative flex w-full items-baseline gap-2 rounded-[6px] py-[4px] pr-2 pl-2.5 text-left',
                'text-[12px] transition-colors',
                active
                  ? 'bg-[var(--hover)] text-primary'
                  : 'text-secondary hover:bg-[var(--hover)] hover:text-primary',
              )}
              title={chapter.title}
            >
              {/* Two pixels on the leading edge. A filled pill alone would say
               *you chose this*; the bar is reporting where you are. */}
              {active ? (
                <span
                  className="absolute top-[5px] bottom-[5px] left-0 w-[2px] rounded-r-full"
                  style={{ backgroundColor: 'var(--accent)' }}
                />
              ) : null}
              <span className="w-[1.5rem] shrink-0 text-right tabular-nums text-tertiary">
                {chapter.ordinal}
              </span>
              <span className="min-w-0 flex-1 truncate">{chapter.title || 'Untitled'}</span>
            </button>
          )
        })}
      </div>
    </nav>
  )
}
