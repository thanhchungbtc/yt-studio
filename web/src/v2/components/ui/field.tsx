import { ChevronsUpDown } from 'lucide-react'
import {
  useId,
  type CSSProperties,
  type InputHTMLAttributes,
  type ReactNode,
  type SelectHTMLAttributes,
  type TextareaHTMLAttributes,
} from 'react'

import { cn } from '../../core/utils'

/**
 * A form row, laid out the way macOS lays one out: the label in a fixed column
 * on the left, right-aligned against the control it names.
 *
 * That alignment is not decoration. A column of right-aligned labels puts every
 * label a fixed distance from the thing it belongs to, so the eye reads pairs
 * instead of scanning for which caption goes with which box — which is exactly
 * what labels stacked above controls make you do once there are more than three.
 */
const LABEL = 'w-[92px] shrink-0 text-right text-[12px] text-secondary'
/** Everything below a control lines up with the control, not with the label. */
export const INDENT = 'pl-[104px]'

export function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: ReactNode
  children: (id: string) => ReactNode
}) {
  const id = useId()
  return (
    <div className="flex gap-3 py-[5px]">
      <label htmlFor={id} className={cn(LABEL, 'pt-[4px]')}>
        {label}
      </label>
      <div className="min-w-0 flex-1">
        {children(id)}
        {hint ? <p className="mt-1 text-[11px] leading-snug text-tertiary">{hint}</p> : null}
      </div>
    </div>
  )
}

/**
 * A number you can sweep or type: a track, the figure, and its unit.
 *
 * Both controls, because the two ways of answering are different questions.
 * "About three hours" is a drag — you are choosing a size, and a slider is the
 * only control that lets you feel where the ends are — and "exactly 137" is
 * typed. A form that offered one would make the other a chore, and the range
 * input is what carries the keyboard: arrows step, Home and End go to the
 * bounds, and none of that has to be written here.
 *
 * This was a lone number box, which is why it is worth saying what the track
 * bought: a `1–500` printed beside an empty field is a range you have to
 * imagine, where a knob a fifth of the way along is one you can see.
 *
 * The value is a string because a half-typed field holds `""`, and a `0` there
 * would be the form answering for you. The track needs a number, so an unset
 * field parks its knob at the floor.
 */
export function RangeField({
  label,
  unit,
  value,
  onChange,
  min,
  max,
  step = 1,
  /** Under the row, in the control column: what this shape comes to. */
  children,
}: {
  label: string
  unit: ReactNode
  value: string
  onChange: (value: string) => void
  min: number
  max: number
  step?: number
  children?: ReactNode
}) {
  const id = useId()
  const current = Number(value)
  const position = Number.isFinite(current) ? Math.min(Math.max(current, min), max) : min
  // Clamped before it is a percentage, so a typed 9999 fills the track rather
  // than overrunning the gradient.
  const fill = max > min ? ((position - min) / (max - min)) * 100 : 0

  return (
    <div className="py-[5px]">
      <div className="flex items-center gap-3">
        <label htmlFor={id} className={LABEL}>
          {label}
        </label>
        <input
          type="range"
          aria-label={label}
          min={min}
          max={max}
          step={step}
          value={position}
          onChange={(event) => onChange(event.target.value)}
          className="slider min-w-0 flex-1"
          style={{ '--slider-fill': `${fill}%` } as CSSProperties}
        />
        <input
          id={id}
          type="number"
          min={min}
          max={max}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          className="control w-[58px] shrink-0 text-right tabular-nums"
        />
        <span className="w-[92px] shrink-0 truncate text-[11px] text-tertiary">{unit}</span>
      </div>
      {children ? <div className={cn(INDENT, 'pt-1.5')}>{children}</div> : null}
    </div>
  )
}

type InputProps = InputHTMLAttributes<HTMLInputElement>

export function Input({ className, ...props }: InputProps) {
  return <input {...props} className={cn('control w-full', className)} />
}

type TextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement>

export function Textarea({ className, ...props }: TextareaProps) {
  return <textarea {...props} className={cn('control w-full resize-none', className)} />
}

type SelectProps = SelectHTMLAttributes<HTMLSelectElement>

/**
 * A macOS pop-up button: a well with the accent cap and its double chevron on
 * the trailing edge. The chevron is the affordance — a select drawn as a plain
 * text field is a control nobody knows they can click.
 */
export function Select({ className, children, ...props }: SelectProps) {
  return (
    <div className="relative">
      <select {...props} className={cn('control w-full appearance-none pr-8', className)}>
        {children}
      </select>
      <span
        aria-hidden
        className="pointer-events-none absolute top-1/2 right-[3px] flex size-[17px] -translate-y-1/2 items-center justify-center rounded-[4px]"
        style={{ backgroundColor: 'var(--accent)' }}
      >
        <ChevronsUpDown className="size-3 text-white" strokeWidth={2.5} />
      </span>
    </div>
  )
}

/** A checkbox and its label, which on macOS is always to the right of the box. */
export function Checkbox({
  checked,
  onChange,
  children,
}: {
  checked: boolean
  onChange: (checked: boolean) => void
  children: ReactNode
}) {
  return (
    <label className="flex cursor-pointer items-center gap-2 text-[12px] text-primary">
      <input
        type="checkbox"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
        className="size-[13px] accent-[var(--accent)]"
      />
      {children}
    </label>
  )
}

/**
 * The rule between two groups of fields; the only structure the dialog needs.
 *
 * `seam-h` rather than `hairline-t`: the hairline utilities are *inset*
 * shadows, which need a box to be inset into, and this element has no height of
 * its own. A filled rule is the shape that draws.
 */
export function FieldDivider() {
  return <div className="seam-h my-3.5" />
}
