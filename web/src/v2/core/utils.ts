import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

/** Conditional classes, with later Tailwind utilities winning over earlier. */
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs))
}

/**
 * A random v4 UUID.
 *
 * `crypto.randomUUID` only exists in a secure context, so a page opened over
 * plain http from another host would throw on the first dialog it rendered.
 * `getRandomValues` has no such restriction, and is all a v4 UUID needs.
 */
export function uuid(): string {
  if (typeof crypto.randomUUID === 'function') return crypto.randomUUID()
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  bytes[6] = (bytes[6]! & 0x0f) | 0x40
  bytes[8] = (bytes[8]! & 0x3f) | 0x80
  const hex = [...bytes].map((byte) => byte.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}
