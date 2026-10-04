import { useEffect, useRef } from 'react'

import { count } from '../../../../core/format'
import type { Chapter } from '../../../../core/types'
import { cn } from '../../../../core/utils'

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
