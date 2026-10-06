import type { LucideIcon } from 'lucide-react'

import { cn } from '../../core/utils'

/**
 * A stable colour per channel.
 *
 * macOS gives every conversation a coloured token so the eye can find a thread
 * before it has read a word. Hashing the seed means that colour is the same on
 * every machine and after every reload without anyone having to store it — and
 * the same channel is the same colour in the source list, in its tab and in its
 * title strip, which is what ties the three together.
 */
const PALETTE = ['#e8836a', '#5b9bd5', '#7fb069', '#c77dbb', '#e0a458', '#6b8fbf', '#d4736f']

export function avatarColor(seed: string): string {
  let hash = 0
  for (let i = 0; i < seed.length; i += 1) hash = (hash * 31 + seed.charCodeAt(i)) >>> 0
  return PALETTE[hash % PALETTE.length] ?? PALETTE[0]!
}

interface AvatarProps {
  /** Where the initial comes from, and the colour when no seed is given. */
  name: string
  seed?: string
  /** Drawn instead of the initial, for documents that are not a thing yet. */
  icon?: LucideIcon
  className?: string
}

export function Avatar({ name, seed, icon: Icon, className }: AvatarProps) {
  return (
    <div
      className={cn(
        'flex size-8 shrink-0 items-center justify-center rounded-full',
        'text-[13px] font-semibold text-white',
        className,
      )}
      style={{ backgroundColor: avatarColor(seed ?? name) }}
      aria-hidden
    >
      {Icon ? (
        <Icon className="size-1/2" strokeWidth={2.25} />
      ) : (
        name.trim().slice(0, 1).toUpperCase() || '?'
      )}
    </div>
  )
}
