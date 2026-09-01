import {
  AudioLines,
  Clapperboard,
  Gauge,
  Image,
  PenLine,
  Plug,
  RotateCcw,
  Server,
  ShieldCheck,
  SquareStack,
  type LucideIcon,
} from 'lucide-react'

/**
 * What the settings screen calls things.
 *
 * A key is an identifier and reads like one. `labelFor` strips the dots off and
 * capitalises, which turns `upload.sample_megabytes_per_second` into "Sample
 * megabytes per second" — accurate, and nothing a person would ever say.
 *
 * So the labels are written out. Forty-eight short strings is dull to maintain
 * and it is the difference between a settings pane and a dump of the schema;
 * the derivation stays as the fallback, so a key added on the server appears
 * with an ugly name rather than not appearing at all.
 */

export const GROUPS: Record<string, { title: string; icon: LucideIcon }> = {
  providers: { title: 'Providers', icon: Plug },
  pools: { title: 'Concurrency', icon: Gauge },
  writing: { title: 'Writing', icon: PenLine },
  narration: { title: 'Narration', icon: AudioLines },
  slides: { title: 'Slides', icon: Image },
  thumbnail: { title: 'Thumbnail', icon: SquareStack },
  video: { title: 'Video', icon: Clapperboard },
  gates: { title: 'Approvals', icon: ShieldCheck },
  retries: { title: 'Retries', icon: RotateCcw },
  server: { title: 'Server', icon: Server },
}

const LABELS: Record<string, string> = {
  'pool.llm.limit': 'Language model',
  'pool.tts.limit': 'Narration',
  'pool.image.limit': 'Images',
  'pool.compose.limit': 'Composition',
  'pool.cache.limit': 'Cache',
  'pool.upload.limit': 'Uploads',

  'gate.blueprint.enabled': 'Approve the blueprint',
  'gate.upload.enabled': 'Approve the upload',

  'provider.llm': 'Language model',
  'provider.tts': 'Narration',
  'provider.slide': 'Slides',
  'provider.composer': 'Composition',
  'provider.thumbnail': 'Thumbnail',
  'provider.thumbnail_icon': 'Thumbnail icons',
  'provider.uploader': 'Publishing',
  'upload.dry_run': 'Dry run',
  'upload.sample_megabytes_per_second': 'Sample upload speed',

  'ninerouter.url': 'Gateway',
  'ninerouter.key': 'API key',
  'ninerouter.model': 'Model',
  'blueprint.chapter_tolerance_percent': 'Chapter tolerance',

  'xtts.url': 'Server',
  'xtts.voice': 'Voice',
  'xtts.language': 'Language',
  'xtts.speed': 'Speed',
  'xtts.chunk.min_chars': 'Smallest chunk',
  'xtts.chunk.silence_ms': 'Silence between chunks',
  'kokoro.url': 'Server',
  'kokoro.key': 'API key',
  'kokoro.model': 'Model',
  'kokoro.voice': 'Voice',
  'kokoro.speed': 'Speed',

  'runware.key': 'API key',
  'runware.model': 'Model',
  'runware.width': 'Width',
  'runware.height': 'Height',

  'thumbnail.icon.style': 'Icon style',
  'thumbnail.icon.size': 'Icon size',
  'thumbnail.font': 'Typeface',
  'thumbnail.grid.rows': 'Grid rows',

  'video.default_chapter_count': 'Chapters',
  'video.default_slides_per_chapter': 'Slides per chapter',
  'video.default_thumbnail_cells': 'Thumbnail tiles',

  'task.max_attempts': 'Attempts',
  'task.retry_base_ms': 'First retry after',
  'task.retry_max_ms': 'Longest retry wait',

  'sse.coalesce_ms': 'Event batching',
  'log.level': 'Log level',
}

/** The written label, or the key with its dots knocked out. */
export function labelFor(key: string): string {
  const written = LABELS[key]
  if (written) return written
  const tail = key.split('.').slice(1).join(' ').replace(/[._]/g, ' ')
  const words = (tail || key).trim()
  return words.charAt(0).toUpperCase() + words.slice(1)
}

/**
 * Which backend a group's hidden rows belong to, for the note under the card.
 *
 * Read off the rows themselves rather than tabulated here: the answer is
 * already in the data, and a second copy would be one to keep in step.
 */
export function backendsOf(keys: string[]): string {
  const names = [...new Set(keys)].sort()
  if (names.length === 0) return ''
  if (names.length === 1) return names[0] ?? ''
  return `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}`
}
