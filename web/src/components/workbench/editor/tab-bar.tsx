import { useQuery } from '@tanstack/react-query'
import { ChevronDown, Columns2, Film, Settings as SettingsIcon, Tv, X } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'

import { Button, IconButton } from '../ui/controls'
import { ContextMenu, Dropdown, DropdownItem, MenuItem, MenuLabel, MenuSeparator } from '../ui/menu'
import { Kbd, Modal, Ring, Tooltip } from '../ui/primitives'
import { VideoStateDot } from '../ui/status'
import { docTitle, useWorkbenchStore, type Group, type Tab } from '../lib/store'
import { api, qk } from '@/core/api'
import { cn } from '@/core/utils'

/**
 * The tab strip for one editor group.
 *
 * Three things carry the whole look, and all three are borrowed from editors
 * that got this right:
 *
 *   Tabs are sized by their content, not padded out to a uniform block. A strip
 *   of equal-width tabs reads as a table; a strip of content-width tabs reads as
 *   a set of documents.
 *
 *   The only rule between two tabs is a *short* inset hairline, and it hides
 *   next to the active tab and under the cursor. Full-height borders on every
 *   tab are what made the first version look like a spreadsheet.
 *
 *   The active tab lifts to the editor's own background and covers the strip's
 *   bottom border, so it is visibly continuous with the document beneath it
 *   rather than a highlighted row above it.
 *
 * A preview tab is drawn in italic and is the only tab a single click from the
 * explorer will replace.
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
  const strip = useRef<HTMLDivElement>(null)
  const [overflowing, setOverflowing] = useState(false)

  const requestClose = (tab: Tab) => {
    if (tab.dirty) setConfirming(tab)
    else close(group.id, tab.id)
  }

  // A strip that scrolls needs to say so, or the tabs past the edge simply do
  // not exist as far as the operator is concerned.
  useEffect(() => {
    const element = strip.current
    if (!element) return
    const measure = () => setOverflowing(element.scrollWidth > element.clientWidth + 1)
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [group.tabs.length])

  // Activating from the palette or the explorer can select a tab that is
  // scrolled out of sight.
  useEffect(() => {
    if (!group.activeId) return
    strip.current
      ?.querySelector(`[data-tab-id="${CSS.escape(group.activeId)}"]`)
      ?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
  }, [group.activeId])

  return (
    <div className="flex h-9 shrink-0 items-stretch border-b border-[hsl(var(--border))] bg-panel no-select">
      <div className="relative flex min-w-0 flex-1">
        <div
          ref={strip}
          role="tablist"
          aria-label="Open documents"
          className="flex min-w-0 flex-1 items-stretch overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
        >
          {group.tabs.map((tab, index) => (
            <TabButton
              key={tab.id}
              tab={tab}
              active={tab.id === group.activeId}
              // The separator belongs to the gap, so a tab needs to know whether
              // the one after it is the active one.
              nextIsActive={group.tabs[index + 1]?.id === group.activeId}
              groupFocused={focused}
              onActivate={() => activate(group.id, tab.id)}
              onPin={() => pin(group.id, tab.id)}
              onClose={() => requestClose(tab)}
              onCloseOthers={() => closeOthers(group.id, tab.id)}
              onCloseAll={() => closeAll(group.id)}
            />
          ))}
        </div>
        {overflowing && (
          <span
            aria-hidden
            className="pointer-events-none absolute inset-y-0 right-0 w-10 bg-gradient-to-l from-[hsl(var(--bg-panel))] to-transparent"
          />
        )}
      </div>

      <div className="flex shrink-0 items-center gap-0.5 border-l border-[hsl(var(--border))] px-1.5">
        {overflowing && (
          <Dropdown
            align="end"
            trigger={
              <IconButton aria-label="Open documents">
                <ChevronDown className="h-3.5 w-3.5" />
              </IconButton>
            }
            items={group.tabs.map((tab) => (
              <DropdownItem
                key={tab.id}
                selected={tab.id === group.activeId}
                onSelect={() => activate(group.id, tab.id)}
              >
                <span className={cn('min-w-0 flex-1 truncate', tab.preview && 'italic')}>
                  {docTitle(tab.doc)}
                </span>
                {tab.dirty && (
                  <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-[hsl(var(--fg-muted))]" />
                )}
              </DropdownItem>
            ))}
          />
        )}
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
  nextIsActive,
  groupFocused,
  onActivate,
  onPin,
  onClose,
  onCloseOthers,
  onCloseAll,
}: {
  tab: Tab
  active: boolean
  nextIsActive: boolean
  groupFocused: boolean
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
          <MenuItem onSelect={onPin} disabled={!tab.preview}>
            Keep open
          </MenuItem>
          <MenuSeparator />
          <MenuItem onSelect={onClose} shortcut={<Kbd keys="$mod+KeyW" />}>
            Close
          </MenuItem>
          <MenuItem onSelect={onCloseOthers}>Close others</MenuItem>
          <MenuItem onSelect={onCloseAll}>Close all</MenuItem>
        </>
      }
    >
      <div
        role="tab"
        data-tab-id={tab.id}
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
          'group relative flex h-full min-w-0 max-w-[210px] shrink-0 cursor-pointer items-center gap-2 pl-3 pr-1.5 transition-colors duration-75',
          active ? 'bg-app text-fg' : 'text-muted hover:bg-[hsl(var(--bg-hover))] hover:text-fg',
        )}
      >
        {/* Two rules, both belonging to the active tab: the accent above it, and
            a patch below it that covers the strip's own border so the tab and
            the document read as one surface. The accent goes quiet when the
            group is not focused — which is how a split says which half is live,
            instead of dimming the whole strip. */}
        {active && (
          <>
            <span
              aria-hidden
              className={cn(
                'absolute inset-x-0 top-0 h-[2px]',
                groupFocused ? 'bg-[hsl(var(--accent))]' : 'bg-[hsl(var(--border-strong))]',
              )}
            />
            <span aria-hidden className="absolute inset-x-0 -bottom-px h-px bg-app" />
          </>
        )}

        <TabIcon tab={tab} />

        <span className={cn('min-w-0 flex-1 truncate text-[12px]', tab.preview && 'italic')}>
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
            'flex h-5 w-5 shrink-0 items-center justify-center rounded-[var(--radius-xs)] transition-colors',
            'text-subtle hover:bg-[hsl(var(--fg)/0.1)] hover:text-fg',
            // The slot is always reserved, so a label never reflows on hover.
            !active && !tab.dirty && 'opacity-0 group-hover:opacity-100 focus-visible:opacity-100',
          )}
        >
          {tab.dirty ? (
            <>
              <span className="h-2 w-2 rounded-full bg-[hsl(var(--fg-muted))] group-hover:hidden" />
              <X className="hidden h-3.5 w-3.5 group-hover:block" />
            </>
          ) : (
            <X className="h-3.5 w-3.5" />
          )}
        </button>

        {/* Inset, short, and gone wherever it would crowd the active tab or the
            one being pointed at. */}
        {!active && !nextIsActive && (
          <span
            aria-hidden
            className="absolute inset-y-[9px] right-0 w-px bg-[hsl(var(--border))] transition-opacity group-hover:opacity-0"
          />
        )}
      </div>
    </ContextMenu>
  )
}

