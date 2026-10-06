import { useEffect, useMemo, useRef, useState } from 'react'
import { AlertTriangle, Pencil, Plus, RotateCcw, Search, Trash2 } from 'lucide-react'
import { cn } from '@/kit/lib/cn'
import { defaultKeybindings, eventToPress, type KeybindingRule } from '@/kit/commands/keybindings'
import { useCommandRegistry } from '@/kit/commands/registry'
import { useEffectiveKeybindings } from '@/kit/commands/useKeybindings'
import { setPref, usePref } from '@/prefs'
import { Kbd } from '@/kit/ui/Kbd'
import { Button } from '@/kit/ui/Button'
import { Badge, TextInput } from '@/kit/ui/misc'
import { Tooltip } from '@/kit/ui/Tooltip'

/** Captures a key combination (chords: press a second combo within 1s). */
function KeyRecorder({
  onDone,
  onCancel,
}: {
  onDone: (key: string) => void
  onCancel: () => void
}) {
  const [presses, setPresses] = useState<string[]>([])
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    document.body.dataset.recordingKeys = 'true'
    ref.current?.focus()
    return () => {
      delete document.body.dataset.recordingKeys
      clearTimeout(timer.current)
    }
  }, [])

  return (
    <div
      ref={ref}
      tabIndex={0}
      onKeyDown={(e) => {
        e.preventDefault()
        e.stopPropagation()
        if (e.key === 'Escape' && presses.length === 0) return onCancel()
        if (e.key === 'Enter' && presses.length > 0 && !e.metaKey) return onDone(presses.join(' '))
        const p = eventToPress(e.nativeEvent)
        if (!p) return
        const next = [...presses, p].slice(-2)
        setPresses(next)
        clearTimeout(timer.current)
        timer.current = setTimeout(() => onDone(next.join(' ')), 1100)
      }}
      onBlur={onCancel}
      className="flex h-7 min-w-44 items-center gap-2 rounded-[7px] bg-accent-soft px-2 text-xs text-accent outline-none shadow-[0_0_0_1.5px_var(--accent)]"
    >
      {presses.length ? <Kbd keys={presses.join(' ')} /> : 'Press keys…'}
    </div>
  )
}

