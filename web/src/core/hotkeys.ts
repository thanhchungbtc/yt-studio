import { useEffect, useRef } from 'react'

/**
 * Keyboard shortcuts, the way a desktop application has them: global, always
 * live, and inert while the operator is typing into a field.
 *
 * A binding is written the way it is printed — `mod+k`, `shift+?`, `alt+down` —
 * where `mod` is ⌘ on a Mac and Ctrl everywhere else, so one string serves both
 * the handler and the label in the shortcuts sheet.
 */

export const IS_MAC =
  typeof navigator !== 'undefined' &&
  /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent)

export interface Binding {
  /** e.g. `mod+k`, `alt+ArrowDown`, `shift+/`, `g then v` is not supported. */
  keys: string
  /** What the shortcut does; shown in the shortcuts sheet. */
  label: string
  /** Grouping in the shortcuts sheet. */
  group: string
  run: (event: KeyboardEvent) => void
  /** Fire even while a text field has focus (Escape, ⌘K). Defaults to false. */
  whileTyping?: boolean
  /** Omit from the shortcuts sheet. */
  hidden?: boolean
}

function normalise(keys: string): string {
  return keys
    .toLowerCase()
    .split('+')
    .map((part) => part.trim())
    .sort()
    .join('+')
}

function eventSignature(event: KeyboardEvent): string {
  const parts: string[] = []
  if (event.metaKey || event.ctrlKey) parts.push('mod')
  if (event.altKey) parts.push('alt')
  if (event.shiftKey) parts.push('shift')
  parts.push(event.key.toLowerCase())
  return parts.sort().join('+')
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
 * Registers a set of bindings for as long as the component is mounted. The
 * array is read through a ref, so passing a fresh array literal every render —
 * which every call site does — does not re-attach the listener.
 */
export function useHotkeys(bindings: Binding[], enabled = true): void {
  const ref = useRef(bindings)
  ref.current = bindings

  useEffect(() => {
    if (!enabled) return

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.repeat) return
      const signature = eventSignature(event)
      const typing = isTyping(event.target)
      for (const binding of ref.current) {
        if (normalise(binding.keys) !== signature) continue
        if (typing && !binding.whileTyping) continue
        event.preventDefault()
        binding.run(event)
        return
      }
    }

    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [enabled])
}

const SYMBOLS: Record<string, string> = {
  mod: IS_MAC ? '⌘' : 'Ctrl',
  alt: IS_MAC ? '⌥' : 'Alt',
  shift: IS_MAC ? '⇧' : 'Shift',
  enter: '↵',
  escape: 'Esc',
  arrowup: '↑',
  arrowdown: '↓',
  arrowleft: '←',
  arrowright: '→',
  backspace: '⌫',
}

/** The pieces of a binding, ready to be drawn as individual keycaps. */
export function keycaps(keys: string): string[] {
  return keys
    .split('+')
    .map((part) => part.trim().toLowerCase())
    .map((part) => SYMBOLS[part] ?? (part.length === 1 ? part.toUpperCase() : part))
}
