import {
  AppWindow,
  AudioLines,
  Brain,
  Clapperboard,
  Gauge,
  Image,
  Info,
  Keyboard,
  Paintbrush,
  PenLine,
  Plug,
  RotateCcw,
  ShieldCheck,
  SquareStack,
  type LucideIcon,
} from 'lucide-react'

export interface SectionInfo {
  id: string
  title: string
  description: string
  icon: LucideIcon
  color: string
  /** The backend settings group it shows, if any. */
  group?: string
}

export const SECTIONS: SectionInfo[] = [
  {
    id: 'appearance',
    title: 'Appearance',
    description: 'Theme, window material and the size of everything',
    icon: Paintbrush,
    color: 'var(--c-purple)',
  },
  {
    id: 'providers',
    title: 'Providers',
    description: 'Which backend runs each step of the pipeline',
    icon: Plug,
    color: 'var(--c-blue)',
    group: 'providers',
  },
  {
    id: 'models',
    title: 'Models',
    description: 'Which model writes each part of a video',
    icon: Brain,
    color: 'var(--c-indigo)',
    group: 'models',
  },
  {
    id: 'writing',
    title: 'Writing',
    description: 'The language model gateway and how blueprints are planned',
    icon: PenLine,
    color: 'var(--c-orange)',
    group: 'writing',
  },
  {
    id: 'narration',
    title: 'Narration',
    description: 'Voice, speed and the narration server',
    icon: AudioLines,
    color: 'var(--c-pink)',
    group: 'narration',
  },
  {
    id: 'slides',
    title: 'Slides',
    description: 'Image generation for slides and thumbnail icons',
    icon: Image,
    color: 'var(--c-teal)',
    group: 'slides',
  },
  {
    id: 'thumbnail',
    title: 'Thumbnail',
    description: 'How thumbnails are composed',
    icon: SquareStack,
    color: 'var(--c-amber)',
    group: 'thumbnail',
  },
  {
    id: 'video',
    title: 'Video',
    description: 'What new videos start with',
    icon: Clapperboard,
    color: 'var(--c-red)',
    group: 'video',
  },
  {
    id: 'gates',
    title: 'Approvals',
    description: 'Where the pipeline stops for your review',
    icon: ShieldCheck,
    color: 'var(--c-green)',
    group: 'gates',
  },
  {
    id: 'pools',
    title: 'Concurrency',
    description: 'How many tasks of each kind run at once',
    icon: Gauge,
    color: 'var(--c-blue)',
    group: 'pools',
  },
  {
    id: 'retries',
    title: 'Retries',
    description: 'How failed tasks are tried again',
    icon: RotateCcw,
    color: 'var(--c-gray)',
    group: 'retries',
  },
  {
    id: 'keyboard',
    title: 'Keyboard',
    description: 'Shortcuts for every action in the app',
    icon: Keyboard,
    color: 'var(--c-pink)',
  },
  {
    id: 'server',
    title: 'Advanced',
    description: 'Logging and event batching',
    icon: AppWindow,
    color: 'var(--c-gray)',
    group: 'server',
  },
  {
    id: 'about',
    title: 'About',
    description: 'Version, updates and where your data lives',
    icon: Info,
    color: 'var(--c-amber)',
  },
]

export const sectionInfo = (id: string) => SECTIONS.find((s) => s.id === id)!

export interface Query {
  words: string[]
  modified: boolean
}

export const MODIFIED_FILTER = '@modified'
const MODIFIED_TOKENS = new Set([MODIFIED_FILTER, '@changed'])

export function parseQuery(text: string): Query {
  const tokens = text.trim().toLowerCase().split(/\s+/).filter(Boolean)
  return {
    words: tokens.filter((t) => !MODIFIED_TOKENS.has(t)),
    modified: tokens.some((t) => MODIFIED_TOKENS.has(t)),
  }
}

export const isSearching = (q: Query) => q.modified || q.words.length > 0

export function matches(q: Query, ...texts: Array<string | undefined>) {
  if (!q.words.length) return true
  const haystack = texts.filter(Boolean).join(' ').toLowerCase()
  return q.words.every((word) => haystack.includes(word))
}
