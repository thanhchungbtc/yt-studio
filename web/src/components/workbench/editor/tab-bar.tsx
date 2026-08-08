import { Columns2, Film, Settings as SettingsIcon, Tv, X } from 'lucide-react'
import { useState } from 'react'

import { Button, IconButton } from '../ui/controls'
import { ContextMenu, MenuItem, MenuLabel, MenuSeparator } from '../ui/menu'
import { Kbd, Modal, Tooltip } from '../ui/primitives'
import { docTitle, useWorkbenchStore, type Group, type Tab } from '../lib/store'
import { cn } from '@/core/utils'

/**
 * The tab strip for one editor group.
 *
 * A preview tab is drawn in italic and is the only tab a single click from the
 * explorer will replace. That is the whole reason tabs are affordable here: you
 * can walk twenty videos with the arrow keys and end with one tab, not twenty.
 */
export function TabBar({ group, focused }: { group: Group; focused: boolean }) {
  // Selected one at a time on purpose: a selector returning a fresh object is
  // never `Object.is`-equal to the last one, and zustand would re-render this
  // strip on every unrelated store write.
  const activate = useWorkbenchStore((s) => s.activate)
  const close = useWorkbenchStore((s) => s.close)
  const closeOthers = useWorkbenchStore((s) => s.closeOthers)
  const closeAll = useWorkbenchStore((s) => s.closeAll)
  const pin = useWorkbenchStore((s) => s.pin)
  const split = useWorkbenchStore((s) => s.split)
  const focusGroup = useWorkbenchStore((s) => s.focusGroup)
  const groupCount = useWorkbenchStore((s) => s.groups.length)

  // Asking before discarding lives here rather than in the tab, so the dialog
  // survives the tab unmounting the instant the close goes through.
  const [confirming, setConfirming] = useState<Tab | null>(null)

  const requestClose = (tab: Tab) => {
    if (tab.dirty) setConfirming(tab)
    else close(group.id, tab.id)
  }

  return (
    <div
      className={cn(
        'flex h-[35px] shrink-0 items-center border-b border-[hsl(var(--border))] bg-panel no-select',
        !focused && 'opacity-80',
      )}
    >
      <div className="flex min-w-0 flex-1 items-stretch overflow-x-auto">
        {group.tabs.map((tab) => (
          <TabButton
            key={tab.id}
            tab={tab}
            active={tab.id === group.activeId}
            onActivate={() => activate(group.id, tab.id)}
            onPin={() => pin(group.id, tab.id)}
            onClose={() => requestClose(tab)}
            onCloseOthers={() => closeOthers(group.id, tab.id)}
            onCloseAll={() => closeAll(group.id)}
          />
        ))}
      </div>

      <div className="flex shrink-0 items-center gap-0.5 px-1.5">
        {groupCount < 3 && (
          <Tooltip label="Split the editor" keys="$mod+Backslash" side="bottom">
            <IconButton
              aria-label="Split the editor"
              onClick={() => {
                focusGroup(group.id)
                split()
              }}
            >
              <Columns2 className="h-3.5 w-3.5" />
            </IconButton>
          </Tooltip>
        )}
      </div>

      <Modal
        open={confirming !== null}
        onOpenChange={(next) => {
          if (!next) setConfirming(null)
        }}
        title={confirming ? `Discard changes to ${docTitle(confirming.doc)}?` : ''}
        description="This document has edits that have not been saved."
        footer={
          <>
            <Button variant="ghost" onClick={() => setConfirming(null)}>
              Keep editing
            </Button>
            <Button
              variant="danger"
              onClick={() => {
                if (confirming) close(group.id, confirming.id)
                setConfirming(null)
              }}
            >
              Discard
            </Button>
          </>
        }
      >
        <p className="text-[12.5px] text-muted">
          Closing the tab throws the draft away. Saving it first keeps it.
        </p>
      </Modal>
    </div>
  )
}

function TabButton({
  tab,
  active,
  onActivate,
  onPin,
  onClose,
  onCloseOthers,
  onCloseAll,
}: {
  tab: Tab
  active: boolean
  onActivate: () => void
  onPin: () => void
  onClose: () => void
  onCloseOthers: () => void
  onCloseAll: () => void
}) {
  return (
    <ContextMenu
      items={
        <>
          <MenuLabel>{docTitle(tab.doc)}</MenuLabel>
          {tab.preview && <MenuItem onSelect={onPin}>Keep open</MenuItem>}
          <MenuItem onSelect={onClose} shortcut={<Kbd keys="$mod+KeyW" />}>
            Close
          </MenuItem>
          <MenuItem onSelect={onCloseOthers}>Close others</MenuItem>
          <MenuItem onSelect={onCloseAll}>Close all</MenuItem>
          <MenuSeparator />
          <MenuItem onSelect={onPin} disabled={!tab.preview}>
            Pin this tab
          </MenuItem>
        </>
      }
    >
      <div
        role="tab"
        aria-selected={active}
        tabIndex={active ? 0 : -1}
        onClick={onActivate}
        // Double-click is the reference's "I mean it" — the same gesture that
        // pins from the tree pins from the strip.
        onDoubleClick={onPin}
        onAuxClick={(event) => {
          if (event.button === 1) onClose()
        }}
        onKeyDown={(event) => {
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault()
            onActivate()
          }
        }}
        title={docTitle(tab.doc)}
        className={cn(
          'group relative flex h-full min-w-[110px] max-w-[220px] shrink-0 cursor-pointer items-center gap-1.5 border-r border-[hsl(var(--border))] pl-2.5 pr-1.5 transition-colors',
          active ? 'bg-app text-fg' : 'text-muted hover:bg-[hsl(var(--bg-hover))] hover:text-fg',
        )}
      >
        {active && (
          <span aria-hidden className="absolute inset-x-0 top-0 h-[2px] bg-[hsl(var(--accent))]" />
        )}

        <TabIcon tab={tab} />
        <span
          className={cn(
            'min-w-0 flex-1 truncate text-[12px]',
            // Italic is the preview tell. It is the one typographic signal the
            // reference uses and it reads instantly once you know it.
            tab.preview && 'italic',
          )}
        >
          {docTitle(tab.doc)}
        </span>

        <button
          type="button"
          aria-label={tab.dirty ? 'Unsaved changes — close' : 'Close'}
          onClick={(event) => {
            event.stopPropagation()
            onClose()
          }}
          className={cn(
            'flex h-4 w-4 shrink-0 items-center justify-center rounded-[var(--radius-xs)] text-subtle',
            'hover:bg-[hsl(var(--bg-hover))] hover:text-fg',
            !active && !tab.dirty && 'opacity-0 group-hover:opacity-100',
          )}
        >
          {/* The dot becomes a × on hover, so "unsaved" and "close" occupy one
              slot instead of competing for the same corner. */}
          {tab.dirty ? (
            <>
              <span className="h-2 w-2 rounded-full bg-[hsl(var(--fg-muted))] group-hover:hidden" />
              <X className="hidden h-3 w-3 group-hover:block" />
            </>
          ) : (
            <X className="h-3 w-3" />
          )}
        </button>
      </div>
    </ContextMenu>
  )
}

function TabIcon({ tab }: { tab: Tab }) {
  switch (tab.doc.kind) {
    case 'video':
      return <Film className="h-3.5 w-3.5 shrink-0 text-subtle" aria-hidden />
    case 'channel':
      return <Tv className="h-3.5 w-3.5 shrink-0 text-subtle" aria-hidden />
    case 'settings':
      return <SettingsIcon className="h-3.5 w-3.5 shrink-0 text-subtle" aria-hidden />
    default:
      return null
  }
}
