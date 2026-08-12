const { JSDOM } = require('jsdom')
const dom = new JSDOM('', { url: 'http://localhost/' })
global.window = dom.window; global.localStorage = dom.window.localStorage
global.document = dom.window.document
const { useWorkbenchStore: S } = require('./.tmp/store.cjs')
const get = () => S.getState()
const tabs = () => get().groups[0].tabs.map((t) => `${t.id}${t.preview ? '~' : ''}${t.dirty ? '*' : ''}`)
const g0 = () => get().groups[0].id

let pass = 0, fail = 0
const is = (name, actual, expected) => {
  const a = JSON.stringify(actual), e = JSON.stringify(expected)
  if (a === e) { pass++; console.log(`  ok   ${name}`) }
  else { fail++; console.log(` FAIL  ${name}\n         got ${a}\n         want ${e}`) }
}

// Browsing reuses the one preview slot.
get().open({ kind: 'video', ref: 'A' })
get().open({ kind: 'video', ref: 'B' })
get().open({ kind: 'video', ref: 'C' })
is('preview reuses one slot', tabs(), ['video:C~'])

// Double-click pins; the next preview then opens beside it, in the same slot.
get().open({ kind: 'video', ref: 'C' }, { preview: false })
get().open({ kind: 'video', ref: 'D' })
get().open({ kind: 'video', ref: 'E' })
is('pinned tab survives browsing', tabs(), ['video:C', 'video:E~'])

// The preview slot keeps its position rather than jumping to the end.
get().open({ kind: 'channel', slug: 'x' }, { preview: false })
get().open({ kind: 'video', ref: 'F' })
is('preview keeps its position', tabs(), ['video:C', 'video:F~', 'channel:x'])

// A dirty document pins itself and stops being replaceable.
get().setDirty('video:F', true)
is('dirt pins', tabs(), ['video:C', 'video:F*', 'channel:x'])
get().open({ kind: 'video', ref: 'G' })
is('dirty tab is not replaced', tabs(), ['video:C', 'video:F*', 'channel:x', 'video:G~'])

// Closing picks the right-hand neighbour.
get().activate(g0(), 'video:C')
get().close(g0(), 'video:C')
is('close picks the right neighbour', get().groups[0].activeId, 'video:F')

// Close-others spares dirty tabs rather than discarding them unasked.
get().closeOthers(g0(), 'channel:x')
is('close others spares dirty', tabs(), ['video:F*', 'channel:x'])

// Split copies the active tab into a second group, pinned, and caps at three.
get().activate(g0(), 'channel:x')
get().split(); get().split(); get().split()
is('split caps at three groups', get().groups.length, 3)
is('split pins the copy', get().groups[1].tabs.map((t) => [t.id, t.preview]), [['channel:x', false]])

// Emptying a group folds it away; the last one stays as the floor.
const second = get().groups[1].id
get().close(second, 'channel:x')
is('emptied group folds away', get().groups.length, 2)
while (get().groups.length > 1) get().close(get().groups[1].id, get().groups[1].tabs[0].id)
get().setDirty('video:F', false)
for (const t of [...get().groups[0].tabs]) get().close(g0(), t.id)
is('last group survives empty', get().groups.length, 1)
is('empty group has no active tab', get().groups[0].activeId, null)

// Views are per tab, so two videos can sit on different sections.
get().open({ kind: 'video', ref: 'A' }, { preview: false })
get().open({ kind: 'video', ref: 'B' }, { preview: false })
get().setView('video:A', 'artifacts')
get().setView('video:B', 'info')
is('view is per tab', get().groups[0].tabs.map((t) => t.view), ['artifacts', 'info'])

// Column widths are remembered per column, and resetting drops the override
// rather than writing a default back over it.
get().setColumnWidth('slides', 320)
get().setColumnWidth('chapter', 200)
is('column widths are stored', get().columnWidths, { slides: 320, chapter: 200 })
get().resetColumnWidth('slides')
is('reset removes the override', get().columnWidths, { chapter: 200 })

console.log(`\n${pass} passed, ${fail} failed`)
process.exit(fail ? 1 : 0)
