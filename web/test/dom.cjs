/**
 * The DOM the render suites mount into.
 *
 * jsdom is a document, not a browser: it lays nothing out, observes nothing and
 * paints nothing, and a workbench asks about all three. Everything below is the
 * minimum set of lies that lets a real component tree mount and answer
 * questions — each one added because something crashed or silently rendered
 * nothing without it.
 *
 * Shared by both suites so a fix found by one is not re-found by the other.
 */

const { JSDOM } = require('jsdom')
const dom = new JSDOM(
  '<!doctype html><html class="dark"><body><div id="root"></div></body></html>',
  {
    url: 'http://localhost:5173/workbench',
    pretendToBeVisual: true,
  },
)
const w = dom.window
global.window = w
global.document = w.document
global.navigator = w.navigator
global.HTMLElement = w.HTMLElement
global.Element = w.Element
global.Node = w.Node
global.getComputedStyle = w.getComputedStyle
global.requestAnimationFrame = w.requestAnimationFrame
global.cancelAnimationFrame = w.cancelAnimationFrame
global.localStorage = w.localStorage
global.CSS = { escape: (s) => s }
// jsdom rejects a Node AbortSignal passed to its own addEventListener.
// Take jsdom's DOM classes wholesale rather than discovering them one crash at
// a time — which is exactly how this was written, through NodeFilter, then
// HTMLInputElement, then whatever came next.
//
// Two rules. Anything jsdom defines that Node does not (HTMLInputElement,
// NodeFilter, TreeWalker) has to exist at all. And for the handful Node *also*
// defines, jsdom's must win: it rejects a foreign Event passed to its own
// dispatchEvent, which is how a Radix dialog opens.
const NODE_SHADOWED = new Set([
  'Event',
  'CustomEvent',
  'EventTarget',
  'AbortController',
  'AbortSignal',
  'MessageEvent',
  'MessageChannel',
  'MessagePort',
  'Blob',
  'File',
  'FormData',
  'Headers',
  'URL',
  'URLSearchParams',
  'DOMException',
])
for (const key of Object.getOwnPropertyNames(w)) {
  // Uppercase only: the ECMAScript built-ins share this realm, and reassigning
  // Array or Promise from the window would be a no-op at best.
  if (!/^[A-Z]/.test(key)) continue
  if (key in globalThis && !NODE_SHADOWED.has(key)) continue
  try {
    globalThis[key] = w[key]
  } catch {
    /* getter-only on the window */
  }
}
global.MutationObserver = w.MutationObserver
global.DOMRect = w.DOMRect
// jsdom implements no scrolling at all.
w.Element.prototype.scrollIntoView = function () {}

// jsdom has no canvas, so the thumbnail editor's seeding — which measures text
// with the real face to fit the headline — silently returns undefined and the
// editor opens on a skeleton. That is correct degradation, and it also means the
// whole seed/render path goes untested. A permissive 2D stub exercises it:
// data properties are real, every drawing call is a no-op, and anything that
// returns an object (gradients, patterns) returns the stub so chains work.
function context2d() {
  const target = {
    font: '',
    canvas: { width: 1280, height: 720 },
    measureText: (s) => ({
      width: String(s || '').length * 8,
      actualBoundingBoxAscent: 8,
      actualBoundingBoxDescent: 2,
    }),
  }
  const proxy = new Proxy(target, {
    get: (o, k) => (k in o ? o[k] : () => proxy),
    set: (o, k, v) => ((o[k] = v), true),
  })
  return proxy
}
w.HTMLCanvasElement.prototype.getContext = function () {
  return context2d()
}
// A no-op ResizeObserver silently disables every virtualized list: react-virtual
// learns the viewport size from here and nowhere else, so it measures zero and
// renders no rows. Report a plausible box instead.
class RO {
  constructor(cb) {
    this.cb = cb
  }
  observe(el) {
    const box = { inlineSize: 1440, blockSize: 900 }
    const entry = {
      target: el,
      contentRect: { width: 1440, height: 900, top: 0, left: 0, right: 1440, bottom: 900 },
      borderBoxSize: [box],
      contentBoxSize: [box],
      devicePixelContentBoxSize: [box],
    }
    setTimeout(() => this.cb([entry], this), 0)
  }
  unobserve() {}
  disconnect() {}
}
global.ResizeObserver = RO
w.ResizeObserver = RO
class IO {
  observe() {}
  unobserve() {}
  disconnect() {}
}
global.IntersectionObserver = IO
w.IntersectionObserver = IO
class ES {
  constructor() {
    this.readyState = 0
  }
  addEventListener() {}
  removeEventListener() {}
  close() {}
}
global.EventSource = ES
w.EventSource = ES
w.matchMedia =
  w.matchMedia ||
  (() => ({
    matches: false,
    addEventListener() {},
    removeEventListener() {},
    addListener() {},
    removeListener() {},
  }))
global.matchMedia = w.matchMedia
// The editor preloads icon art through `new Image()`; jsdom has the constructor
// but never fires load, so every image stays pending forever. Resolve them.
global.Image = class {
  constructor() {
    setTimeout(() => this.onload && this.onload(), 0)
  }
  set src(_value) {}
  get complete() {
    return true
  }
}
global.FontFace = class {
  load() {
    return Promise.resolve(this)
  }
}
w.document.fonts = w.document.fonts || { add() {}, delete() {}, ready: Promise.resolve() }
// jsdom lays nothing out, so every measurement is zero; give panels a window.
w.HTMLElement.prototype.getBoundingClientRect = function () {
  return {
    width: 1440,
    height: 900,
    top: 0,
    left: 0,
    right: 1440,
    bottom: 900,
    x: 0,
    y: 0,
    toJSON() {},
  }
}

const errors = []
const origError = console.error
console.error = (...a) => {
  errors.push(a.map(String).join(' '))
  origError(...a)
}

module.exports = { dom, w, errors }
