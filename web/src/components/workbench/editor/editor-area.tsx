import { Fragment, useRef } from 'react'
import { Panel, PanelGroup } from 'react-resizable-panels'

import { Handle, usePixelConstraints } from '../lib/panels'
import { useWorkbenchStore, type Group } from '../lib/store'
import { TabBar } from './tab-bar'
import { Document } from './document'
import { Welcome } from '../documents/welcome'
import { cn } from '@/core/utils'

/**
 * The editor area: one horizontal split per group.
 *
 * Groups are capped at three by the store. A fourth column of a 1440px window
 * is 300 pixels wide, which is a sidebar pretending to be an editor.
 */
export function EditorArea({ onNewVideo, onOpenPalette }: EditorAreaProps) {
  const groups = useWorkbenchStore((s) => s.groups)
  const focusedGroupId = useWorkbenchStore((s) => s.focusedGroupId)
  const container = useRef<HTMLDivElement>(null)
  const pct = usePixelConstraints(container)

  return (
    <div ref={container} className="h-full min-h-0">
      <PanelGroup direction="horizontal" autoSaveId="yt-studio.wb.editors">
        {groups.map((group, index) => (
          // A handle goes *between* groups, so it is emitted with the group it
          // precedes rather than in a second pass that would leave one dangling
          // after the last column.
          <Fragment key={group.id}>
            {index > 0 && <Handle />}
            <Panel id={group.id} order={index} minSize={pct(320, 80 / groups.length)} className="flex min-w-0 flex-col">
              <EditorGroup
                group={group}
                focused={group.id === focusedGroupId}
                onNewVideo={onNewVideo}
                onOpenPalette={onOpenPalette}
              />
            </Panel>
          </Fragment>
        ))}
      </PanelGroup>
    </div>
  )
}

interface EditorAreaProps {
  onNewVideo: (channel?: string) => void
  onOpenPalette: () => void
}

function EditorGroup({
  group,
  focused,
  onNewVideo,
  onOpenPalette,
}: EditorAreaProps & { group: Group; focused: boolean }) {
  const focusGroup = useWorkbenchStore((s) => s.focusGroup)
  const active = group.tabs.find((tab) => tab.id === group.activeId)

  return (
    <section
      aria-label="Editor group"
      // Focus follows the pointer into a group, so the run panel and every
      // keyboard command act on the half you are actually working in.
      onMouseDown={() => {
        if (!focused) focusGroup(group.id)
      }}
      className={cn(
        'flex h-full min-h-0 flex-1 flex-col overflow-hidden border-r border-[hsl(var(--border))] bg-app last:border-r-0',
      )}
    >
      <TabBar group={group} focused={focused} />
      <div className="min-h-0 flex-1 overflow-hidden">
        {active ? (
          <Document
            key={active.id}
            tab={active}
            onNewVideo={onNewVideo}
            onOpenPalette={onOpenPalette}
          />
        ) : (
          <Welcome onNewVideo={() => onNewVideo()} onOpenPalette={onOpenPalette} />
        )}
      </div>
    </section>
  )
}
