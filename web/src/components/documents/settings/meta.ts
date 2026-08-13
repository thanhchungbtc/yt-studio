import {
  Boxes,
  Film,
  Gauge,
  Images,
  LayoutGrid,
  PenLine,
  RefreshCw,
  Server,
  ShieldCheck,
  SlidersHorizontal,
  Speech,
  Wand2,
  type LucideIcon,
} from 'lucide-react'

import type { Setting } from '@/core/types'

/** The section that owns no settings rows, only the buttons that move them. */
export const PRESETS = 'presets'

/* ---------------------------------------------------------------- sections */

export interface GroupMeta {
  title: string
  /** One line under the section heading: what the whole group is for. */
  blurb: string
  icon: LucideIcon
}

/**
 * A human name and a sentence for each server group.
 *
 * The server names a group after the task the operator is doing, and the key
 * itself is a fine identifier but a poor heading — "pools" says nothing about
 * why anyone would open it. An unknown group still renders: this table only
 * improves on the fallback, it is not a whitelist.
 */
const GROUPS: Record<string, GroupMeta> = {
  [PRESETS]: {
    title: 'Presets',
    blurb: 'Named provider line-ups. One click moves every row a preset names.',
    icon: Wand2,
  },
  pools: {
    title: 'Concurrency',
    blurb: 'How much of each kind of work may run at once, across every video.',
    icon: Gauge,
  },
  gates: {
    title: 'Approval gates',
    blurb: 'Where the pipeline stops and waits for a human.',
    icon: ShieldCheck,
  },
  providers: {
    title: 'Providers',
    blurb: 'Which backend serves each port. Everything below follows from these.',
    icon: Boxes,
  },
  writing: {
    title: 'Writing',
    blurb: 'The model that writes blueprints, scripts, prompts and metadata.',
    icon: PenLine,
  },
  narration: {
    title: 'Narration',
    blurb: 'The voice, and how a chapter is cut into pieces to synthesise it.',
    icon: Speech,
  },
  slides: {
    title: 'Slides',
    blurb: 'The image backend that draws each slide, and the size it draws at.',
    icon: Images,
  },
  thumbnail: {
    title: 'Thumbnail',
    blurb: 'The icon grid, its style and the typeface the headline is set in.',
    icon: LayoutGrid,
  },
  video: {
    title: 'Video defaults',
    blurb: 'What a new video is created with. Read once, then frozen into the row.',
    icon: Film,
  },
  retries: {
    title: 'Retries',
    blurb: 'How often a failed task is tried again, and how long it waits.',
    icon: RefreshCw,
  },
  server: {
    title: 'Server',
    blurb: 'Event batching and log verbosity. Applied without a restart.',
    icon: Server,
  },
}

export function groupMeta(name: string): GroupMeta {
  return GROUPS[name] ?? { title: humanize(name), blurb: '', icon: SlidersHorizontal }
}

/* -------------------------------------------------------------------- rows */

/** Words that are not capitalised by title-casing them. */
const ACRONYMS: Record<string, string> = {
  llm: 'LLM',
  tts: 'TTS',
  xtts: 'XTTS',
  sse: 'SSE',
  url: 'URL',
  api: 'API',
  id: 'ID',
  ui: 'UI',
  ninerouter: '9router',
  ffmpeg: 'FFmpeg',
  // Abbreviations a key can afford and a heading cannot.
  min: 'minimum',
  max: 'maximum',
}

/**
 * The first segment of a key, where dropping it is dropping the section's own
 * name. `pool.llm.limit` inside "Concurrency" is "LLM limit"; `xtts.url` is not
 * listed, because a bare "URL" under Narration would lose which server it is.
 */
const GROUP_PREFIX: Record<string, string> = {
  pools: 'pool',
  gates: 'gate',
  providers: 'provider',
  thumbnail: 'thumbnail',
  video: 'video',
}

/** Trailing words a unit chip already says, so the title stops saying them. */
const UNIT_WORDS = new Set(['ms', 'percent', 'chars'])

/**
 * The unit a numeric field carries, drawn inside the input.
 *
 * Derived from the key rather than declared on the server: a unit is a fact
 * about how the name reads, and every key that means milliseconds already ends
 * in `_ms`. An unrecognised key simply gets no chip.
 */
export function settingUnit(row: Setting): string {
  if (row.type !== 'int' && row.type !== 'float') return ''
  if (row.key.endsWith('_ms')) return 'ms'
  if (row.key.endsWith('_percent')) return '%'
  if (row.key.endsWith('_chars')) return 'chars'
  const leaf = row.key.split('.').pop() ?? ''
  if (row.type === 'int' && (leaf === 'width' || leaf === 'height' || leaf === 'size')) return 'px'
  return ''
}

/**
 * The key as a heading. The key itself stays on screen underneath — this is the
 * line that makes a settings table skimmable, not a replacement for the
 * identifier that documentation and support conversations use.
 */
export function settingTitle(row: Setting): string {
  let segments = row.key.split('.')
  const prefix = GROUP_PREFIX[row.group]
  if (prefix && segments.length > 1 && segments[0] === prefix) segments = segments.slice(1)

  const words = segments.flatMap((segment) => segment.split('_')).filter(Boolean)
  if (settingUnit(row) && UNIT_WORDS.has(words[words.length - 1] ?? '')) words.pop()
  // "Blueprint enabled … On" says it twice; the switch is the verb.
  if (row.type === 'bool' && words.length > 1 && words[words.length - 1] === 'enabled') words.pop()

  const text = words.map((word) => ACRONYMS[word] ?? word).join(' ')
  return text.charAt(0).toUpperCase() + text.slice(1)
}

/** A group or backend name as prose: `thumbnail_icon` → `Thumbnail icon`. */
export function humanize(name: string): string {
  const text = name.replace(/[._-]+/g, ' ').trim()
  return text.charAt(0).toUpperCase() + text.slice(1)
}

/**
 * The backends currently selected on some port.
 *
 * A row tagged with a backend nobody has selected still holds its value and
 * still saves — it is simply not read by anything, which is worth saying rather
 * than hiding. The provider rows are the whole of the answer, so it is computed
 * here rather than asked of the server.
 */
export function activeBackends(rows: Setting[]): Set<string> {
  const active = new Set<string>()
  for (const row of rows) {
    if (row.key.startsWith('provider.') && row.value) active.add(row.value)
  }
  return active
}

/** Whether a row's backend is idle, and so whether the row is currently read. */
export function isDormant(row: Setting, active: Set<string>): boolean {
  return row.backend !== '' && !active.has(row.backend)
}
