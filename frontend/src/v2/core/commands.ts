import type { QueryClient } from '@tanstack/react-query'
import {
  ArrowLeftRight,
  Clapperboard,
  Copy,
  FileJson,
  Keyboard,
  PanelBottom,
  PanelLeft,
  PanelRight,
  RefreshCw,
  Search,
  Settings,
  SunMoon,
  Tv,
  X,
  ZoomIn,
  ZoomOut,
} from 'lucide-react'

import { registerCommands } from '@/kit/commands/registry'
import { getPref, setPref } from '@/prefs'

import { duplicateVideos } from '../components/duplicate-video'
import { closeActive, closeOthers, cycleTab, openDoc } from '../components/editor/dock'
import { newFromBlueprint } from '../components/new-from-blueprint'
import { newVideo } from '../components/new-video'
import { openSettings } from '../components/settings'
import { useWorkbench } from '../store/workbench'
import { qk } from './api'
import { toggleTheme } from './theme'
import type { Video } from './types'
import { checkForUpdates } from './update'

const zoom = (step: number) =>
  setPref('fontSize', Math.min(17, Math.max(11, getPref('fontSize') + step)))

/** Registers every workbench command; returns the unregister. */
export function registerWorkbenchCommands(client: QueryClient): () => void {
  const wb = () => useWorkbench.getState()
  return registerCommands(
    {
      id: 'workbench.commandPalette',
      title: 'Show All Commands',
      category: 'View',
      icon: Search,
      hidden: true,
      run: () => wb().openPalette('commands'),
    },
    {
      id: 'workbench.goToVideo',
      title: 'Go to Video…',
      category: 'Go',
      icon: Clapperboard,
      run: () => wb().openPalette('videos'),
    },
    {
      id: 'workbench.togglePrimarySidebar',
      title: 'Toggle Library',
      category: 'View',
      icon: PanelLeft,
      run: () => wb().togglePrimary(),
    },
    {
      id: 'workbench.togglePanel',
      title: 'Toggle Console',
      category: 'View',
      icon: PanelBottom,
      run: () => wb().toggleBottom(),
    },
    {
      id: 'workbench.toggleSecondarySidebar',
      title: 'Toggle Inspector',
      category: 'View',
      icon: PanelRight,
      run: () => wb().toggleSecondary(),
    },
    {
      id: 'workbench.toggleTheme',
      title: 'Toggle Light/Dark Theme',
      category: 'View',
      icon: SunMoon,
      run: toggleTheme,
    },
    {
      id: 'workbench.showVideos',
      title: 'Show Videos',
      category: 'Library',
      icon: Clapperboard,
      run: () => wb().setScope('videos'),
    },
    {
      id: 'workbench.showChannels',
      title: 'Show Channels',
      category: 'Library',
      icon: Tv,
      run: () => wb().setScope('channels'),
    },
    {
      id: 'video.new',
      title: 'New Video…',
      category: 'Video',
      icon: Clapperboard,
      run: () => newVideo(),
    },
    {
      id: 'video.newFromBlueprint',
      title: 'New Video from Blueprint…',
      category: 'Video',
      icon: FileJson,
      run: () => newFromBlueprint(),
    },
    {
      id: 'channel.new',
      title: 'New Channel…',
      category: 'Channel',
      icon: Tv,
      run: () => openDoc({ kind: 'new', of: 'channel' }, 'New Channel'),
    },
    {
      id: 'video.duplicate',
      title: 'Duplicate Selected Videos',
      category: 'Video',
      icon: Copy,
      run: () => {
        const wanted = new Set(wb().selected)
        const library = client.getQueryData<Video[]>(qk.videos) ?? []
        duplicateVideos(library.filter((video) => wanted.has(video.ref)))
      },
    },
    { id: 'workbench.closeTab', title: 'Close Tab', category: 'View', icon: X, run: closeActive },
    {
      id: 'workbench.closeOtherTabs',
      title: 'Close Other Tabs',
      category: 'View',
      run: closeOthers,
    },
    {
      id: 'workbench.nextTab',
      title: 'Next Tab',
      category: 'View',
      icon: ArrowLeftRight,
      run: () => cycleTab(1),
    },
    {
      id: 'workbench.previousTab',
      title: 'Previous Tab',
      category: 'View',
      run: () => cycleTab(-1),
    },
    {
      id: 'workbench.openSettings',
      title: 'Open Settings',
      category: 'Preferences',
      icon: Settings,
      run: () => openSettings(),
    },
    {
      id: 'workbench.openKeybindings',
      title: 'Keyboard Shortcuts',
      category: 'Preferences',
      icon: Keyboard,
      run: () => openSettings('keyboard'),
    },
    {
      id: 'console.find',
      title: 'Find in Console',
      category: 'Console',
      icon: Search,
      run: () => {
        if (!wb().bottomVisible) wb().toggleBottom()
        requestAnimationFrame(() =>
          document.querySelector<HTMLInputElement>('[data-console-search]')?.focus(),
        )
      },
    },
    { id: 'view.zoomIn', title: 'Zoom In', category: 'View', icon: ZoomIn, run: () => zoom(1) },
    { id: 'view.zoomOut', title: 'Zoom Out', category: 'View', icon: ZoomOut, run: () => zoom(-1) },
    {
      id: 'view.resetZoom',
      title: 'Reset Zoom',
      category: 'View',
      run: () => setPref('fontSize', 13),
    },
    {
      id: 'app.checkForUpdates',
      title: 'Check for Updates…',
      category: 'Application',
      icon: RefreshCw,
      run: () => void checkForUpdates(),
    },
  )
}
