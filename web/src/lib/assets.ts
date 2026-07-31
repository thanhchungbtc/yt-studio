/**
 * Everything the UI needs to *describe* an artifact, kept separate from the
 * components that draw one. An asset row alone is a hash, a MIME type and a
 * size; these helpers rejoin it to its chapter and to the prompt or script that
 * produced it, so a preview needs no second request.
 */

import { chapterKey } from './format'
import type { Asset, Chapter } from './types'

export type MediaType = 'image' | 'video' | 'audio' | 'text' | 'binary'

/** How an artifact should be *rendered*, as opposed to what produced it. */
export function mediaTypeOf(mime: string): MediaType {
  if (mime.startsWith('image/')) return 'image'
  if (mime.startsWith('video/')) return 'video'
  if (mime.startsWith('audio/')) return 'audio'
  if (mime === 'application/json' || mime.startsWith('text/')) return 'text'
  return 'binary'
}

/** A block of provenance shown beside the artifact — the prompt, the script. */
export interface ViewerNote {
  label: string
  body: string
  /** Rendered in the monospace face; true for anything model-authored. */
  mono?: boolean
}

/**
 * One entry in the viewer. Deliberately not an `Asset`: a chapter still is
 * viewable long before the artifacts list has been fetched, and the viewer
 * should not care which of the two it was handed.
 */
export interface ViewerItem {
  id: string
  kind: string
  mime: string
  title: string
  subtitle?: string
  size?: number
  createdAt?: string
  notes?: ViewerNote[]
}

const KIND_TITLES: Record<string, string> = {
  blueprint: 'Blueprint',
  final: 'Final render',
  metadata: 'Publish metadata',
  thumbnail: 'Thumbnail',
  thumbnail_plan: 'Thumbnail plan',
  thumbnail_icon: 'Thumbnail icon',
  script: 'Script',
  prompt: 'Prompts',
  audio: 'Narration',
  clip: 'Clip',
  image: 'Still',
}

export function kindTitle(kind: string): string {
  return KIND_TITLES[kind] ?? kind
}

/** The first eight hex characters — enough to tell two artifacts apart. */
export function shortId(id: string): string {
  return id.slice(0, 8)
}

/*
  Mirrors entity.AssetKind's Ext and MIME. A viewer item can be built from a
  chapter alone — long before the artifacts list has been fetched — and those
  two tables are what let it say what the artifact is without asking.
*/

const KIND_EXT: Record<string, string> = {
  blueprint: '.json',
  metadata: '.json',
  thumbnail_plan: '.json',
  thumbnail_icon: '.png',
  script: '.txt',
  prompt: '.txt',
  audio: '.wav',
  image: '.png',
  thumbnail: '.png',
  clip: '.mp4',
  final: '.mp4',
}

const KIND_MIME: Record<string, string> = {
  blueprint: 'application/json',
  metadata: 'application/json',
  thumbnail_plan: 'application/json',
  thumbnail_icon: 'image/png',
  script: 'text/plain; charset=utf-8',
  prompt: 'text/plain; charset=utf-8',
  audio: 'audio/wav',
  image: 'image/png',
  thumbnail: 'image/png',
  clip: 'video/mp4',
  final: 'video/mp4',
}

export function kindMime(kind: string): string {
  return KIND_MIME[kind] ?? 'application/octet-stream'
}

/**
 * A filename an operator can find again on disk. The content address is kept
 * as the suffix so a downloaded file can still be traced back to the artifact
 * it came from.
 */
export function downloadName(item: ViewerItem): string {
  const slug =
    `${item.subtitle ?? ''} ${item.title}`
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '')
      .slice(0, 60) || item.kind
  return `${slug}-${shortId(item.id)}${KIND_EXT[item.kind] ?? '.bin'}`
}

function notesFor(kind: string, chapter: Chapter | undefined, slot: number): ViewerNote[] {
  if (!chapter) return []
  switch (kind) {
    case 'image': {
      const prompt = chapter.imagePrompts[slot]
      return prompt ? [{ label: `Image prompt ${slot + 1}`, body: prompt, mono: true }] : []
    }
    case 'audio':
    case 'clip':
      return chapter.script ? [{ label: 'Narration script', body: chapter.script, mono: true }] : []
    default:
      return chapter.summary ? [{ label: 'Chapter summary', body: chapter.summary }] : []
  }
}

/**
 * Everything one chapter has produced, in the order it was produced: stills in
 * slot order, then the narration, then the rendered clip. Walking the viewer
 * with the arrow keys therefore walks a chapter end to end.
 */
export function chapterStillItems(chapter: Chapter, videoRef: string): ViewerItem[] {
  const subtitle = `${chapterKey(videoRef, chapter.ordinal)} · ${chapter.title}`

  const items: ViewerItem[] = chapter.imageAssetIds.flatMap((id, slot) =>
    id
      ? [
          {
            id,
            kind: 'image',
            mime: kindMime('image'),
            title: `Still ${slot + 1}`,
            subtitle,
            notes: notesFor('image', chapter, slot),
          },
        ]
      : [],
  )

  if (chapter.audioAssetId) {
    items.push({
      id: chapter.audioAssetId,
      kind: 'audio',
      mime: kindMime('audio'),
      title: kindTitle('audio'),
      subtitle,
      notes: notesFor('audio', chapter, 0),
    })
  }
  if (chapter.clipAssetId) {
    items.push({
      id: chapter.clipAssetId,
      kind: 'clip',
      mime: kindMime('clip'),
      title: kindTitle('clip'),
      subtitle,
      notes: notesFor('clip', chapter, 0),
    })
  }

  return items
}

/**
 * Every artifact of a video, described. Sorted the way an operator reads the
 * video: what belongs to the whole thing first, then chapter by chapter.
 */
export function videoAssetItems(
  assets: Asset[],
  chapters: Chapter[],
  videoRef: string,
): ViewerItem[] {
  const byId = new Map(chapters.map((chapter) => [chapter.id, chapter]))

  const items = assets.map((asset) => {
    const chapter = asset.chapterId ? byId.get(asset.chapterId) : undefined
    const slot = chapter ? chapter.imageAssetIds.indexOf(asset.id) : -1
    const label = kindTitle(asset.kind)

    return {
      item: {
        id: asset.id,
        kind: asset.kind,
        mime: asset.mime,
        size: asset.size,
        createdAt: asset.createdAt,
        title: asset.kind === 'image' && slot >= 0 ? `${label} ${slot + 1}` : label,
        subtitle: chapter
          ? `${chapterKey(videoRef, chapter.ordinal)} · ${chapter.title}`
          : videoRef,
        notes: notesFor(asset.kind, chapter, Math.max(0, slot)),
      } satisfies ViewerItem,
      ordinal: chapter?.ordinal ?? 0,
      slot: slot < 0 ? 0 : slot,
    }
  })

  items.sort(
    (a, b) =>
      a.ordinal - b.ordinal ||
      a.item.kind.localeCompare(b.item.kind) ||
      a.slot - b.slot ||
      (a.item.createdAt ?? '').localeCompare(b.item.createdAt ?? ''),
  )

  return items.map((entry) => entry.item)
}
