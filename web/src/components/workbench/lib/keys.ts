import { createContext, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { tinykeys } from 'tinykeys'

/**
 * Keybindings and the command registry, in one place because they are one thing:
 * a command is what the palette lists, what a menu item runs and what a chord
 * fires, and three separate descriptions of it would drift.
 *
 * Bindings are written in tinykeys' notation — `$mod+KeyB`, `$mod+Shift+KeyE`,
 * `Alt+ArrowDown`. The physical-key spellings (`KeyB`, not `b`) are the point:
 * on macOS `event.key` for ⌥B is `∫`, so a layer that matches on `key` cannot
 * bind Option chords at all. Matching on `event.code` is what lets this carry
 * the reference's actual keymap instead of an approximation of it.
 */

export const IS_MAC =
  typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.userAgent)

export interface Command {
  /** Stable and unique; also the palette's identity for the row. */
  id: string
  label: string
  /** The heading the palette files it under. */
  category: string
  /** tinykeys binding. Omit for commands that are only ever run from the palette. */
  keys?: string
  run: () => void
  /** Fire even while a text field has focus — ⌘K, Escape. Defaults to false. */
  whileTyping?: boolean
  /** Bound, but not offered in the palette. */
  hidden?: boolean
  /** Greyed out and unrunnable; the reason is shown beside it. */
  disabled?: boolean
}

interface Registry {
  register: (commands: Command[]) => () => void
  snapshot: () => Command[]
  subscribe: (listener: () => void) => () => void
}

const RegistryContext = createContext<Registry | null>(null)

/**
 * Holds every command the mounted tree has declared and binds all of their
 * chords with a single listener.
 */
export function useCommandRegistry(): Registry {
  const sets = useRef(new Set<Command[]>())
  const listeners = useRef(new Set<() => void>())
  const cache = useRef<Command[]>([])
  const dirty = useRef(true)

  return useMemo<Registry>(() => {
    const notify = () => {
      dirty.current = true
      for (const listener of listeners.current) listener()
    }
    return {
      register: (commands) => {
        sets.current.add(commands)
        notify()
        return () => {
          sets.current.delete(commands)
          notify()
        }
      },
      // Referentially stable between changes, so `useSyncExternalStore`-shaped
      // consumers and effect dependencies do not spin.
      snapshot: () => {
        if (dirty.current) {
          const flat: Command[] = []
          const seen = new Set<string>()
          for (const set of sets.current) {
            for (const command of set) {
              // Last registration wins: a document's own binding overrides the
              // shell's default for as long as that document is open.
              if (seen.has(command.id)) {
                flat[flat.findIndex((c) => c.id === command.id)] = command
              } else {
                seen.add(command.id)
                flat.push(command)
              }
            }
          }
          cache.current = flat
          dirty.current = false
        }
        return cache.current
      },
      subscribe: (listener) => {
        listeners.current.add(listener)
        return () => listeners.current.delete(listener)
      },
    }
  }, [])
}

export const CommandRegistryContext = RegistryContext

function useRegistry(): Registry {
  const registry = useContext(RegistryContext)
  if (!registry) throw new Error('commands must be used inside the workbench')
  return registry
}

/**
 * Declares commands for as long as the component is mounted.
 *
 * Call sites write a fresh array literal every render, so registering on
 * identity would re-bind the whole window on every keystroke. Instead the
 * *handlers* are read through a ref — always the newest closure — and the
 * registration is renewed only when something the palette actually draws has
 * changed. Getting that second half wrong is how a command ends up permanently
 * disabled because it was disabled the first time it was registered.
 */
export function useCommands(commands: Command[]): void {
  const registry = useRegistry()
  const ref = useRef(commands)
  ref.current = commands

  const signature = commands
    .map(
      (c) =>
        `${c.id}|${c.keys ?? ''}|${c.label}|${c.category}|${c.disabled ? 1 : 0}|${c.hidden ? 1 : 0}|${c.whileTyping ? 1 : 0}`,
    )
    .join('\n')

  useEffect(() => {
    const proxy: Command[] = ref.current.map((command, index) => ({
      ...command,
      run: () => ref.current[index]?.run(),
    }))
    return registry.register(proxy)
  }, [registry, signature])
}

/** Every command currently declared, re-read when the registry changes. */
export function useAllCommands(): Command[] {
  const registry = useRegistry()
  const [, bump] = useState(0)
  useEffect(() => registry.subscribe(() => bump((n) => n + 1)), [registry])
  return registry.snapshot()
}

function isTyping(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  const tag = target.tagName
  return (
    tag === 'INPUT' ||
    tag === 'TEXTAREA' ||
    tag === 'SELECT' ||
    target.isContentEditable ||
    target.getAttribute('role') === 'textbox'
  )
}

/**
 * Binds every registered chord. One listener for the whole window, rebuilt when
 * the set of commands changes — which is rare, and never on a keystroke.
 */
export function useKeybindings(registry: Registry): void {
  const [version, bump] = useState(0)
  useEffect(() => registry.subscribe(() => bump((n) => n + 1)), [registry])

  useEffect(() => {
    const map: Record<string, (event: KeyboardEvent) => void> = {}
    for (const command of registry.snapshot()) {
      if (!command.keys) continue
      map[command.keys] = (event) => {
        if (command.disabled) return
        if (isTyping(event.target) && !command.whileTyping) return
        event.preventDefault()
        command.run()
      }
    }
    return tinykeys(window, map)
  }, [registry, version])
}

/* ------------------------------------------------------------------ labels */

const SYMBOLS: Record<string, string> = {
  $mod: IS_MAC ? '⌘' : 'Ctrl',
  meta: '⌘',
  control: 'Ctrl',
  ctrl: 'Ctrl',
  alt: IS_MAC ? '⌥' : 'Alt',
  shift: IS_MAC ? '⇧' : 'Shift',
  enter: '↵',
  escape: 'Esc',
  backspace: '⌫',
  delete: '⌦',
  space: 'Space',
  tab: '⇥',
  arrowup: '↑',
  arrowdown: '↓',
  arrowleft: '←',
  arrowright: '→',
  comma: ',',
  period: '.',
  slash: '/',
  backslash: '\\',
  backquote: '`',
  minus: '−',
  equal: '=',
  bracketleft: '[',
  bracketright: ']',
}

/** The pieces of a binding, ready to be drawn as individual keycaps. */
export function keycaps(keys: string): string[] {
  return keys.split('+').map((raw) => {
    const part = raw.trim()
    const lower = part.toLowerCase()
    if (SYMBOLS[lower]) return SYMBOLS[lower]
    // `KeyB` → B, `Digit1` → 1: the physical spellings are for the matcher, not
    // for the reader.
    if (/^key[a-z]$/i.test(part)) return part.slice(3).toUpperCase()
    if (/^digit\d$/i.test(part)) return part.slice(5)
    return part.length === 1 ? part.toUpperCase() : part
  })
}
