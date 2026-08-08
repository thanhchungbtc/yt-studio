import { build } from 'esbuild'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

/**
 * Smoke tests for the workbench.
 *
 * Not a test framework — there is no runner in this project and this is not the
 * moment to introduce one. It is two questions the type checker cannot answer:
 * does the window actually mount, and does the tab state machine behave.
 *
 * Both matter more than they sound. The first found a crash on load that `tsc`
 * and `vite build` were both perfectly happy with: the panel library throws
 * rather than answers if you ask a panel whether it is collapsed before its
 * group has laid out.
 *
 *   node test/smoke.mjs            store only
 *   node test/smoke.mjs --render   also mount the window against a live server
 *
 * The render pass seeds the query cache from a running yt-studio on :8080, so it
 * exercises the real documents with real shapes rather than fixtures that drift.
 */

const here = dirname(fileURLToPath(import.meta.url))
const tmp = join(here, '.tmp')

const shared = {
  bundle: true,
  platform: 'node',
  format: 'cjs',
  alias: { '@': join(here, '..', 'src') },
  define: { 'process.env.NODE_ENV': '"development"' },
  logLevel: 'error',
}

await build({ ...shared, entryPoints: [join(here, 'entry-store.ts')], outfile: join(tmp, 'store.cjs') })

const suites = [['store', 'run-store.cjs']]

if (process.argv.includes('--render')) {
  const reachable = await fetch('http://127.0.0.1:8080/api/health')
    .then((r) => r.ok)
    .catch(() => false)
  if (reachable) {
    await build({
      ...shared,
      entryPoints: [join(here, 'entry-render.tsx')],
      outfile: join(tmp, 'render.cjs'),
      jsx: 'automatic',
    })
    suites.push(['render', 'run-render.cjs'])
  } else {
    console.log('render: skipped — no server on 127.0.0.1:8080\n')
  }
}

let failed = false
for (const [name, script] of suites) {
  console.log(`\n── ${name} ${'─'.repeat(Math.max(0, 60 - name.length))}`)
  const result = spawnSync(process.execPath, [join(here, script)], { stdio: 'inherit', cwd: here })
  if (result.status !== 0) failed = true
}
process.exit(failed ? 1 : 0)
