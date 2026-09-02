import type { DockviewApi } from 'dockview-react'
import { create } from 'zustand'

/**
 * The handle on the dock, and the one rule layered on top of it.
 *
 * Dockview's API only exists once its container has laid out, and everything
 * that opens a document — the sidebar, a menu, a keystroke — sits outside that
 * container. Parking the handle in a store rather than threading it through
 * props keeps `openDoc` a plain function anyone can call, and keeps the tab
 * layout in the one place that is authoritative about it: dockview itself.
 *
 * The rule is the preview tab. A single click opens a document in *one*
 * italic tab that the next single click reuses, so browsing the library costs
 * nothing and only what you deliberately keep accumulates. Double-clicking the
 * row, or the tab, pins it.
 */

/** Everything the editor area can show. Each variant is one panel component. */
export type Doc =
  | { kind: 'video'; ref: string }
  | { kind: 'channel'; slug: string }
  | { kind: 'new'; of: 'channel' }

/** Stable per document, so opening the same thing twice reuses one tab. */
export function docId(doc: Doc): string {
  switch (doc.kind) {
    case 'channel':
      return `channel:${doc.slug}`
    case 'new':
      return `new:${doc.of}`
    default:
      return `${doc.kind}:${doc.ref}`
  }
}

interface DockState {
  api: DockviewApi | null
  /** The one tab that is a preview, if any. */
  previewId: string | null
  /**
   * What the front tab is showing, mirrored out of dockview.
   *
   * The inspector is outside the dock and has to follow the front document, and
   * `api.activePanel` is a value rather than a subscription — reading it in a
   * render would be reading it once and then going stale. So the editor area
   * mirrors it here on every change, and the inspector subscribes like it does
   * to anything else.
   */
  activeDoc: Doc | null
  setApi: (api: DockviewApi | null) => void
  setPreviewId: (id: string | null) => void
  setActiveDoc: (doc: Doc | null) => void
}

export const useDock = create<DockState>((set) => ({
  api: null,
  previewId: null,
  activeDoc: null,
  setApi: (api) => set({ api, previewId: null }),
  setPreviewId: (previewId) => set({ previewId }),
  setActiveDoc: (activeDoc) => set({ activeDoc }),
}))

interface OpenOptions {
  /** Reuse the single preview tab rather than adding one. Default: pinned. */
  preview?: boolean
  /**
   * The channel the document belongs to, so its tab and its title strip wear
   * the same coloured token its row does. Carried in the panel's params, which
   * is what a restored layout has instead of a loaded record.
   */
  seed?: string
  initial?: string
}

/** What every editor panel is handed, and what a saved layout restores. */
export interface DocPanelParams {
  doc: Doc
  title: string
  seed?: string
  initial?: string
}

/**
 * Opens a document, or brings it forward if it is already open.
 *
 * `title` is passed rather than derived here because the caller is the one
 * holding the loaded record; the editor gets the same string as a param so a
 * restored layout still has a name for a tab it has not fetched yet.
 */
export function openDoc(doc: Doc, title: string, options: OpenOptions = {}): void {
  const { api, previewId, setPreviewId } = useDock.getState()
  if (!api) return

  const id = docId(doc)
  const existing = api.getPanel(id)
  if (existing) {
    existing.api.setActive()
    // Asking for it again without preview is asking to keep it.
    if (!options.preview && previewId === id) setPreviewId(null)
    return
  }

  // The outgoing preview is closed *after* the new one is added, so the group
  // never empties in between — an empty dock drops back to the watermark and
  // takes the split layout with it.
  const outgoing = options.preview && previewId ? api.getPanel(previewId) : undefined
  const params: DocPanelParams = { doc, title }
  if (options.seed) params.seed = options.seed
  if (options.initial) params.initial = options.initial
  api.addPanel({ id, component: doc.kind, title, params })
  outgoing?.api.close()
  setPreviewId(options.preview ? id : null)
}

/** Keeps the preview tab: what a double-click, or an edit, means. */
export function pinPreview(id: string): void {
  const { previewId, setPreviewId } = useDock.getState()
  if (previewId === id) setPreviewId(null)
}

/** Closes the active tab. ⌘W. */
export function closeActive(): void {
  useDock.getState().api?.activePanel?.api.close()
}

/**
 * Closes everything except the active tab. ⇧⌘W.
 *
 * The list is copied before anything is closed: closing a panel mutates the
 * collection being walked, and a live iteration over it silently skips every
 * other tab — which reads as the shortcut half-working rather than as a bug.
 */
export function closeOthers(): void {
  const { api } = useDock.getState()
  const active = api?.activePanel
  if (!api || !active) return
  for (const panel of [...api.panels]) {
    if (panel.id !== active.id) panel.api.close()
  }
}
