import { cn } from '../../core/utils'

/**
 * A paragraph folded to two lines, with the way to the rest of it underneath.
 *
 * Two screens show a brief they did not write and cannot shorten — a chapter's
 * summary in the pipeline table, a video's topic over it — and both had the
 * same problem from opposite ends. The table printed every word, which cost it
 * its shape; the summary band clamped without saying so, which is a sentence
 * that appears to end mid-word. One answer to both: fold it, and say that it is
 * folded.
 *
 * Here rather than in either folder because both use it, which is the rule the
 * `Caption` beside it states: a part belonging to one screen lives with that
 * screen, and a part two screens share lives where neither owns it.
 *
 * Two lines, not a length. Where a sentence runs out of room depends on the
 * width of the window, not on the number of characters in the string, so
 * `line-clamp` is the only measure that stays true when the pane is dragged.
 *
 * Open is the caller's state, not this component's. In the table one control in
 * the column head opens thirty of these at once and the blueprint gate opens
 * them all by itself; a component holding its own boolean could do neither.
 */
export function Clamped({
  text,
  open,
  onToggle,
  label,
  className,
}: {
  text: string
  open: boolean
  onToggle: () => void
  /** What this is a brief of, for anyone not reading pixels. */
  label: string
  /** The type of the prose, which is each caller's own. */
  className?: string
}) {
  return (
    <>
      {/* Text, and a shortcut to the button below it. Deliberately not a button
          itself: the brief is the content here, and a paragraph wearing
          `role="button"` hands a screen reader the word "brief" in place of the
          sentence it is announcing. */}
      <p onClick={onToggle} className={cn('cursor-pointer', !open && 'line-clamp-2', className)}>
        {text}
      </p>
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={open}
        aria-label={label}
        className="mt-0.5 text-xs text-fg-subtle transition-colors hover:text-fg-muted"
      >
        {open ? 'Less' : 'More'}
      </button>
    </>
  )
}
