import { PRESETS, groupMeta } from './meta'
import { Skeleton } from '../../ui/primitives'
import { cn } from '@/core/utils'

export interface RailSection {
  name: string
  /** Rows in the section, before the filter. */
  total: number
  /** Rows the filter keeps; equal to `total` when nothing is being filtered. */
  hits: number
}

/**
 * Navigation inside one document, which is why it lives here and not in the
 * primary sidebar.
 *
 * While a filter is active this stops being a switch and becomes a map: every
 * section is still listed, each says how much of the match is behind it, and a
 * section with nothing dims rather than disappearing — a rail that reflows
 * under the cursor as characters are typed is a rail nobody can aim at.
 */
export function SectionRail({
  sections,
  active,
  filtering,
  loading,
  onSelect,
}: {
  sections: RailSection[]
  active: string
  filtering: boolean
  loading: boolean
  onSelect: (name: string) => void
}) {
  return (
    <nav
      aria-label="Settings sections"
      className="flex w-[212px] shrink-0 flex-col overflow-y-auto border-r border-[hsl(var(--border))] bg-subtle py-2 no-select"
    >
      {loading && (
        <div className="space-y-1.5 px-2.5 pt-1">
          {Array.from({ length: 8 }, (_, i) => (
            <Skeleton key={i} className="h-7 w-full" />
          ))}
        </div>
      )}

      {sections.map((section, index) => {
        const meta = groupMeta(section.name)
        const Icon = meta.icon
        const selected = section.name === active
        const empty = filtering && section.hits === 0
        return (
          <div key={section.name}>
            {/* The presets section owns no rows; a hairline says so. */}
            {index === 1 && sections[0]?.name === PRESETS && (
              <div className="mx-3 my-1.5 h-px bg-[hsl(var(--border))]" aria-hidden />
            )}
            <button
              type="button"
              onClick={() => onSelect(section.name)}
              aria-current={selected ? 'page' : undefined}
              className={cn(
                'relative mx-1.5 flex w-[calc(100%-12px)] items-center gap-2 rounded-[var(--radius-sm)] py-[5px] pl-3 pr-2 text-left transition-colors',
                selected
                  ? 'bg-[hsl(var(--bg-active))] text-fg'
                  : 'text-muted hover:bg-[hsl(var(--bg-hover))] hover:text-fg',
                empty && !selected && 'opacity-45',
              )}
            >
              {selected && (
                <span
                  aria-hidden
                  className="absolute left-0 top-1/2 h-4 w-[2px] -translate-y-1/2 rounded-r-full bg-[hsl(var(--accent))]"
                />
              )}
              <Icon
                className={cn(
                  'h-3.5 w-3.5 shrink-0',
                  selected ? 'text-[hsl(var(--accent))]' : 'text-subtle',
                )}
                aria-hidden
              />
              <span
                className={cn('min-w-0 flex-1 truncate text-[12px]', selected && 'font-medium')}
              >
                {meta.title}
              </span>
              <span
                className={cn(
                  'tabular shrink-0 text-[10.5px]',
                  filtering && section.hits > 0
                    ? 'font-medium text-[hsl(var(--accent))]'
                    : 'text-subtle',
                )}
              >
                {filtering ? section.hits : section.total}
              </span>
            </button>
          </div>
        )
      })}
    </nav>
  )
}
