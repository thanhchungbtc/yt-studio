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
  const first = videos.videos[0]
  const video = await j('/api/videos/' + first.ref)
  const seed = {
    videos, channels, video,
    health: await j('/api/health'),
    scheduler: await j('/api/scheduler'),
    settings: (await j('/api/settings')).settings,
    recentTasks: (await j('/api/tasks?limit=300')).tasks,
    chapters: (await j('/api/videos/' + first.ref + '/chapters')).chapters,
    tasks: (await j('/api/videos/' + first.ref + '/tasks')).tasks,
    assets: (await j('/api/videos/' + first.ref + '/assets')).assets,
  }
  console.log(`seeded: ${videos.videos.length} videos, ${channels.length} channels, ` +
    `${seed.chapters.length} chapters, ${seed.tasks.length} tasks, ${seed.settings.length} settings; ` +
    `subject ${first.ref} (${first.state})`)
  global.SUBJECT = first
  const { mount } = require('./.tmp/render.cjs')
  const other = videos.videos[1] || videos.videos[0]
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
  const real = errors.filter((e) => !/not wrapped in act|Warning: /.test(e))
  if (real.length) { console.log('\nconsole.error output:'); real.slice(0, 6).forEach((e) => console.log('  ' + e.slice(0, 400))) }
  console.log(`\nhtml ${html.length} bytes`)
  process.exit(ok && real.length === 0 ? 0 : 1)
}, 1200)
