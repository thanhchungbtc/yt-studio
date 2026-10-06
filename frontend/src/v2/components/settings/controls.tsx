import { ChevronDown, Minus, Plus, RotateCcw } from 'lucide-react'
import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'

import { cn } from '@/kit/lib/cn'
import { isComposing } from '@/kit/lib/keys'
import { DropdownMenu } from '@/kit/ui/Menu'
import { confirm } from '@/kit/workbench/confirm'

import { matches, parseQuery, sectionInfo, type Query } from './schema'

export const QueryContext = createContext<Query>(parseQuery(''))
const SectionContext = createContext('')

export function Highlight({ text }: { text: string }) {
  const { words } = useContext(QueryContext)
  const parts = useMemo(() => {
    if (!words.length) return null
    const re = new RegExp(
      `(${words.map((w) => w.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|')})`,
      'gi',
    )
    return text.split(re)
  }, [text, words])
  if (!parts) return <>{text}</>
  return (
    <>
      {parts.map((p, i) =>
        i % 2 ? (
          <mark key={i} className="rounded-[3px] bg-[var(--accent-soft)] text-fg">
            {p}
          </mark>
        ) : (
          p
        ),
      )}
    </>
  )
}

export function Row({
  label,
  description,
  keywords,
  children,
  modified = false,
  onReset,
  dim,
}: {
  label: string
  description?: ReactNode
  keywords?: string
  children: ReactNode
  modified?: boolean
  onReset?: () => void
  /** Applies only when another backend is selected. */
  dim?: boolean
}) {
  const query = useContext(QueryContext)
  const section = useContext(SectionContext)
  if (query.modified && !modified) return null
  const text = typeof description === 'string' ? description : undefined
  if (!matches(query, label, text, keywords, section)) return null
  return (
    <div
      data-setting-row
      className={cn(
        'group/row flex min-h-12 items-center gap-6 border-b border-line py-2.5 last:border-b-0 @max-[30rem]/content:flex-col @max-[30rem]/content:items-start @max-[30rem]/content:gap-2',
        dim && 'opacity-55',
      )}
    >
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5 text-sm font-medium">
          <Highlight text={label} />
          {modified && (
            <span
              className="size-1.5 shrink-0 rounded-full bg-accent"
              title="Changed from the default"
              aria-label="Changed from the default"
            />
          )}
        </div>
        {description && (
          <div className="mt-0.5 text-xs leading-relaxed text-pretty text-fg-subtle">
            {text ? <Highlight text={text} /> : description}
          </div>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-2">
        {children}
        {onReset ? (
          <button
            title="Reset to default"
            aria-label={`Reset ${label} to default`}
            disabled={!modified}
            onClick={onReset}
            className="flex size-6 items-center justify-center rounded-full text-fg-faint opacity-0 group-hover/row:opacity-100 hover:bg-hover hover:text-fg focus-visible:opacity-100 disabled:invisible"
          >
            <RotateCcw className="size-3.5" />
          </button>
        ) : (
          <span className="size-6 @max-[30rem]/content:hidden" />
        )}
      </div>
    </div>
  )
}

export function Block({ keywords, children }: { keywords: string; children: ReactNode }) {
  const query = useContext(QueryContext)
  const section = useContext(SectionContext)
  if (query.modified || !matches(query, keywords, section)) return null
  return <div data-setting-row>{children}</div>
}

export function Group({
  title,
  description,
  children,
}: {
  title?: string
  description?: ReactNode
  children: ReactNode
}) {
  return (
    <div className="settings-group mb-6">
      {title && (
        <h3 className="mb-1.5 px-1 text-2xs font-semibold tracking-wide text-fg-subtle uppercase">
          {title}
        </h3>
      )}
      <div className="rounded-xl bg-well px-4 hairline">{children}</div>
      {description && <p className="mt-1.5 px-1 text-xs text-fg-subtle">{description}</p>}
    </div>
  )
}

export function Section({
  id,
  changed = 0,
  onResetAll,
  children,
}: {
  id: string
  changed?: number
  onResetAll?: () => Promise<unknown> | void
  children: ReactNode
}) {
  const info = sectionInfo(id)
  const searching = useContext(QueryContext).words.length > 0
  const Icon = info.icon
  const resetAll = async () => {
    const ok = await confirm({
      title: `Reset ${info.title} settings?`,
      message: `${changed === 1 ? '1 setting goes' : `${changed} settings go`} back to ${changed === 1 ? 'its' : 'their'} default.`,
      confirmLabel: 'Reset',
    })
    if (ok) await onResetAll?.()
  }
  return (
    <SectionContext.Provider value={info.title}>
      <section
        data-settings-section={id}
        aria-labelledby={`settings-${id}`}
        className="settings-section mb-10 scroll-mt-6"
      >
        <header className="mb-4 flex items-center gap-3">
          <span
            className="flex size-8 shrink-0 items-center justify-center rounded-[9px] text-white shadow-sm"
            style={{ background: info.color }}
          >
            <Icon className="size-4" strokeWidth={2.2} />
          </span>
          <div className="min-w-0 flex-1">
            <h2
              id={`settings-${id}`}
              className="text-md leading-tight font-semibold tracking-tight"
            >
              {info.title}
            </h2>
            {!searching && <p className="truncate text-xs text-fg-subtle">{info.description}</p>}
          </div>
          {changed > 0 && onResetAll && (
            <button
              onClick={() => void resetAll()}
              className="flex h-6 shrink-0 items-center gap-1 rounded-full px-2 text-xs text-fg-subtle transition-colors hover:bg-hover hover:text-fg"
            >
              <RotateCcw className="size-3" />
              Reset section
            </button>
          )}
        </header>
        {children}
      </section>
    </SectionContext.Provider>
  )
}

export function Select<T extends string>({
  value,
  options,
  onChange,
  label,
  className,
}: {
  value: T
  options: Array<{ value: T; label: string; description?: string }>
  onChange: (v: T) => void
  label: string
  className?: string
}) {
  const current = options.find((o) => o.value === value)
  return (
    <DropdownMenu
      align="end"
      items={options.map((o) => ({
        label: o.label,
        description: o.description,
        checked: o.value === value,
        onSelect: () => onChange(o.value),
      }))}
      trigger={
        <button
          aria-label={label}
          className={cn(
            'flex h-7 min-w-36 items-center justify-between gap-2 rounded-full bg-content pr-2 pl-3 text-sm hairline hover:bg-hover',
            className,
          )}
        >
          <span className="truncate">{current?.label ?? (value || '—')}</span>
          <ChevronDown className="size-3.5 shrink-0 text-fg-subtle" />
        </button>
      }
    />
  )
}

export function Stepper({
  value,
  min,
  max,
  step,
  onChange,
  format = String,
  label,
}: {
  value: number
  min: number
  max: number
  step: number
  onChange: (v: number) => void
  format?: (v: number) => string
  label: string
}) {
  const round = (v: number) => Math.round(v * 100) / 100
  const dec = () => onChange(round(Math.max(min, value - step)))
  const inc = () => onChange(round(Math.min(max, value + step)))
  return (
    <div
      className="flex h-7 items-center rounded-full bg-content hairline"
      role="spinbutton"
      tabIndex={0}
      aria-label={label}
      aria-valuenow={value}
      aria-valuemin={min}
      aria-valuemax={max}
      aria-valuetext={format(value)}
      onKeyDown={(e) => {
        if (e.key === 'ArrowUp' || e.key === 'ArrowRight') inc()
        else if (e.key === 'ArrowDown' || e.key === 'ArrowLeft') dec()
        else return
        e.preventDefault()
      }}
    >
      <button
        tabIndex={-1}
        aria-label={`Decrease ${label}`}
        disabled={value <= min}
        onClick={dec}
        className="flex size-7 items-center justify-center rounded-full text-fg-muted hover:bg-hover hover:text-fg disabled:opacity-35"
      >
        <Minus className="size-3.5" />
      </button>
      <span className="min-w-12 text-center text-sm tabular-nums">{format(value)}</span>
      <button
        tabIndex={-1}
        aria-label={`Increase ${label}`}
        disabled={value >= max}
        onClick={inc}
        className="flex size-7 items-center justify-center rounded-full text-fg-muted hover:bg-hover hover:text-fg disabled:opacity-35"
      >
        <Plus className="size-3.5" />
      </button>
    </div>
  )
}

export function CommitInput({
  value,
  onCommit,
  placeholder,
  label,
  mono,
  secret,
  numeric,
  className,
}: {
  value: string
  onCommit: (v: string) => void
  placeholder?: string
  label: string
  mono?: boolean
  secret?: boolean
  numeric?: boolean
  className?: string
}) {
  const [draft, setDraft] = useState(value)
  useEffect(() => setDraft(value), [value])
  const commit = () => {
    const next = draft.trim()
    if (next !== value) onCommit(next)
  }
  return (
    <label
      className={cn(
        'flex h-7 items-center rounded-full bg-content px-3 hairline focus-within:shadow-[0_0_0_2px_var(--focus)]',
        className,
      )}
    >
      <input
        value={draft}
        type={secret ? 'password' : 'text'}
        inputMode={numeric ? 'decimal' : undefined}
        aria-label={label}
        placeholder={placeholder}
        spellCheck={false}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={commit}
        onKeyDown={(e) => {
          if (isComposing(e)) return
          if (e.key === 'Enter') commit()
          if (e.key === 'Escape') setDraft(value)
        }}
        className={cn(
          'min-w-0 flex-1 bg-transparent text-base text-fg outline-none placeholder:font-sans placeholder:text-fg-subtle',
          mono && 'font-mono text-xs',
          numeric && 'text-right tabular-nums',
        )}
      />
    </label>
  )
}
