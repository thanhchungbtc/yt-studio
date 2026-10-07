import type { LucideIcon } from 'lucide-react'

import { KeybindingHint } from '@/kit/ui/Kbd'

interface PlaceholderProps {
  icon: LucideIcon
  title: string
  detail: string
  command?: string
}

/**
 * What an editor shows before it is an editor.
 *
 * Every document in v2 starts here, and each one is replaced by real content in
 * its own step. Centring an icon over two lines is macOS's own empty state, so
 * a screen that has not been built yet still looks like it belongs.
 */
export function Placeholder({ icon: Icon, title, detail, command }: PlaceholderProps) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 px-8 text-center">
      <div className="glass-pill flex size-14 items-center justify-center p-0">
        <Icon className="size-6 text-fg-faint" strokeWidth={1.5} />
      </div>
      <div>
        <div className="text-md font-semibold text-fg">{title}</div>
        <div className="mt-1 max-w-80 text-sm text-fg-subtle">{detail}</div>
      </div>
      {command ? <KeybindingHint command={command} /> : null}
    </div>
  )
}