function TabIcon({ tab }: { tab: Tab }) {
  switch (tab.doc.kind) {
    case 'video':
      return <VideoTabIcon videoRef={tab.doc.ref} />
    case 'channel':
      return <Tv className="h-3.5 w-3.5 shrink-0 text-subtle" aria-hidden />
    case 'settings':
      return <SettingsIcon className="h-3.5 w-3.5 shrink-0 text-subtle" aria-hidden />
    default:
      return <Film className="h-3.5 w-3.5 shrink-0 text-subtle" aria-hidden />
  }
}

/**
 * A video's tab carries its progress. The list is already in the cache — the
 * explorer keeps it warm and the stream patches it — so this costs a lookup
 * rather than a request, and a tab scrolled out of the tree still says whether
 * the thing behind it is moving.
 */
function VideoTabIcon({ videoRef }: { videoRef: string }) {
  const { data } = useQuery({ queryKey: qk.videos({}), queryFn: () => api.listVideos({}) })
  const video = data?.videos.find((v) => v.ref === videoRef)

  if (!video) return <Film className="h-3.5 w-3.5 shrink-0 text-subtle" aria-hidden />
  const { succeeded, total, failed } = video.counts
  return (
    <span className="flex h-3.5 w-3.5 shrink-0 items-center justify-center">
      {total > 0 ? (
        <Ring value={succeeded} total={total} failed={failed} size={13} />
      ) : (
        <VideoStateDot state={video.state} />
      )}
    </span>
  )
}
