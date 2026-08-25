const { w, errors } = require('./dom.cjs')

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
    scored.push({
      v,
      chapters,
      score: chapters.length * 10 + (v.state === 'awaiting_approval' ? 5 : 0),
    })
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
    videos,
    channels,
    video,
    health: await j('/api/health'),
    scheduler: await j('/api/scheduler'),
    settings: (await j('/api/settings')).settings,
    recentTasks: (await j('/api/tasks?limit=300')).tasks,
    chapters: best.chapters,
    tasks: (await j('/api/videos/' + first.ref + '/tasks')).tasks,
    assets: (await j('/api/videos/' + first.ref + '/assets')).assets,
  }
  // Blank one slot so the undrawn case is exercised: clicking a dashed square
  // used to do nothing at all, because the handler was given an asset id and an
  // undrawn slot has none.
  const last = seed.chapters[seed.chapters.length - 1]
  if (last && last.slideAssetIds.length > 1) {
    last.slideAssetIds = [...last.slideAssetIds]
    last.slideAssetIds[last.slideAssetIds.length - 1] = ''
    global.BLANKED = true
  }

  console.log(
    `seeded: ${videos.videos.length} videos, ${channels.length} channels, ` +
      `${seed.chapters.length} chapters, ${seed.tasks.length} tasks, ${seed.settings.length} settings; ` +
      `subject ${first.ref} (${first.state})`,
  )
  global.SUBJECT = first
  global.GATED = seed.tasks.some((task) => task.state === 'awaiting_approval')
  const scripted = best.chapters.find((c) => (c.script || '').length > 60)
  global.SCRIPT_HEAD = scripted ? scripted.script.slice(0, 60) : ''
  // The tail of the subject's longest summary, so "shown in full" is checked
  // against whatever this server actually holds. It used to be a string copied
  // out of one video's chapter 2, which stopped existing the moment a
  // higher-scoring video appeared.
  const longest = best.chapters.reduce(
    (a, c) => ((c.summary || '').length > (a?.summary || '').length ? c : a),
    null,
  )
  global.SUMMARY_TAIL = longest && longest.summary.length > 200 ? longest.summary.slice(-60) : ''
  const { mount, useWorkbenchStore } = require('./.tmp/render.cjs')
  global.STORE = useWorkbenchStore
  const other = (scored[1] || scored[0]).v
  mount(
    w.document.getElementById('root'),
    seed,
    [
      { kind: 'channel', slug: channels[0].slug },
      { kind: 'settings' },
      { kind: 'video', ref: other.ref },
      { kind: 'video', ref: first.ref },
    ],
    'output',
  )
}
main()

const wait = (ms) => new Promise((r) => setTimeout(r, ms))

/** Reports a group of checks and folds the result into `ok`. */
let ok = true
function report(checks) {
  for (const [name, pass] of Object.entries(checks)) {
    console.log(`${pass ? '  ok  ' : ' FAIL '} ${name}`)
    if (!pass) ok = false
  }
}

