import { Kbd, Modal } from '@/components/ui/primitives'

/**
 * The shortcuts sheet is written out rather than derived from the live hotkey
 * registry: bindings are registered by whichever component happens to be
 * mounted, and a reference card that changes depending on where you are is
 * worse than one that does not.
 */
const SECTIONS: { group: string; items: { keys: string; label: string }[] }[] = [
  {
    group: 'General',
    items: [
      { keys: 'mod+k', label: 'Command palette — jump to any video' },
      { keys: 'mod+n', label: 'New video' },
      { keys: 'mod+b', label: 'Show or hide the sidebar' },
      { keys: 'shift+?', label: 'This sheet' },
      { keys: 'escape', label: 'Dismiss the topmost surface' },
    ],
  },
  {
    group: 'Navigation',
    items: [
      { keys: 'mod+1', label: 'Videos' },
      { keys: 'mod+2', label: 'Channels' },
      { keys: 'mod+3', label: 'Scheduler' },
      { keys: 'mod+4', label: 'Settings' },
      { keys: 'mod+f', label: 'Filter the sidebar' },
    ],
  },
  {
    group: 'Sidebar',
    items: [
      { keys: 'alt+arrowup', label: 'Previous video' },
      { keys: 'alt+arrowdown', label: 'Next video' },
      { keys: 'arrowup', label: 'Move the cursor up (while the list has focus)' },
      { keys: 'arrowdown', label: 'Move the cursor down' },
      { keys: 'enter', label: 'Open the video under the cursor' },
    ],
  },
  {
    group: 'Video',
    items: [
      { keys: 'mod+enter', label: 'Approve the open gate' },
      { keys: 'mod+arrowleft', label: 'Previous tab' },
      { keys: 'mod+arrowright', label: 'Next tab' },
    ],
  },
]

export function ShortcutsSheet({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title="Keyboard shortcuts"
      description="Everything here is reachable without the mouse."
      wide
    >
      <div className="grid grid-cols-2 gap-x-8 gap-y-5">
        {SECTIONS.map((section) => (
          <section key={section.group}>
            <h3 className="mb-1.5 text-[11px] font-semibold uppercase tracking-wider text-subtle">
              {section.group}
            </h3>
            <dl className="divide-y divide-[hsl(var(--border))]">
              {section.items.map((item) => (
                <div key={item.keys + item.label} className="flex items-center gap-4 py-1.5">
                  <dt className="min-w-0 flex-1 text-[12.5px] text-fg">{item.label}</dt>
                  <dd className="shrink-0">
                    <Kbd keys={item.keys} />
                  </dd>
                </div>
              ))}
            </dl>
          </section>
        ))}
      </div>
    </Modal>
  )
}
