import { cn } from '../../../../core/utils'

/**
 * The name over a block, in the app's own caption.
 *
 * The same 10px uppercase the reader puts over a script and the strip puts over
 * FINAL. This page is two mocks of somebody else's interface stacked on each
 * other, so the labels that say which is which have to be unmistakably *this*
 * application talking, not part of the thing being mocked.
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
