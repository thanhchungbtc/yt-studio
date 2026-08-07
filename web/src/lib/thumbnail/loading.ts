/**
 * Getting the editor's ingredients into the browser: the icons and backdrop it
 * composites, and the typeface it measures with.
 *
 * Both matter for fidelity rather than for looks. The images are same-origin —
 * `/assets/{id}` and `/resources/*` — so the canvas is never tainted and
 * `toBlob` can export it. And the font has to be the renderer's own: laying out
 * a headline in a substitute face means designing something other than what
 * gets published.
 */

import { useEffect, useMemo, useState } from 'react'

import type { ImageBank } from './render'

/** The URL of an operator-supplied file: a typeface, the backdrop. */
export function resourceUrl(name: string): string {
  return `/resources/${name.split('/').map(encodeURIComponent).join('/')}`
}

/** The CSS family name a typeface file is registered under. */
export function fontFamilyOf(file: string): string {
  return `yts-${file.replace(/\.[^.]+$/, '').replace(/[^A-Za-z0-9-]/g, '-')}`
}

const loadedFonts = new Map<string, Promise<boolean>>()

/**
 * Registers a typeface from the resources directory under a stable family
 * name. Resolves false when it cannot be fetched or parsed, so the editor can
 * say so rather than silently laying out in a fallback face.
 */
export function loadFont(file: string): Promise<boolean> {
  const family = fontFamilyOf(file)
  const existing = loadedFonts.get(family)
  if (existing) return existing

  const pending = (async () => {
    try {
      const face = new FontFace(family, `url("${resourceUrl(`fonts/${file}`)}")`)
      await face.load()
      document.fonts.add(face)
      return true
    } catch {
      return false
    }
  })()
  loadedFonts.set(family, pending)
  return pending
}

/** Loads a typeface and reports whether it is ready, so the canvas can wait for
 *  it rather than drawing one frame in the fallback face. */
export function useFont(file: string): { family: string; ready: boolean; failed: boolean } {
  const family = fontFamilyOf(file)
  const [state, setState] = useState<'loading' | 'ready' | 'failed'>('loading')

  useEffect(() => {
    let live = true
    setState('loading')
    void loadFont(file).then((ok) => {
      if (live) setState(ok ? 'ready' : 'failed')
    })
    return () => {
      live = false
    }
  }, [file])

  return { family, ready: state === 'ready', failed: state === 'failed' }
}

const imageCache: ImageBank = new Map()

function loadImage(url: string): Promise<void> {
  return new Promise((resolve) => {
    const existing = imageCache.get(url)
    if (existing?.complete) {
      resolve()
      return
    }
    const img = new Image()
    // Same-origin already, but stated so a future CDN for assets cannot taint
    // the canvas and break the export without anyone noticing why.
    img.crossOrigin = 'anonymous'
    img.onload = () => resolve()
    img.onerror = () => resolve()
    img.src = url
    imageCache.set(url, img)
  })
}

/**
 * Loads every URL and re-renders as they land.
 *
 * `generation` is what the canvas depends on: the bank is a mutable Map, so
 * only a changing number tells React that a redraw is due.
 */
export function useImages(urls: string[]): {
  images: ImageBank
  generation: number
  ready: boolean
} {
  const key = useMemo(() => urls.join('\n'), [urls])
  const [generation, setGeneration] = useState(0)
  const [ready, setReady] = useState(false)

  useEffect(() => {
    let live = true
    const list = key === '' ? [] : key.split('\n')
    setReady(list.length === 0)
    let settled = 0
    for (const url of list) {
      void loadImage(url).then(() => {
        if (!live) return
        settled++
        setGeneration((n) => n + 1)
        if (settled === list.length) setReady(true)
      })
    }
    return () => {
      live = false
    }
  }, [key])

  return { images: imageCache, generation, ready }
}
