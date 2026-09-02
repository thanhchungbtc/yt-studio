import { cn } from '../../core/utils'

/**
 * The name over a thing, in the app's own voice.
 *
 * Ten pixels, uppercase, tracked out, tertiary. It labels a field on the upload
 * page, a block in the thumbnail builder, and the FINAL strip's own row, and it
 * has to be identical in all three or the screens stop reading as one
 * application.
 *
 * Here rather than in any one of those folders because more than one uses it,
 * which is the whole rule: a part belonging to a single screen lives with that
 * screen, and a part two screens share lives where neither owns it.
 */
export function Caption({ children, className }: { children: string; className?: string }) {
  return (
    <span
      className={cn(
        'text-[10px] font-semibold tracking-[0.07em] text-tertiary uppercase',
        className,
      )}
    >
      {children}
    </span>
  )
}
