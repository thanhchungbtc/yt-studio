import { createContext, useCallback, useContext, useLayoutEffect, useRef, useState } from 'react'

/**
 * Drawing only what is near the viewport.
 *
 * `content-visibility: auto` was meant to do this, and does in Chromium — but
 * WebKit, which the desktop app runs on, lays every section out regardless.
 * Measured on an eighty-chapter video, the reader cost 49ms of layout on every
 * visit against 10ms for the five chapters actually on screen, and the table
 * of the same video was most of what a tab switch spent. So a long list decides
 * for itself: an item more than a screen and a half away draws a spacer of its
 * last height in place of its body, so the scroll does not move when it comes
 * back.
 *
 * One observer per scroller, handed down by context, rather than one per item.
 */
type Watch = (element: Element, onChange: (near: boolean) => void) => () => void

const Nearby = createContext<Watch | null>(null)

export const NearbyProvider = Nearby.Provider

/**
 * The observer for one scroller. Attach `root` as the scroller's ref and hand
 * `watch` to `NearbyProvider`.
 *
 * A callback ref rather than an object one, because the scroller may not exist
 * on the first render — a video with no chapters yet draws a notice instead —
 * and the observer has to start whenever it does.
 */
export function useNearbyRoot(): { root: (element: HTMLElement | null) => void; watch: Watch } {
  const listeners = useRef(new Map<Element, (near: boolean) => void>())
  const observer = useRef<IntersectionObserver | null>(null)
  const [element, setElement] = useState<HTMLElement | null>(null)

  useLayoutEffect(() => {
    if (!element) return
    const next = new IntersectionObserver(
      (entries) => {
        // A scroller that is not drawn at all — its mode or its tab hidden —
        // reports everything in it as gone. That is not the reader scrolling
        // away, and acting on it would tear every item down on the way out
        // and build it again on the way back. Nothing moves while it is hidden.
        const bounds = entries[0]?.rootBounds
        if (!bounds || bounds.height === 0) return
        for (const entry of entries) listeners.current.get(entry.target)?.(entry.isIntersecting)
      },
      { root: element, rootMargin: '150% 0px' },
    )
    observer.current = next
    // Items subscribe in their own layout effects, which run before this one;
    // they were queued in the map and are observed now.
    for (const target of listeners.current.keys()) next.observe(target)
    return () => {
      next.disconnect()
      observer.current = null
    }
  }, [element])

  const watch = useCallback<Watch>((target, onChange) => {
    listeners.current.set(target, onChange)
    observer.current?.observe(target)
    return () => {
      listeners.current.delete(target)
      observer.current?.unobserve(target)
    }
  }, [])

  return { root: setElement, watch }
}

/**
 * Whether this element is near its scroller's viewport. Outside a provider it
 * always is, so a list nobody wired up simply draws everything.
 *
 * `eager` items start drawn, so the first frame is not a column of spacers.
 * `onAway` runs just before an item stops being near — from the observer's
 * callback, after layout, so a measurement taken there forces nothing.
 */
export function useNear(
  element: { current: Element | null },
  eager: boolean,
  onAway?: () => void,
): boolean {
  const watch = useContext(Nearby)
  const [near, setNear] = useState(eager || !watch)
  const away = useRef(onAway)
  away.current = onAway
  useLayoutEffect(() => {
    const target = element.current
    if (!watch || !target) return
    return watch(target, (next) => {
      if (!next) away.current?.()
      setNear(next)
    })
  }, [watch, element])
  return near
}
