import { cn } from '@/lib/utils'

/**
 * A boolean as a switch rather than a two-option dropdown: the state is legible
 * without reading, and flipping it is one click instead of three.
 *
 * Hand-rolled on a `role="switch"` button rather than pulled from Radix — this
 * is the whole of the behaviour, and a dependency for it would be larger than
 * the component.
 */
export function Switch({
  checked,
  onCheckedChange,
  disabled,
  id,
  'aria-label': ariaLabel,
  'aria-describedby': ariaDescribedBy,
}: {
  checked: boolean
  onCheckedChange: (checked: boolean) => void
  disabled?: boolean
  id?: string
  'aria-label'?: string
  'aria-describedby'?: string
}) {
  return (
    <button
      type="button"
      role="switch"
      id={id}
      aria-checked={checked}
      aria-label={ariaLabel}
      aria-describedby={ariaDescribedBy}
      disabled={disabled}
      onClick={() => onCheckedChange(!checked)}
      className={cn(
        'relative inline-flex h-[20px] w-[34px] shrink-0 items-center rounded-full border no-select',
        'transition-colors duration-150 disabled:pointer-events-none disabled:opacity-45',
        checked
          ? 'border-transparent bg-[hsl(var(--accent))]'
          : 'border-[hsl(var(--border-strong))] bg-[hsl(var(--bg-hover))]',
      )}
    >
      <span
        aria-hidden
        className={cn(
          'block h-[14px] w-[14px] rounded-full transition-transform duration-150 ease-out elev-1',
          checked
            ? 'translate-x-[17px] bg-[hsl(var(--accent-fg))]'
            : 'translate-x-[3px] bg-[hsl(var(--bg-elevated))]',
        )}
      />
    </button>
  )
}
