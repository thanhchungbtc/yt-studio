import { cn } from '../../core/utils'

export function Kbd({ keys, className }: { keys: string; className?: string }) {
  return (
    <kbd
      className={cn(
        'inline-flex h-[16px] items-center rounded-[4px] bg-[var(--well)] px-1 font-sans text-[10.5px] leading-none tracking-[0.04em] text-secondary',
        'shadow-[inset_0_-0.5px_0_var(--line-strong)]',
        className,
      )}
    >
      {keys}
    </kbd>
  )
}
