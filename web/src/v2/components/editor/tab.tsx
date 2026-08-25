import type { IDockviewPanelHeaderProps } from 'dockview-react'
import { Plus, X } from 'lucide-react'
import { useEffect, useState } from 'react'

import { cn } from '../../core/utils'
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

  return (
    <div
      data-tab-id={api.id}
      className={cn(
        'group/tab flex h-full w-full items-center gap-2 pr-1.5 pl-3',
        active ? 'text-primary' : 'text-secondary',
      )}
      onDoubleClick={() => pinPreview(api.id)}
      onAuxClick={(event) => {
        // Middle-click closes, as it does in every tabbed thing.
        if (event.button === 1) api.close()
      }}
      onPointerEnter={() => setHovered(true)}
      onPointerLeave={() => setHovered(false)}
    >
      <Avatar
        name={params.initial ?? params.title ?? '?'}
        seed={params.seed ?? api.id}
        icon={isNew ? Plus : undefined}
        className={cn('size-5 text-[11px]', active ? '' : 'opacity-80')}
      />
      <span className={cn('min-w-0 flex-1 truncate text-[13px]', preview && 'italic')}>
        {params.title ?? api.title}
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
          'flex size-[18px] shrink-0 items-center justify-center rounded-[4px]',
          'hover:bg-[var(--hover)] hover:text-primary',
          active || hovered ? 'opacity-100' : 'opacity-0',
        )}
      >
        <X className="size-[13px]" strokeWidth={2} />
      </button>
    </div>
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