export function KeybindingsPanel() {
  const commands = useCommandRegistry((s) => s.commands)
  const user = usePref('keybindings.user')
  const effective = useEffectiveKeybindings()
  const [query, setQuery] = useState('')
  const [recording, setRecording] = useState<string>()

  const rows = useMemo(() => {
    const q = query.toLowerCase()
    return Object.values(commands)
      .filter(
        (c) =>
          !q ||
          c.title.toLowerCase().includes(q) ||
          c.id.toLowerCase().includes(q) ||
          c.category?.toLowerCase().includes(q),
      )
      .sort(
        (a, b) =>
          (a.category ?? '').localeCompare(b.category ?? '') || a.title.localeCompare(b.title),
      )
      .map((c) => ({
        command: c,
        rules: effective.filter((r) => r.command === c.id),
        customized: user.some((r) => r.command === c.id || r.command === `-${c.id}`),
      }))
  }, [commands, effective, user, query])

  const conflicts = useMemo(() => {
    const byKey = new Map<string, string[]>()
    for (const r of effective) {
      const k = `${r.key}|${r.when ?? ''}`
      byKey.set(k, [...(byKey.get(k) ?? []), r.command])
    }
    return byKey
  }, [effective])

  const save = (next: KeybindingRule[]) => setPref('keybindings.user', next)

  const setBinding = (commandId: string, key: string) => {
    const defaults = defaultKeybindings.filter((d) => d.command === commandId)
    const others = user.filter((r) => r.command !== commandId && r.command !== `-${commandId}`)
    const removals = defaults.map((d) => ({ key: d.key, command: `-${commandId}` }))
    const when = defaults[0]?.when
    save([...others, ...removals, { key, command: commandId, when }])
  }

  const addBinding = (commandId: string, key: string) =>
    save([...user, { key, command: commandId }])

  const removeBinding = (commandId: string, rule: KeybindingRule) => {
    const isDefault = defaultKeybindings.some((d) => d.command === commandId && d.key === rule.key)
    const withoutUser = user.filter((r) => !(r.command === commandId && r.key === rule.key))
    save(isDefault ? [...withoutUser, { key: rule.key, command: `-${commandId}` }] : withoutUser)
  }

  const reset = (commandId: string) =>
    save(user.filter((r) => r.command !== commandId && r.command !== `-${commandId}`))

  return (
    <div className="flex h-full flex-col bg-content">
      <div className="edge-line relative z-[1] flex items-center gap-3 px-6 py-3">
        <h1 className="text-md font-semibold">Keyboard Shortcuts</h1>
        <TextInput
          value={query}
          onChange={setQuery}
          placeholder="Search commands"
          icon={Search}
          className="ml-auto w-72"
          autoFocus
        />
        {user.length > 0 && (
          <Button size="sm" variant="ghost" icon={RotateCcw} onClick={() => save([])}>
            Reset all
          </Button>
        )}
      </div>
      <div className="scroll-edge min-h-0 flex-1 overflow-auto px-6 py-2">
        <table className="w-full text-sm">
          <thead className="sticky top-0 z-10 bg-[var(--content-solid)] text-left text-2xs tracking-wide text-fg-subtle uppercase">
            <tr>
              <th className="py-2 font-semibold">Command</th>
              <th className="py-2 font-semibold">Keybinding</th>
              <th className="py-2 font-semibold">When</th>
              <th className="w-24" />
            </tr>
          </thead>
          <tbody>
            {rows.map(({ command, rules, customized }) => (
              <tr key={command.id} className="group/kb border-t border-line hover:bg-hover">
                <td className="py-2 pr-4">
                  <div className="flex items-center gap-2">
                    <span className="font-medium">{command.title}</span>
                    {customized && <Badge tone="accent">custom</Badge>}
                  </div>
                  <div className="font-mono text-2xs text-fg-subtle">
                    {command.category ? `${command.category} · ` : ''}
                    {command.id}
                  </div>
                </td>
                <td className="py-2 pr-4">
                  {recording === command.id ? (
                    <KeyRecorder
                      onCancel={() => setRecording(undefined)}
                      onDone={(key) => {
                        setRecording(undefined)
                        setBinding(command.id, key)
                      }}
                    />
                  ) : recording === `+${command.id}` ? (
                    <KeyRecorder
                      onCancel={() => setRecording(undefined)}
                      onDone={(key) => {
                        setRecording(undefined)
                        addBinding(command.id, key)
                      }}
                    />
                  ) : rules.length === 0 ? (
                    <span className="text-xs text-fg-faint">—</span>
                  ) : (
                    <div className="flex flex-wrap items-center gap-2">
                      {rules.map((r) => {
                        const clash = (conflicts.get(`${r.key}|${r.when ?? ''}`)?.length ?? 0) > 1
                        return (
                          <span key={r.key} className="inline-flex items-center gap-1">
                            <Kbd keys={r.key} />
                            {clash && (
                              <Tooltip
                                content={`Also bound to: ${conflicts
                                  .get(`${r.key}|${r.when ?? ''}`)!
                                  .filter((c) => c !== command.id)
                                  .join(', ')}`}
                              >
                                <AlertTriangle className="size-3.5 text-warning" />
                              </Tooltip>
                            )}
                            <button
                              aria-label="Remove keybinding"
                              onClick={() => removeBinding(command.id, r)}
                              className="hidden size-4 items-center justify-center rounded text-fg-subtle group-hover/kb:flex hover:text-danger"
                            >
                              <Trash2 className="size-3" />
                            </button>
                          </span>
                        )
                      })}
                    </div>
                  )}
                </td>
                <td className="py-2 pr-4 font-mono text-2xs text-fg-subtle">
                  {rules[0]?.when ?? command.when ?? ''}
                </td>
                <td className="py-2">
                  <div
                    className={cn('flex justify-end gap-0.5 opacity-0 group-hover/kb:opacity-100')}
                  >
                    <Tooltip content="Change keybinding">
                      <button
                        onClick={() => setRecording(command.id)}
                        className="flex size-6 items-center justify-center rounded hover:bg-active"
                      >
                        <Pencil className="size-3.5" />
                      </button>
                    </Tooltip>
                    <Tooltip content="Add another keybinding">
                      <button
                        onClick={() => setRecording(`+${command.id}`)}
                        className="flex size-6 items-center justify-center rounded hover:bg-active"
                      >
                        <Plus className="size-3.5" />
                      </button>
                    </Tooltip>
                    {customized && (
                      <Tooltip content="Reset to default">
                        <button
                          onClick={() => reset(command.id)}
                          className="flex size-6 items-center justify-center rounded hover:bg-active"
                        >
                          <RotateCcw className="size-3.5" />
                        </button>
                      </Tooltip>
                    )}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
