const { w, errors } = require('./dom.cjs')

/**
 * Does Workbench V2 mount, and does the shell do what it exists for?
 *
 * Not a substitute for looking at it — translucency and hairlines are not
 * assertable — but the structural claims are: the source list groups real data,
 * a single click previews and a second one replaces that preview, the panes
 * answer the keyboard, and the bottom panel spans the window rather than the
 * editor column. The value is the same as the v1 suite's: dock and panel
 * libraries throw when asked about a layout that has not happened yet, and
 * neither `tsc` nor `vite build` will ever notice.
 */

const API = 'http://127.0.0.1:8080'
const j = async (path) => (await fetch(API + path)).json()

const wait = (ms) => new Promise((r) => setTimeout(r, ms))
const text = () => w.document.body.textContent || ''
const tabs = () => [...w.document.querySelectorAll('[data-tab-id]')]
const tabIds = () => tabs().map((t) => t.getAttribute('data-tab-id'))
const rows = () => [...w.document.querySelectorAll('[data-row-id]')]

/** A real click is two events; dockview and the preview rule react to both. */
function click(el, detail = 1) {
  el.dispatchEvent(new w.MouseEvent('click', { bubbles: true, detail }))
  if (detail === 2) el.dispatchEvent(new w.MouseEvent('dblclick', { bubbles: true, detail }))
}

function chord(code, modifiers = {}) {
  const key = code
    .replace(/^Key/, '')
    .replace(/^Digit/, '')
    .toLowerCase()
  w.dispatchEvent(new w.KeyboardEvent('keydown', { code, key, bubbles: true, ...modifiers }))
}

let ok = true
function report(checks) {
  for (const [name, pass] of Object.entries(checks)) {
    console.log(`${pass ? '  ok  ' : ' FAIL '} ${name}`)
    if (!pass) ok = false
  }
}

async function main() {
  const channels = (await j('/api/channels')).channels
  const videos = (await j('/api/videos?limit=200')).videos
  if (channels.length === 0 || videos.length < 2) {
    console.log('render-v2: skipped — this server needs a channel and two videos\n')
    process.exit(0)
  }

  const byRecency = [...videos].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
  const first = byRecency[0]
  const second = byRecency[1]
  const channel = channels.find((c) => c.id === first.channelId)
  console.log(
    `seeded: ${videos.length} videos, ${channels.length} channels; ` +
      `subjects ${first.ref} and ${second.ref}`,
  )

  const { mount, useWorkbench } = require('./.tmp/render-v2.cjs')
  // A previous run's persisted state would decide what this one is testing.
  w.localStorage.clear()
  useWorkbench.setState({
    primaryVisible: true,
    secondaryVisible: false,
    bottomVisible: false,
    scope: 'videos',
    selected: null,
  })
  mount(w.document.getElementById('root'), { channels, videos })

  await wait(900)

  const groups = [...w.document.querySelectorAll('[data-panel-group-direction]')]
  const outer = groups[0]

  report({
    'the shell mounts': text().length > 0,
    'panes are resizable': groups.length >= 2,
    'the bottom panel is not indented by the sidebar':
      outer?.getAttribute('data-panel-group-direction') === 'vertical' &&
      Boolean(outer.querySelector('[data-panel-group-direction="horizontal"]')),
    'the dock is up': Boolean(w.document.querySelector('.dockview-theme-macos')),
    'nothing open shows the watermark': text().includes('Ready when you are'),
    'the source list has both scopes': text().includes('Videos') && text().includes('Channels'),
    'it groups videos under their channel': text().includes(channel.name),
    'and lists the subject': text().includes(first.title || first.ref),
    'there is a create control': Boolean(w.document.querySelector('[aria-label="Create"]')),
    'and no pane toggles in the chrome': !w.document.querySelector(
      '[aria-label^="Hide the"],[aria-label^="Show the"],[aria-label^="Toggle the"]',
    ),
  })

  // ── preview tabs ────────────────────────────────────────────────────────
  // Taken from the rendered list rather than from the API, so the assertions
  // are about what a person would click, in the order the sidebar put it in.
  const [rowA, rowB] = rows()
  if (!rowA || !rowB) {
    report({ 'the list rendered two rows to click': false })
  } else {
    const idA = rowA.getAttribute('data-row-id')
    const idB = rowB.getAttribute('data-row-id')
    click(rowA)
    await wait(400)
    // The token is the thread between the three places a document appears.
    const token = (el) => {
      const avatar = el?.querySelector('div[style*="background-color"]')
      return avatar ? `${avatar.textContent}|${avatar.getAttribute('style')}` : null
    }
    report({
      'a click opens one tab': tabIds().length === 1,
      'and that tab is the row that was clicked': tabIds()[0] === idA,
      'the tab wears the same token as its row':
        Boolean(token(tabs()[0])) && token(tabs()[0]) === token(rowA),
      'and it is a preview, in italic': Boolean(tabs()[0]?.querySelector('.italic')),
      'the editor names the document': text().includes(first.ref),
      'the watermark is gone': !text().includes('Ready when you are'),
      'the selection is marked': Boolean(w.document.querySelector('[aria-current="true"]')),
    })

    click(rowB)
    await wait(400)
    report({
      'the next click reuses the preview': tabIds().length === 1,
      'and the preview is now the second document': tabIds()[0] === idB,
    })

    click(rowB, 2)
    await wait(400)
    report({
      'double-clicking keeps it': tabs().length === 1 && !tabs()[0].querySelector('.italic'),
    })

    click(rowA)
    await wait(400)
    report({
      'so the next preview is a second tab': tabIds().length === 2,
      'and both documents are still open': [idA, idB].every((id) => tabIds().includes(id)),
      'every tab can be closed': tabs().every((t) =>
        t.querySelector('[aria-label="Close the tab"]'),
      ),
    })
  }

  // ── the keyboard, which is the only way to move the panes ────────────────
  chord('KeyJ', { metaKey: true })
  chord('Digit3', { metaKey: true })
  await wait(400)
  report({
    '⌘J opens the bottom panel': text().includes('Console'),
    '⌘3 opens the inspector': text().includes('Inspector'),
  })

  chord('Digit1', { metaKey: true })
  await wait(400)
  report({ '⌘1 hides the sidebar': !text().includes('Library') })

  chord('KeyN', { metaKey: true })
  await wait(400)
  report({
    '⌘N opens a new video': text().includes('New video'),
    'which is a document, not a sheet': tabIds().includes('new:video'),
  })

  // ── the channel scope ───────────────────────────────────────────────────
  chord('Digit1', { metaKey: true })
  useWorkbench.getState().setScope('channels')
  await wait(400)
  report({
    'the channel scope lists channels': channels.every((c) => text().includes(c.name)),
  })

  const real = errors.filter((e) => !/not wrapped in act|Warning: /.test(e))
  if (real.length) {
    console.log('\nconsole.error output:')
    real.slice(0, 6).forEach((e) => console.log('  ' + e.slice(0, 400)))
  }
  console.log(`\nhtml ${w.document.body.innerHTML.length} bytes`)
  process.exit(ok && real.length === 0 ? 0 : 1)
}

void main()
