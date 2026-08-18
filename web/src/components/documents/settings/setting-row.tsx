import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Check, Eye, EyeOff, KeyRound, Loader2 } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'

import { settingTitle, settingUnit } from './meta'
import { Badge, Button, Input, Segmented, Switch } from '../../ui/controls'
import { CopyButton, ErrorNotice, Tooltip } from '../../ui/primitives'
import { api, qk } from '@/core/api'
import { formatRelative } from '@/core/format'
import type { Setting } from '@/core/types'
import { cn } from '@/core/utils'

/**
 * One setting.
 *
 * The row is a heading, the identifier, a sentence of prose and a control — in
 * that order, because that is the order the question is asked in: what is this,
 * what is it called, what does it do, what is it set to.
 *
 * A control that carries its own value — a switch, a select, a suggestion —
 * commits the moment it changes. A free-text field waits for blur or Enter, so
 * a half-typed number is never sent, and says so in its own status line while
 * it is holding something unsaved.
 */
export function SettingRow({ row, dormant }: { row: Setting; dormant: boolean }) {
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState(row.value)
  const [saved, setSaved] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)

  // Another client — or the server itself — can move a value under us.
  useEffect(() => setDraft(row.value), [row.value])
  useEffect(() => () => clearTimeout(timer.current), [])

  const update = useMutation({
    mutationFn: (value: string) => api.updateSetting(row.key, value),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: qk.settings })
      setSaved(true)
      clearTimeout(timer.current)
      timer.current = setTimeout(() => setSaved(false), 1600)
    },
  })

  const commit = (value: string) => {
    if (value === row.value) return
    update.mutate(value)
  }

  // A secret's value never arrives, so `value === row.value` cannot tell an
  // untouched field from a cleared one. Clearing is its own button instead.
  const commitSecret = (value: string) => update.mutate(value)

  const unit = settingUnit(row)
  const bounded = row.min !== row.max
  const dirty = !row.secret && draft !== row.value

  return (
    <li
      className={cn(
        'group grid grid-cols-[minmax(0,1fr)_268px] items-start gap-x-8 gap-y-2 px-5 py-3.5',
        'transition-colors hover:bg-[hsl(var(--bg-hover)/0.45)]',
      )}
    >
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <h3
            className={cn('text-[12.5px] font-semibold leading-5 text-fg', dormant && 'text-muted')}
          >
            {settingTitle(row)}
          </h3>

          {row.backend && (
            <Tooltip
              label={
                dormant
                  ? `Nothing reads this: no provider is set to ${row.backend}. The value is kept either way.`
                  : `Read by the ${row.backend} backend, which is in use.`
              }
            >
              {/* A span rather than the badge itself: the trigger is given a ref. */}
              <span className="inline-flex cursor-default">
                <Badge
                  tone={dormant ? 'neutral' : 'info'}
                  dot={dormant}
                  className="px-1.5 text-[10px] font-normal leading-[15px]"
                >
                  {row.backend}
                </Badge>
              </span>
            </Tooltip>
          )}

          {row.secret && (
            <Badge
              tone="violet"
              className="cursor-default px-1.5 text-[10px] font-normal leading-[15px]"
            >
              <KeyRound className="h-2.5 w-2.5" aria-hidden />
              secret
            </Badge>
          )}
        </div>

        {row.description && (
          <p className="mt-1 max-w-prose text-[11.5px] leading-[1.55] text-muted">
            {row.description}
          </p>
        )}

        <div className="mt-1.5 flex items-center gap-1">
          <Tooltip label={`Last changed ${formatRelative(row.updatedAt)}`}>
            <code className="cursor-default font-mono text-[10.5px] text-subtle">{row.key}</code>
          </Tooltip>
          <CopyButton
            value={row.key}
            label="Copy the key"
            className="opacity-0 transition-opacity focus-visible:opacity-100 group-hover:opacity-100"
          />
        </div>
      </div>

      <div className="min-w-0">
        {row.type === 'bool' ? (
          <div className="flex h-8 items-center gap-2">
            <Switch
              checked={draft === 'true'}
              onCheckedChange={(next) => {
                const value = next ? 'true' : 'false'
                setDraft(value)
                commit(value)
              }}
              aria-label={row.key}
            />
            <span
              className={cn(
                'text-[11.5px] font-medium',
                draft === 'true' ? 'text-fg' : 'text-subtle',
              )}
            >
              {draft === 'true' ? 'On' : 'Off'}
            </span>
          </div>
        ) : row.secret ? (
          <SecretField row={row} draft={draft} setDraft={setDraft} commit={commitSecret} />
        ) : row.options.length > 0 ? (
          /* The same control the Providers section uses, at the width a
             two-column row allows. Filtering is the only way a provider row is
             reached from here, and a different control in that one place would
             be a different answer to the same question. */
          <Segmented
            aria-label={row.key}
            size="sm"
            value={draft}
            onChange={(next) => {
              setDraft(next)
              commit(next)
            }}
            options={row.options.map((option) => ({ value: option, label: option }))}
          />
        ) : (
          <>
            <div className="relative">
              <Input
                value={draft}
                aria-label={row.key}
                type={row.type === 'int' || row.type === 'float' ? 'number' : 'text'}
                spellCheck={false}
                autoComplete="off"
                {...(bounded ? { min: row.min, max: row.max } : {})}
                {...(row.type === 'float' ? { step: 0.1 } : {})}
                onChange={(event) => setDraft(event.target.value)}
                onBlur={() => commit(draft)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault()
                    commit(draft)
                  } else if (event.key === 'Escape') {
                    event.stopPropagation()
                    setDraft(row.value)
                  }
                }}
                className={cn(
                  // The spinners a number input draws sit exactly where the unit
                  // does, and a settings table is not stepped through anyway.
                  '[&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none',
                  '[appearance:textfield]',
                  unit && 'pr-10',
                  dirty && 'border-[hsl(var(--accent))]',
                )}
              />
              {unit && (
                <span
                  aria-hidden
                  className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-[10.5px] text-subtle"
                >
                  {unit}
                </span>
              )}
            </div>

            {/* Advisory: the catalogue these come from lives on someone else's
                server, so the field still takes anything. */}
            {row.suggestions.length > 0 && (
              <div className="mt-1.5 flex flex-wrap gap-1">
                {row.suggestions.slice(0, 4).map((suggestion) => (
                  <button
                    key={suggestion.value}
                    type="button"
                    title={suggestion.value}
                    onClick={() => {
                      setDraft(suggestion.value)
                      commit(suggestion.value)
                    }}
                    className={cn(
                      'max-w-full truncate rounded-full border px-2 text-[10px] leading-[17px] transition-colors',
                      suggestion.value === row.value
                        ? 'border-[hsl(var(--accent)/0.4)] bg-[hsl(var(--accent-soft))] text-[hsl(var(--accent))]'
                        : 'border-[hsl(var(--border))] text-subtle hover:border-[hsl(var(--border-strong))] hover:text-fg',
                    )}
                  >
                    {suggestion.label || suggestion.value}
                  </button>
                ))}
              </div>
            )}
          </>
        )}

        {/* One line, always present, so nothing below it moves when it changes.
            The hint omits the unit: the field is already showing it. */}
        <StatusLine
          saving={update.isPending}
          saved={saved}
          dirty={dirty}
          hint={bounded ? `${row.min} – ${row.max}` : ''}
        />
      </div>

      {update.isError && (
        <div className="col-span-2">
          <ErrorNotice error={update.error} />
        </div>
      )}
    </li>
  )
}

