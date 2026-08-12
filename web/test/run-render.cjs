const { JSDOM } = require('jsdom')
const dom = new JSDOM('<!doctype html><html class="dark"><body><div id="root"></div></body></html>', {
  url: 'http://localhost:5173/workbench',
  pretendToBeVisual: true,
})
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
global.AbortController = w.AbortController
global.AbortSignal = w.AbortSignal
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
  constructor(cb) { this.cb = cb }
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
global.ResizeObserver = RO; w.ResizeObserver = RO
class IO { observe() {} unobserve() {} disconnect() {} }
global.IntersectionObserver = IO; w.IntersectionObserver = IO
class ES { constructor() { this.readyState = 0 } addEventListener() {} removeEventListener() {} close() {} }
global.EventSource = ES; w.EventSource = ES
w.matchMedia = w.matchMedia || (() => ({ matches: false, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} }))
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
  return { width: 1440, height: 900, top: 0, left: 0, right: 1440, bottom: 900, x: 0, y: 0, toJSON() {} }
}

const errors = []
const origError = console.error
console.error = (...a) => { errors.push(a.map(String).join(' ')); origError(...a) }

const API = 'http://127.0.0.1:8080'
const j = async (path) => (await fetch(API + path)).json()

async function main() {
  const videos = await j('/api/videos?limit=200')
  const channels = (await j('/api/channels')).channels

  // Chosen by what it can actually exercise, not by position. Picking
  // videos[0] meant a new empty draft silently gutted half the assertions.
  const scored = []
  for (const v of videos.videos) {
    const chapters = (await j('/api/videos/' + v.ref + '/chapters')).chapters
    scored.push({ v, chapters, score: chapters.length * 10 + (v.state === 'awaiting_approval' ? 5 : 0) })
  }
  scored.sort((a, b) => b.score - a.score)
  const best = scored[0]
  if (!best || best.chapters.length === 0) {
    console.log('render: skipped — no video on this server has chapters\n')
    process.exit(0)
  }
  const first = best.v
  const video = await j('/api/videos/' + first.ref)
  const seed = {
    videos, channels, video,
    health: await j('/api/health'),
    scheduler: await j('/api/scheduler'),
    settings: (await j('/api/settings')).settings,
    recentTasks: (await j('/api/tasks?limit=300')).tasks,
    chapters: best.chapters,
    tasks: (await j('/api/videos/' + first.ref + '/tasks')).tasks,
    assets: (await j('/api/videos/' + first.ref + '/assets')).assets,
  }
  console.log(`seeded: ${videos.videos.length} videos, ${channels.length} channels, ` +
    `${seed.chapters.length} chapters, ${seed.tasks.length} tasks, ${seed.settings.length} settings; ` +
    `subject ${first.ref} (${first.state})`)
  global.SUBJECT = first
  const { mount, useWorkbenchStore } = require('./.tmp/render.cjs')
  global.STORE = useWorkbenchStore
  const other = (scored[1] || scored[0]).v
  mount(w.document.getElementById('root'), seed, [
    { kind: 'channel', slug: channels[0].slug },
    { kind: 'settings' },
    { kind: 'video', ref: other.ref },
    { kind: 'video', ref: first.ref },
  ], 'output')
}
main()

setTimeout(() => {
  const text = w.document.body.textContent || ''
  const html = w.document.body.innerHTML
  const rowEls = [...w.document.querySelectorAll('[data-chapter-row]')]
  const s = global.SUBJECT || {}
  const checks = {
    'shell chrome': text.includes('yt-studio') && text.includes('Explorer'),
    'explorer lists the subject': text.includes(s.ref),
    'tab strip holds four distinct tabs': (html.match(/data-tab-id=/g) || []).length >= 4,
    'active tab is distinguished': html.includes('aria-selected="true"'),
    'settings tab present': text.includes('Settings'),

    'view tabs are all one click': ['Blueprint', 'Publish'].every((v) => text.includes(v)),
    'table column heads': ['CHAPTER', 'SCRIPT', 'NARRATION', 'SLIDES', 'CLIP'].every((c) =>
      text.toUpperCase().includes(c),
    ),
    'chapter rows render (virtualized)': (html.match(/data-chapter-row/g) || []).length > 0,
    'blueprint budget shown': /~\d+w/.test(text),
    'summary shown in full, uncut': text.includes(
      'Hand-off: If you replace every single part of an object',
    ),
    'no line clamp on the summary': !html.includes('line-clamp'),
    'rows measure themselves': html.includes('data-index='),
    'every column has a resize handle': (html.match(/Resize the \w+ column/g) || []).length === 5,
    // Scoped to the row subtrees. Slicing the raw html caught the run panel's
    // tooltips and reported a problem that was never in the table.
    'no radix roots inside rows': rowEls.every(
      (el) => !el.querySelector('[data-state],[data-radix-popper-content-wrapper]'),
    ),
    'rows stay light': rowEls.every((el) => el.querySelectorAll('button').length <= 6),
    'no colour transition on rows': !/data-chapter-row[^>]*transition-colors/.test(html),
    'chapter column is no longer 1fr':
      /gridTemplateColumns|grid-template-columns/.test(html) && !/minmax\(240px,1fr\)/.test(html),
    'written words shown beside it': /data-script-words="[1-9]\d*"/.test(html),
    'narration duration shown': /\d+:\d\d/.test(text),
    'slide thumbnails rendered': html.includes('/assets/') && html.includes('alt="Slide'),
    'video document body': text.includes(s.title || '\u0000'),
    'run panel': text.includes('Pipeline') || text.includes('RUN') || text.includes('Run'),
    'bottom panel tabs': text.includes('Console') && text.includes('Output'),
    'output view (live log)': text.includes('events') && text.includes('Task and scheduler frames'),
    'gate card on a gated video':
      /(blueprint|upload) gate/.test(text) &&
      text.includes('Approve') &&
      text.includes('holding here until you approve'),
    'pipeline stages named': text.includes('Blueprint') && text.includes('Narration'),
    'status bar version': text.includes('running') && text.includes('ready'),
    'panel groups': html.includes('data-panel-group'),
  }
  let ok = true
  for (const [name, pass] of Object.entries(checks)) {
    console.log(`${pass ? '  ok  ' : ' FAIL '} ${name}`)
    if (!pass) ok = false
  }
  global.STORE.getState().open({ kind: 'thumbnail', ref: s.ref }, { preview: false })
  setTimeout(() => {
    const t2 = w.document.body.textContent || ''
    const second = {
      'thumbnail editor mounts': t2.includes('Thumbnail') && t2.includes('Use this thumbnail'),
      'its inspector renders': t2.includes('Typeface') && t2.includes('Backdrop brightness'),
      'its frame is stated': /1280|frame is/.test(t2),
    }
    for (const [name, pass] of Object.entries(second)) {
      console.log(`${pass ? '  ok  ' : ' FAIL '} ${name}`)
      if (!pass) ok = false
    }
    const real = errors.filter((e) => !/not wrapped in act|Warning: /.test(e))
    if (real.length) { console.log('\nconsole.error output:'); real.slice(0, 6).forEach((e) => console.log('  ' + e.slice(0, 400))) }
    console.log(`\nhtml ${w.document.body.innerHTML.length} bytes`)
    process.exit(ok && real.length === 0 ? 0 : 1)
  }, 700)
}, 1200)
