import type { IDockviewPanelHeaderProps } from 'dockview-react'
import { Keyboard, Plus, Settings, X } from 'lucide-react'
import { useEffect, useState } from 'react'

import { cn } from '@/kit/lib/cn'
import { ContextMenu, type MenuEntry } from '@/kit/ui/Menu'

import { Avatar } from '../ui/avatar'
import { pinPreview, useDock, type DocPanelParams } from './dock'

/**
 * One tab.
 *
 * Custom rather than dockview's default, because three of the four things the
 * reference tab does are not styling: the coloured token that ties the tab to
 * its row in the source list, the italic that says *this tab is a preview and
 * the next click will take it*, and the close control that only appears on the
 * tab you are actually on.
 *
 * The fourth is that the strip is the titlebar, so a tab is also somewhere the
 * window can be picked up from — by the long press that is not a drag of the
 * tab itself.
 */
export function EditorTab({ api, params }: IDockviewPanelHeaderProps<DocPanelParams>) {
  const active = useActive(api)
  const preview = useDock((s) => s.previewId) === api.id
  const [hovered, setHovered] = useState(false)

  const doc = params.doc
  const isNew = doc?.kind === 'new'
  const Glyph = doc?.kind === 'settings' ? Settings : doc?.kind === 'keybindings' ? Keyboard : null

  const menu = (): MenuEntry[] => {
    const panels = api.group.panels
    const at = panels.findIndex((panel) => panel.id === api.id)
    const close = (keep: (index: number) => boolean) =>
      panels.filter((_, index) => !keep(index)).forEach((panel) => panel.api.close())
    return [
      ...(preview
        ? [
            { label: 'Keep Open', onSelect: () => pinPreview(api.id) },
            { type: 'separator' as const },
          ]
        : []),
      { label: 'Close', shortcut: '$mod+w', onSelect: () => api.close() },
      {
        label: 'Close Others',
        shortcut: '$mod+Shift+w',
        disabled: panels.length < 2,
        onSelect: () => close((index) => index === at),
      },
      {
        label: 'Close to the Right',
        disabled: at === panels.length - 1,
        onSelect: () => close((index) => index <= at),
      },
      { label: 'Close All', onSelect: () => close(() => false) },
    ]
  }

  return (
    <ContextMenu items={menu}>
      <div
        data-tab-id={api.id}
        className={cn(
          'group/tab flex h-full w-full max-w-60 min-w-0 items-center gap-1.5',
          active ? 'text-fg' : 'text-fg-muted',
        )}
        onDoubleClick={() => pinPreview(api.id)}
        onAuxClick={(event) => {
          // Middle-click closes, as it does in every tabbed thing.
          if (event.button === 1) api.close()
        }}
        onPointerEnter={() => setHovered(true)}
        onPointerLeave={() => setHovered(false)}
      >
        {Glyph ? (
          <Glyph className={cn('size-3.5 shrink-0', active ? '' : 'opacity-75')} />
        ) : (
          <Avatar
            name={params.initial ?? params.title ?? '?'}
            seed={params.seed ?? api.id}
            icon={isNew ? Plus : undefined}
            className={cn('size-4 text-[9px]', active ? '' : 'opacity-75')}
          />
        )}
        <span
          className={cn(
            'flex min-w-0 flex-1 items-baseline gap-1.5 text-sm',
            active && 'font-medium',
            preview && 'italic',
          )}
        >
          {/* Before the title, not after it.

            The title truncates from the end, so a trailing ref is the first
            thing dropped — and it goes exactly when the strip is crowded, which
            is when two tabs are hardest to tell apart in the first place.
            Leading it, it survives every width. */}
          {doc?.kind === 'video' ? (
            <span className="shrink-0 font-normal text-fg-subtle tabular-nums">{doc.ref}</span>
          ) : null}
          <span className="min-w-0 truncate">{params.title ?? api.title}</span>
        </span>
        <button
          type="button"
          aria-label="Close the tab"
          tabIndex={-1}
          onClick={(event) => {
            event.stopPropagation()
            api.close()
          }}
          className={cn(
            'ml-0.5 flex size-4 shrink-0 items-center justify-center rounded-full transition-opacity duration-100',
            'hover:bg-hover hover:!opacity-100',
            hovered ? 'opacity-70' : active ? 'opacity-50' : 'opacity-0',
          )}
        >
          <X className="size-3" strokeWidth={2.2} />
        </button>
      </div>
    </ContextMenu>
  )
}

/** Dockview reports activity through an event, not a prop. */
function useActive(api: IDockviewPanelHeaderProps['api']): boolean {
  const [active, setActive] = useState(api.isActive)
  useEffect(() => {
    setActive(api.isActive)
    const subscription = api.onDidActiveChange((event) => setActive(event.isActive))
    return () => subscription.dispose()
  }, [api])
  return active
}