/**
 * What the control is doing, under the control. Precedence is by urgency: an
 * unsaved edit outranks the reminder of what the bounds are, because one is
 * about this moment and the other is about the field in general.
 */
function StatusLine({
  saving,
  saved,
  dirty,
  hint,
}: {
  saving: boolean
  saved: boolean
  dirty: boolean
  hint: string
}) {
  return (
    <p className="mt-1.5 flex h-4 items-center gap-1 text-[10.5px] leading-4">
      {saving ? (
        <>
          <Loader2 className="h-3 w-3 shrink-0 animate-spin text-subtle" aria-hidden />
          <span className="text-subtle">Saving…</span>
        </>
      ) : saved ? (
        <>
          <Check className="h-3 w-3 shrink-0 text-[hsl(var(--success))]" aria-hidden />
          <span className="text-[hsl(var(--success))]">Saved</span>
        </>
      ) : dirty ? (
        <span className="text-[hsl(var(--accent))]">Enter to save · Esc to revert</span>
      ) : (
        <span className="tabular text-subtle">{hint}</span>
      )}
    </p>
  )
}

/* ------------------------------------------------------------------ secret */

/**
 * A credential. It differs from every other field in one way that shapes the
 * rest: the value is never sent back, so this cannot show what is stored — only
 * whether something is.
 *
 * That makes an empty box ambiguous, which is why nothing commits on blur here.
 * A blank field means "leave it alone", saving takes Enter or the button, and
 * removing a stored key is its own explicit action rather than the accident of
 * tabbing through a form.
 */
function SecretField({
  row,
  draft,
  setDraft,
  commit,
}: {
  row: Setting
  draft: string
  setDraft: (value: string) => void
  commit: (value: string) => void
}) {
  const [visible, setVisible] = useState(false)
  const typed = draft.trim() !== ''

  return (
    <>
      <div className="flex items-start gap-1.5">
        <div className="relative min-w-0 flex-1">
          <Input
            value={draft}
            aria-label={row.key}
            type={visible ? 'text' : 'password'}
            autoComplete="off"
            spellCheck={false}
            placeholder={row.configured ? '•••••••••••• stored' : 'not set'}
            onChange={(event) => setDraft(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                event.preventDefault()
                if (typed) commit(draft)
              } else if (event.key === 'Escape') {
                event.stopPropagation()
                setDraft('')
              }
            }}
            className={cn('pr-8 font-mono text-[12px]', typed && 'border-[hsl(var(--accent))]')}
          />
          {typed && (
            <button
              type="button"
              onClick={() => setVisible((on) => !on)}
              aria-label={visible ? 'Hide the value' : 'Show the value'}
              className="absolute right-1.5 top-1/2 flex h-5 w-5 -translate-y-1/2 items-center justify-center rounded-[var(--radius-xs)] text-subtle transition-colors hover:bg-[hsl(var(--bg-hover))] hover:text-fg"
            >
              {visible ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
            </button>
          )}
        </div>
        {typed && (
          <Button
            variant="primary"
            size="sm"
            className="h-8 shrink-0"
            onClick={() => commit(draft)}
          >
            Save
          </Button>
        )}
      </div>

      <p className="mt-1.5 flex h-4 items-center gap-1.5 text-[10.5px] leading-4">
        <span
          aria-hidden
          className={cn(
            'h-1.5 w-1.5 shrink-0 rounded-full',
            row.configured ? 'bg-[hsl(var(--success))]' : 'bg-[hsl(var(--fg-subtle))]',
          )}
        />
        <span className="text-subtle">{row.configured ? 'A key is stored' : 'No key stored'}</span>
        {row.configured && !typed && (
          <button
            type="button"
            onClick={() => {
              setDraft('')
              commit('')
            }}
            className="ml-auto text-[hsl(var(--danger))] transition-opacity hover:opacity-75"
          >
            Remove
          </button>
        )}
      </p>
    </>
  )
}