async function assertAll() {
  await wait(1200)

  const text = w.document.body.textContent || ''
  const html = w.document.body.innerHTML
  const rowEls = [...w.document.querySelectorAll('[data-chapter-row]')]
  const s = global.SUBJECT || {}

  report({
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
    ...(global.SUMMARY_TAIL
      ? { 'summary shown in full, uncut': text.includes(global.SUMMARY_TAIL) }
      : { '~ summary in full (skipped: no summary long enough to truncate)': true }),
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
    'undrawn slots draw as placeholders': !global.BLANKED || html.includes('border-dashed'),
    'video document body': text.includes(s.title || '\u0000'),
    'run panel': text.includes('Pipeline') || text.includes('RUN') || text.includes('Run'),
    'bottom panel tabs': text.includes('Console') && text.includes('Output'),
    'output view (live log)': text.includes('events') && text.includes('Task and scheduler frames'),
    ...(global.GATED
      ? {
          'gate card on a gated video':
            /(blueprint|upload) gate/.test(text) &&
            text.includes('Approve') &&
            text.includes('holding here until you approve'),
        }
      : { '~ gate card (skipped: nothing is gated on this server)': true }),
    'pipeline stages named': text.includes('Blueprint') && text.includes('Narration'),
    'status bar version': text.includes('running') && text.includes('ready'),
    'panel groups': html.includes('data-panel-group'),
  })

  // ── the dedicated narration viewer ──────────────────────────────────────
  const play = w.document.querySelector('[aria-label="Play the narration"]')
  if (!play) {
    report({ '~ narration viewer (skipped: no rendered narration)': true })
  } else {
    play.click()
    // A click is not a render. Reading the DOM in the same tick asserts on the
    // frame before the one the click produced.
    await wait(300)
    const after = w.document.body.textContent || ''
    report({
      'narration viewer opens': after.includes('Narration') && after.includes('Download'),
      'it names the chapter': /#\d/.test(after),
      'it has an audio transport': Boolean(w.document.querySelector('audio')),
      'the script reads along beside it':
        after.includes('Script') &&
        Boolean(global.SCRIPT_HEAD && after.includes(global.SCRIPT_HEAD)),
      'it is one artifact, not a gallery':
        !/\d+ \/ \d+/.test(after) &&
        !w.document.querySelector('[aria-label="Next"],[aria-label="Previous"]'),
    })
    const close = w.document.querySelector('[aria-label="Close"]')
    if (close) close.click()
    await wait(150)
  }

  // ── the slide viewer, including an undrawn slot ─────────────────────────
  const slides = [...w.document.querySelectorAll('[aria-label^="Slide "]')]
  if (slides.length === 0) {
    report({ '~ slide viewer (skipped: no slide cells rendered)': true })
  } else {
    slides[0].click()
    await wait(300)
    const after = w.document.body.textContent || ''
    report({
      'slide viewer opens on a slot': /Slide \d/.test(after),
      'it shows the prompt to edit': Boolean(w.document.querySelector('[aria-label="Prompt"]')),
      'it offers Generate': after.includes('Generate'),
      'it carries no artifact navigation':
        !/\d+ \/ \d+/.test(after) &&
        !w.document.querySelector('[aria-label="Next"],[aria-label="Previous"]'),
    })
    let close = w.document.querySelector('[aria-label="Close"]')
    if (close) close.click()
    await wait(150)

    if (global.BLANKED) {
      slides[slides.length - 1].click()
      await wait(300)
      const undrawn = w.document.body.textContent || ''
      report({
        'an undrawn slot opens too': undrawn.includes('Nothing drawn here yet'),
        'and still offers its prompt': Boolean(w.document.querySelector('[aria-label="Prompt"]')),
      })
      close = w.document.querySelector('[aria-label="Close"]')
      if (close) close.click()
      await wait(150)
    }
  }

  // ── the thumbnail editor ────────────────────────────────────────────────
  global.STORE.getState().open({ kind: 'thumbnail', ref: s.ref }, { preview: false })
  await wait(700)
  const t2 = w.document.body.textContent || ''
  report({
    'thumbnail editor mounts': t2.includes('Thumbnail') && t2.includes('Use this thumbnail'),
    'its inspector renders': t2.includes('Typeface') && t2.includes('Backdrop brightness'),
    'its frame is stated': /1280|frame is/.test(t2),
  })

  const real = errors.filter((e) => !/not wrapped in act|Warning: /.test(e))
  if (real.length) {
    console.log('\nconsole.error output:')
    real.slice(0, 6).forEach((e) => console.log('  ' + e.slice(0, 400)))
  }
  console.log(`\nhtml ${w.document.body.innerHTML.length} bytes`)
  process.exit(ok && real.length === 0 ? 0 : 1)
}

void assertAll()
