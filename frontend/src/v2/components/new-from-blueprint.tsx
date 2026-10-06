import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useDeferredValue, useId, useMemo, useRef, useState } from 'react'
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels'
import { create } from 'zustand'

import { api, qk } from '../core/api'
import { count } from '../core/format'
import { openDoc } from './editor/dock'
import { Button } from './ui/button'
import { Dialog } from './ui/dialog'
import { Input, Select } from './ui/field'

/**
 * The second way to make a video: paste an outline you already wrote.
 *
 * The first way — `new-video.tsx` — asks how big a video should be and hands
 * the answer to the model, which writes the outline. This screen asks almost
 * nothing, because the outline is already in your hand and nearly every field
 * the other dialog collects is inside it: the title, the summary, how many
 * chapters there are and how long each is meant to run. What is left to ask is
 * the channel and the two counts the blueprint has no opinion about.
 *
 * So it is not a form. It is an editor and a proof: the JSON on the left, what
 * it comes to on the right, kept in step on every keystroke. A blueprint is a
 * wall of text nobody reads back, and the whole point of the right-hand pane is
 * that you can see your video before you spend an hour generating it — the
 * order of the chapters, where the words went, and the ones that are thin.
 *
 * Deliberately self-contained. It imports the generic macOS controls and
 * nothing video-specific — not `BriefFields`, not `requestFrom`, not
 * `briefReady` — so the three dialogs that share those cannot be broken from
 * here. The bounds below are copies for the same reason.
 */

/** The server's caps, from `CreateVideoInput` in `delivery/http/videos.go`. */
const TITLE_MAX = 200
const TOPIC_MAX = 5000
const SLIDES = { min: 1, max: 20 }
const CELLS = { min: 1, max: 24 }
/** Also the server's, and the one bound a paste can break on its own. */
const MAX_CHAPTERS = 500
/** The server refuses a longer target; a paste implying one is clamped to it. */
const MAX_DURATION_MINUTES = 720

/**
 * Mirrors `entity.DefaultWordsPerMinute`. Only ever used to turn a word count
 * into the runtime printed beside it, so drift costs a readout that reads a
 * little long. Nothing is decided from it.
 */
const NARRATION_WPM = 130

/** Under this, a chapter is a paragraph read aloud rather than a chapter. */
const THIN_CHAPTER_WORDS = 80

/** The editor's metrics, shared by the text and the gutter beside it so the
 *  numbers cannot drift a pixel off the lines they count. */
const LINE_HEIGHT = 18
const EDITOR_TEXT = 'font-mono text-[11px] leading-[18px]'

const PLACEHOLDER = `{
  "title": "Why Voyager Went Silent",
  "summary": "In 1977 two probes left Earth…",
  "chapters": [
    {
      "order": 1,
      "title": "The Silence Begins",
      "core_concept": "A routine downlink that never arrived…",
      "key_points": ["Open: cold — the last clean signal"],
      "tone": "curious",
      "role": "hook",
      "estimated_words": 900,
      "script": "optional — paste the narration and this chapter skips the model"
    }
  ]
}`

interface NewFromBlueprintState {
  open: boolean
  /** The channel the request came from, if it came from one. */
  channel: string | undefined
  show: (channel?: string) => void
  hide: () => void
}

const useNewFromBlueprint = create<NewFromBlueprintState>((set) => ({
  open: false,
  channel: undefined,
  show: (channel) => set({ open: true, channel }),
  hide: () => set({ open: false }),
}))

/** Opens the dialog. Bound to ⌥⌘N and to the sidebar's create menu. */
export function newFromBlueprint(channel?: string): void {
  useNewFromBlueprint.getState().show(channel)
}

/* ── reading the paste ─────────────────────────────────────────────────── */

/** One chapter, reduced to what the right-hand pane shows. */
interface Outlined {
  order: number
  title: string
  concept: string
  role: string
  tone: string
  /** The budget the outline assigned this chapter. Zero is unset. */
  words: number
  /** Narration written outside the app, or '' for a chapter the model will write. */
  script: string
  /** How long that narration actually runs, against the budget above. */
  scriptWords: number
}

/**
 * A parsed blueprint.
 *
 * `blockers` and `warnings` are kept apart because they stop different things.
 * A blocker is a reason the server would refuse the video, so the button goes
 * dead; a warning is a reason you might not want the video you are about to
 * get, which is yours to overrule. Both are worth saying here rather than an
 * hour into generation, which is the only other place they surface.
 */
interface Outline {
  title: string
  summary: string
  chapters: Outlined[]
  /** What the outline budgeted, summed. */
  words: number
  /** What the pasted narration actually comes to, summed. */
  scriptWords: number
  /** How many chapters arrived with narration; the rest fall to the model. */
  scripted: number
  blockers: string[]
  warnings: string[]
  /** The document as pasted, forwarded to the server untouched. */
  raw: unknown
}

type Parsed = { ok: true; outline: Outline } | { ok: false; message: string; offset: number | null }

/** Reads a `string` field off an unknown object, trimmed, or `''`. */
function str(source: unknown, key: string): string {
  const value = (source as Record<string, unknown> | null)?.[key]
  return typeof value === 'string' ? value.trim() : ''
}

/** Reads a positive whole `number` field, or `0` for anything else. */
function num(source: unknown, key: string): number {
  const value = (source as Record<string, unknown> | null)?.[key]
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : 0
}

/**
 * `JSON.parse`'s complaint, split into a sentence and a place.
 *
 * V8 appends `at position N` and, lately, `(line L column C)` as well. Both
 * tails are stripped and the position kept, because the caret is the useful
 * half — the dialog puts it where the error is rather than printing coordinates
 * and leaving you to count.
 */
function syntaxError(error: Error): { message: string; offset: number | null } {
  const at = /\s*(?:in JSON )?at position (\d+)(?:\s*\(line \d+ column \d+\))?/.exec(error.message)
  if (!at) return { message: error.message, offset: null }
  return { message: error.message.slice(0, at.index), offset: Number(at[1]) }
}

/** Where a character offset falls, as an editor counts. */
function locate(text: string, offset: number): { line: number; column: number } {
  const before = text.slice(0, offset)
  const line = before.split('\n').length
  return { line, column: offset - (before.lastIndexOf('\n') + 1) + 1 }
}

/**
 * Reads a pasted blueprint against the shape the pipeline writes and stores —
 * `blueprintDoc` in `adapters/provider/ninerouter/blueprint.go`. The keys are
 * that document's, snake_case and all: `order`, `core_concept`,
 * `estimated_words`.
 *
 * Structural faults return early, one at a time: the text is one field and the
 * fix is one edit, so a list of thirty complaints about a document that failed
 * to parse at character four would be thirty guesses. What survives that is
 * reported in full, because by then every line has been read.
 *
 * Only two things are actually required of a chapter — that it is an object and
 * that it has a title — because those are what `entity.NewChapter` refuses
 * without. Everything else the model emits is optional there, so demanding it
 * here would make this screen stricter than the pipeline it feeds.
 */
function parseOutline(text: string): Parsed {
  let raw: unknown
  try {
    raw = JSON.parse(text)
  } catch (error) {
    const { message, offset } = syntaxError(error as Error)
    return { ok: false, message, offset }
  }

  const rows = (raw as { chapters?: unknown } | null)?.chapters
  if (!Array.isArray(rows)) {
    return { ok: false, message: 'No "chapters" array in the blueprint', offset: null }
  }
  if (rows.length === 0) {
    return { ok: false, message: 'The blueprint has no chapters', offset: null }
  }
  if (rows.length > MAX_CHAPTERS) {
    return {
      ok: false,
      message: `${count(rows.length)} chapters, and the limit is ${count(MAX_CHAPTERS)}`,
      offset: null,
    }
  }

  const chapters: Outlined[] = []
  for (const [index, row] of rows.entries()) {
    const title = str(row, 'title')
    if (title === '') {
      return { ok: false, message: `Chapter ${index + 1} has no title`, offset: null }
    }
    const script = str(row, 'script')
    chapters.push({
      // Displayed as written so a gap is visible; the pipeline renumbers from
      // position on the way in, which is what the warning below says.
      order: num(row, 'order') || index + 1,
      title,
      // The first paragraph only. `core_concept` is a paragraph of direction and
      // the row has one line for it, so the rest belongs in the chapter editor.
      concept: str(row, 'core_concept').split('\n')[0] ?? '',
      role: str(row, 'role'),
      tone: str(row, 'tone'),
      words: num(row, 'estimated_words'),
      // Optional, and absent for every ordinary blueprint. The backend reads it
      // from this same position in the array, so what is shown beside a chapter
      // here is the narration that chapter will actually be given.
      script,
      scriptWords: script === '' ? 0 : script.split(/\s+/).length,
    })
  }

  const title = str(raw, 'title')
  const summary = str(raw, 'summary')
  const words = chapters.reduce((total, c) => total + c.words, 0)
  const scriptWords = chapters.reduce((total, c) => total + c.scriptWords, 0)
  const scripted = chapters.filter((c) => c.script !== '').length

  return {
    ok: true,
    outline: {
      title,
      summary,
      chapters,
      words,
      scriptWords,
      scripted,
      blockers: blockersOf(title, summary),
      warnings: warningsOf(chapters),
      raw,
    },
  }
}

/** What the server would refuse the video for. */
function blockersOf(title: string, summary: string): string[] {
  const found: string[] = []
  if (title === '') {
    found.push('The blueprint has no "title" — the video needs one to be created')
  } else if (title.length > TITLE_MAX) {
    found.push(`The title is ${count(title.length)} characters, over the ${count(TITLE_MAX)} limit`)
  }
  if (summary.length > TOPIC_MAX) {
    found.push(
      `The summary is ${count(summary.length)} characters, over the ${count(TOPIC_MAX)} limit`,
    )
  }
  return found
}

/**
 * What you might not want, said before it costs an hour.
 *
 * Counted rather than listed one per chapter: a blueprint that forgot word
 * budgets forgot all of them, and thirty identical lines would bury the one
 * warning that was about a single chapter.
 */
function warningsOf(chapters: Outlined[]): string[] {
  const found: string[] = []

  const unbudgeted = chapters.filter((c) => c.words === 0)
  if (unbudgeted.length > 0) {
    found.push(
      unbudgeted.length === chapters.length
        ? 'No chapter has a word budget — lengths will fall back to the default'
        : `${plural(unbudgeted.length, 'chapter')} without a word budget: ${list(unbudgeted)}`,
    )
  }

  const thin = chapters.filter((c) => c.words > 0 && c.words < THIN_CHAPTER_WORDS)
  if (thin.length > 0) {
    found.push(`${plural(thin.length, 'chapter')} under ${THIN_CHAPTER_WORDS} words: ${list(thin)}`)
  }

  // Only when the document is partly scripted. All or nothing is a decision, not
  // an oversight; a gap in the middle is usually the second.
  const unscripted = chapters.filter((c) => c.script === '')
  if (unscripted.length > 0 && unscripted.length < chapters.length) {
    const scripted = chapters.length - unscripted.length
    const which = unscripted.length === 1 ? 'chapter' : 'chapters'
    found.push(
      `${count(scripted)} of ${plural(chapters.length, 'chapter')} ${scripted === 1 ? 'carries' : 'carry'} a script — the model will write ${which} ${list(unscripted)}`,
    )
  }

  const seen = new Map<string, number>()
  const repeated = new Set<string>()
  for (const c of chapters) {
    const key = c.title.toLocaleLowerCase()
    if (seen.has(key)) repeated.add(c.title)
    seen.set(key, (seen.get(key) ?? 0) + 1)
  }
  if (repeated.size > 0) {
    found.push(`Repeated title: ${[...repeated].slice(0, 2).join(', ')}`)
  }

  // The pipeline renumbers from position rather than trusting this field, so a
  // gap is not a fault — but a blueprint assembled from pieces is usually one
  // the author still thinks is numbered the way they left it.
  if (chapters.some((c, i) => c.order !== i + 1)) {
    found.push(`"order" is not 1…${chapters.length} — the chapters will be renumbered`)
  }

  return found
}

function plural(n: number, noun: string): string {
  return `${count(n)} ${noun}${n === 1 ? '' : 's'}`
}

/** The first few offenders by ordinal, so a warning points somewhere. */
function list(chapters: Outlined[]): string {
  const shown = chapters.slice(0, 4).map((c) => c.order)
  return shown.join(', ') + (chapters.length > shown.length ? '…' : '')
}

/** A rough length, as anyone says it out loud: `3h`, `2h 53m`, `45m`. */
function coarse(minutes: number): string {
  const whole = Math.round(minutes)
  if (whole < 60) return `${whole}m`
  const h = Math.floor(whole / 60)
  const m = whole % 60
  return m === 0 ? `${h}h` : `${h}h ${m}m`
}

/** Whether a typed figure is a whole number the server will accept. */
function within(value: string, limit: { min: number; max: number }): boolean {
  const n = Number(value)
  return Number.isInteger(n) && n >= limit.min && n <= limit.max
}

/* ── the dialog ────────────────────────────────────────────────────────── */

export function NewFromBlueprintDialog() {
  const formId = useId()
  const open = useNewFromBlueprint((s) => s.open)
  const from = useNewFromBlueprint((s) => s.channel)
  const hide = useNewFromBlueprint((s) => s.hide)

  const client = useQueryClient()
  const channels = useQuery({ queryKey: qk.channels, queryFn: api.listChannels, enabled: open })

  const [channel, setChannel] = useState('')
  const [json, setJson] = useState('')
  const [slides, setSlides] = useState('2')
  const [cells, setCells] = useState('12')

  const editor = useRef<HTMLTextAreaElement>(null)

  // Opening from a channel row should follow the operator there. Read during
  // render rather than in an effect: an effect would paint one frame with the
  // wrong channel selected, and this is derived state, not a subscription.
  const selected = channel || from || channels.data?.[0]?.slug || ''

  const parsing = useDeferredValue(json)
  const parsed = useMemo(() => (parsing.trim() === '' ? null : parseOutline(parsing)), [parsing])

  /**
   * The last outline that parsed, which is what the right-hand pane draws.
   *
   * Live parsing has one fatal flaw without this: deleting a single brace makes
   * the document invalid, and a pane that rendered only valid documents would
   * blank out every few keystrokes — destroying the thing you were reading in
   * order to tell you about a comma. So the preview holds the last good reading
   * and dims instead, and the error goes under the editor it belongs to.
   *
   * A ref written during render rather than an effect, because an effect would
   * leave the preview one keystroke behind the text that produced it, which is
   * the one thing a live preview may not be. The write is a pure function of
   * the parsed text, so a double render produces the same value.
   */
  const held = useRef<Outline | null>(null)
  if (parsed?.ok) held.current = parsed.outline
  if (parsed === null) held.current = null
  const outline = held.current
  const stale = outline !== null && parsed !== null && !parsed.ok

  const chapters = outline?.chapters.length ?? 0
  const words = outline?.words ?? 0
  // Advisory here: the per-chapter budgets in the paste are what the chapter
  // rows carry. So clamping to the server's ceiling costs nothing, where
  // refusing the paste over it would cost the whole blueprint.
  const minutes = words > 0 ? Math.min(MAX_DURATION_MINUTES, Math.round(words / NARRATION_WPM)) : 0
  // Mirrors scheduler.NodeCountFor: seven video-level tasks, four per chapter,
  // one per slide, and one icon per thumbnail tile.
  const tasks = 7 + 4 * chapters + chapters * (Number(slides) || 0) + (Number(cells) || 0)

  const submit = useMutation({
    mutationFn: () =>
      api.createVideo({
        channel: selected,
        title: outline?.title ?? '',
        topic: outline?.summary ?? '',
        chapterCount: chapters,
        slidesPerChapter: Number(slides),
        thumbnailCells: Number(cells),
        targetDurationMinutes: minutes,
        blueprint: outline?.raw,
        // Not a choice on this screen. The pasted outline is only consumed by
        // the run it is handed to, so a draft made here would be a video that
        // had forgotten what it was made from.
        start: true,
      }),
    onSuccess: (video) => {
      void client.invalidateQueries({ queryKey: qk.videos })
      const owner = (channels.data ?? []).find((c) => c.id === video.channelId)
      hide()
      // The paste belongs to the video just made. The two counts stay: they are
      // how this operator shapes videos, and clearing them would make every
      // second one a form to fill in again.
      setJson('')
      openDoc({ kind: 'video', ref: video.ref }, video.title || 'Untitled', {
        seed: owner?.slug,
        initial: owner?.name,
      })
    },
  })

  const ready =
    // Never submit a reading of text that has since changed.
    parsing === json &&
    outline !== null &&
    !stale &&
    outline.blockers.length === 0 &&
    within(slides, SLIDES) &&
    within(cells, CELLS) &&
    !submit.isPending

  /**
   * Re-indents the document.
   *
   * Also run on the first paste into an empty editor, because a blueprint
   * arrives minified as often as not, and one 40,000-character line makes both
   * the gutter and the caret useless. Only then: reformatting under someone
   * mid-edit would move the text out from under their cursor.
   */
  const reformat = (text: string): string => {
    try {
      return JSON.stringify(JSON.parse(text), null, 2)
    } catch {
      return text
    }
  }

  const jumpTo = (offset: number) => {
    const node = editor.current
    if (!node) return
    node.focus()
    node.setSelectionRange(offset, offset)
    node.scrollTop = Math.max(0, (locate(json, offset).line - 5) * LINE_HEIGHT)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) hide()
      }}
      width={980}
      height="min(760px, 86vh)"
    >
      <Dialog.Header
        title="New video from blueprint"
        description="Paste an outline you prepared. Edit it on the left, check it on the right."
      />
      <Dialog.Body bare>
        <form
          id={formId}
          className="flex min-w-0 flex-1 flex-col"
          onSubmit={(e) => e.preventDefault()}
        >
          <PanelGroup direction="horizontal" className="min-h-0 flex-1">
            <Panel id="json" order={1} defaultSize={46} minSize={26}>
              <JsonPane
                editor={editor}
                value={json}
                onChange={(next, pasted) => setJson(pasted && json === '' ? reformat(next) : next)}
                onFormat={() => setJson(reformat(json))}
                canFormat={parsed?.ok === true}
                error={parsed && !parsed.ok ? parsed : null}
                onJump={jumpTo}
              />
            </Panel>

            <PanelResizeHandle className="seam-v relative z-20 outline-none">
              <div className="absolute inset-y-0 -right-1 -left-1 cursor-col-resize" />
            </PanelResizeHandle>

            <Panel id="outline" order={2} defaultSize={54} minSize={32}>
              <OutlinePane
                outline={outline}
                stale={stale}
                minutes={minutes}
                tasks={tasks}
                words={words}
              />
            </Panel>
          </PanelGroup>

          {/* The two things the blueprint has no opinion about, and the channel
              it belongs to. Spanning both panes because they belong to the
              video rather than to either side of it. */}
          <div className="hairline-t flex shrink-0 items-center gap-5 px-5 py-2.5">
            <label className="flex min-w-0 flex-1 items-center gap-2">
              <span className="shrink-0 text-[11px] text-secondary">Channel</span>
              <Select
                value={selected}
                onChange={(e) => setChannel(e.target.value)}
                className="max-w-[220px]"
              >
                {(channels.data ?? []).map((c) => (
                  <option key={c.id} value={c.slug}>
                    {c.name}
                  </option>
                ))}
              </Select>
            </label>
            <Stepper
              label="Slides"
              unit="per chapter"
              value={slides}
              onChange={setSlides}
              {...SLIDES}
            />
            <Stepper label="Thumbnail" unit="tiles" value={cells} onChange={setCells} {...CELLS} />
          </div>
        </form>
      </Dialog.Body>
      <Dialog.Footer>
        <span className="mr-auto truncate text-[11px] text-tertiary">
          {submit.error ? (
            <span style={{ color: 'var(--failed)' }}>{(submit.error as Error).message}</span>
          ) : outline ? (
            <>
              About <span className="font-medium tabular-nums">{count(tasks)}</span> tasks ·{' '}
              {/*
                What pressing the button costs, in the only currency that is not
                obvious from the numbers beside it. A pasted outline skips the
                blueprint gate, so nothing pauses to be read: the question worth
                answering here is how much of this video the model still has to
                write.
              */}
              {outline.scripted === outline.chapters.length
                ? 'starts immediately, no model calls before narration'
                : outline.scripted === 0
                  ? 'starts immediately and writes every script'
                  : `starts immediately · ${count(outline.chapters.length - outline.scripted)} of ${count(outline.chapters.length)} scripts still to write`}
            </>
          ) : null}
        </span>
        <Button className="h-[26px] px-3.5" onClick={hide}>
          Cancel
        </Button>
        <Button
          primary
          type="button"
          className="h-[26px] px-3.5"
          disabled={!ready}
          onClick={() => submit.mutate()}
        >
          {submit.isPending ? 'Creating…' : 'Create and Start'}
        </Button>
      </Dialog.Footer>
    </Dialog>
  )
}

/* ── the editor ────────────────────────────────────────────────────────── */

/**
 * The document, with a gutter and whatever is wrong with it.
 *
 * The text does not wrap. That is the one setting the gutter depends on: a
 * wrapped line occupies two rows and would put every number below it off by
 * one, so long lines scroll sideways the way they do in an editor.
 */
function JsonPane({
  editor,
  value,
  onChange,
  onFormat,
  canFormat,
  error,
  onJump,
}: {
  editor: React.RefObject<HTMLTextAreaElement | null>
  value: string
  onChange: (value: string, pasted: boolean) => void
  onFormat: () => void
  canFormat: boolean
  error: { message: string; offset: number | null } | null
  onJump: (offset: number) => void
}) {
  const gutter = useRef<HTMLPreElement>(null)
  // One text node rather than a row of elements: a five-hundred-chapter
  // blueprint is six thousand lines, and six thousand divs to letter a margin
  // would be the only thing on this screen you could feel.
  const numbers = useMemo(() => {
    const lines = value === '' ? 1 : value.split('\n').length
    return Array.from({ length: lines }, (_, i) => i + 1).join('\n')
  }, [value])

  const place = error?.offset != null ? locate(value, error.offset) : null

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="hairline-b flex h-[30px] shrink-0 items-center gap-2 px-3">
        <span className="text-[11px] font-medium text-secondary">Blueprint JSON</span>
        <button
          type="button"
          disabled={!canFormat}
          onClick={onFormat}
          className="ml-auto rounded-[5px] px-1.5 py-0.5 text-[11px] text-secondary transition-colors hover:bg-[var(--hover)] hover:text-primary disabled:pointer-events-none disabled:opacity-35"
        >
          Format
        </button>
      </div>

      <div className="flex min-h-0 flex-1">
        <pre
          ref={gutter}
          aria-hidden
          className={`w-[46px] shrink-0 overflow-hidden py-2.5 pr-2 text-right text-tertiary ${EDITOR_TEXT}`}
        >
          {numbers}
        </pre>
        <textarea
          ref={editor}
          data-autofocus
          value={value}
          wrap="off"
          spellCheck={false}
          autoCorrect="off"
          autoCapitalize="off"
          placeholder={PLACEHOLDER}
          onScroll={(event) => {
            if (gutter.current) gutter.current.scrollTop = event.currentTarget.scrollTop
          }}
          onChange={(event) =>
            onChange(
              event.target.value,
              event.nativeEvent instanceof InputEvent
                ? event.nativeEvent.inputType === 'insertFromPaste'
                : false,
            )
          }
          className={`min-w-0 flex-1 resize-none bg-transparent py-2.5 pr-3 text-primary caret-[var(--accent)] outline-none placeholder:text-[var(--text-tertiary)] ${EDITOR_TEXT}`}
        />
      </div>

      {error ? (
        <button
          type="button"
          onClick={() => error.offset != null && onJump(error.offset)}
          disabled={error.offset == null}
          className="hairline-t flex shrink-0 items-start gap-2 px-3 py-2 text-left text-[11px] leading-snug disabled:cursor-default"
          style={{ backgroundColor: 'var(--failed-wash)', color: 'var(--failed)' }}
        >
          <span aria-hidden className="shrink-0 pt-px font-medium">
            ✕
          </span>
          <span className="min-w-0 flex-1">
            {place ? (
              <span className="font-medium tabular-nums">
                Line {place.line}, column {place.column}{' '}
              </span>
            ) : null}
            {error.message}
          </span>
        </button>
      ) : null}
    </div>
  )
}

/* ── the preview ───────────────────────────────────────────────────────── */

/** What the paste comes to: the video, as far as the blueprint decides it. */
function OutlinePane({
  outline,
  stale,
  minutes,
  tasks,
  words,
}: {
  outline: Outline | null
  stale: boolean
  minutes: number
  tasks: number
  words: number
}) {
  if (outline === null) {
    return (
      <div className="flex h-full items-center justify-center px-10 text-center">
        <p className="max-w-[240px] text-[12px] leading-relaxed text-tertiary">
          Your outline appears here, and updates as you type.
        </p>
      </div>
    )
  }

  return (
    <div
      className="flex h-full min-h-0 flex-col transition-opacity duration-150"
      style={stale ? { opacity: 0.4 } : undefined}
    >
      <div className="hairline-b shrink-0 px-5 pt-4 pb-3">
        <h2 className="text-[15px] leading-tight font-semibold text-primary">
          {outline.title || <span className="text-tertiary">Untitled</span>}
        </h2>
        {outline.summary ? (
          <p className="mt-1.5 line-clamp-3 text-[11.5px] leading-relaxed text-secondary">
            {outline.summary}
          </p>
        ) : null}
        <p className="mt-2.5 text-[11px] text-tertiary">
          <span className="tabular-nums">{count(outline.chapters.length)}</span> chapters
          {outline.scripted > 0 ? (
            <>
              {' · '}
              <span className="tabular-nums">{count(outline.scripted)}</span> scripts
            </>
          ) : null}
          {words > 0 ? (
            <>
              {' · '}
              <span className="tabular-nums">{count(words)}</span> planned
              {' · ≈ '}
              <span className="tabular-nums">{coarse(minutes)}</span>
            </>
          ) : null}
          {outline.scriptWords > 0 ? (
            <>
              {' · '}
              <span className="tabular-nums">{count(outline.scriptWords)}</span> written
            </>
          ) : null}
          {' · '}
          <span className="tabular-nums">{count(tasks)}</span> tasks
          {stale ? <span className="ml-2">— showing the last valid outline</span> : null}
        </p>
      </div>

      {outline.blockers.length > 0 || outline.warnings.length > 0 ? (
        <div className="hairline-b shrink-0 space-y-1 px-5 py-2.5">
          {outline.blockers.map((note) => (
            <Note key={note} tone="var(--failed)" mark="✕" text={note} />
          ))}
          {outline.warnings.map((note) => (
            <Note key={note} tone="var(--running)" mark="⚠" text={note} />
          ))}
        </div>
      ) : null}

      <ChapterList chapters={outline.chapters} flagMissing={outline.scripted > 0} />
    </div>
  )
}

function Note({ tone, mark, text }: { tone: string; mark: string; text: string }) {
  return (
    <p className="flex items-start gap-1.5 text-[11px] leading-snug" style={{ color: tone }}>
      <span aria-hidden className="shrink-0">
        {mark}
      </span>
      <span className="min-w-0 flex-1">{text}</span>
    </p>
  )
}

/**
 * The chapters, open by default.
 *
 * Open, because the scripts are the reason to look: a pane that hid them behind
 * a disclosure would make checking a video an errand of thirty clicks, and the
 * one thing worth checking before pressing the button is that the narration
 * against each chapter is the narration meant for it.
 *
 * So what is tracked is the exception — the rows deliberately folded away —
 * which is also why the set survives an edit to the document. Indices shift
 * under it when chapters are added or removed, but the cost of that is a row
 * someone has to click open again, and resetting on every keystroke would undo
 * a fold the moment it was made.
 */
function ChapterList({ chapters, flagMissing }: { chapters: Outlined[]; flagMissing: boolean }) {
  const [collapsed, setCollapsed] = useState<ReadonlySet<number>>(() => new Set())
  const toggle = (index: number) =>
    setCollapsed((current) => {
      const next = new Set(current)
      if (!next.delete(index)) next.add(index)
      return next
    })

  return (
    <ol className="min-h-0 flex-1 overflow-y-auto px-3 py-2">
      {chapters.map((chapter, index) => (
        <ChapterRow
          key={index}
          chapter={chapter}
          open={!collapsed.has(index)}
          flagMissing={flagMissing}
          onToggle={() => toggle(index)}
        />
      ))}
    </ol>
  )
}

/**
 * One chapter, at a glance and then in full.
 *
 * Collapsed, the second line is the script's opening rather than the chapter's
 * brief whenever there is a script. That is the line that answers the question
 * this pane exists for — not "what was this chapter meant to cover" but "is the
 * narration sitting against chapter three the narration I wrote for chapter
 * three". An off-by-one is invisible in a brief and obvious in a first
 * sentence.
 *
 * Open, both are shown: the brief the writer would have been given, then the
 * words that will actually be spoken, quoted and inset so the two can never be
 * read as the same kind of text.
 *
 * `pacing` is the one field left out — `role` and the word count already say it.
 */
function ChapterRow({
  chapter,
  open,
  flagMissing,
  onToggle,
}: {
  chapter: Outlined
  open: boolean
  /** Whether a missing script is worth remarking on — see the note below. */
  flagMissing: boolean
  onToggle: () => void
}) {
  const scripted = chapter.script !== ''
  return (
    <li>
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={open}
        className="flex w-full gap-2.5 rounded-md px-2 py-[7px] text-left transition-colors hover:bg-[var(--hover)]"
      >
        <span className="w-[22px] shrink-0 pt-[1px] text-right text-[11px] text-tertiary tabular-nums">
          {chapter.order}
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-baseline gap-2">
            <span className="min-w-0 flex-1 truncate text-[12.5px] font-medium text-primary">
              {chapter.title}
            </span>
            {scripted ? (
              <span
                aria-label="has a script"
                title="A script was pasted for this chapter"
                className="shrink-0 text-[11px]"
                style={{ color: 'var(--done)' }}
              >
                ✎
              </span>
            ) : null}
            {chapter.role ? <Chip>{chapter.role.replace(/_/g, ' ')}</Chip> : null}
            {chapter.tone ? (
              <span className="shrink-0 text-[10.5px] text-tertiary">{chapter.tone}</span>
            ) : null}
            <span
              className="w-[46px] shrink-0 text-right text-[11px] tabular-nums"
              style={{
                color:
                  chapter.words > 0 && chapter.words < THIN_CHAPTER_WORDS
                    ? 'var(--running)'
                    : 'var(--text-secondary)',
              }}
            >
              {chapter.words > 0 ? `${count(chapter.words)}w` : '—'}
            </span>
            <span
              aria-hidden
              className="w-[10px] shrink-0 text-[10px] text-tertiary transition-transform"
              style={{ transform: open ? 'rotate(90deg)' : undefined }}
            >
              ›
            </span>
          </div>
          {/* Italic when it is narration, upright when it is direction. A
              quotation mark would be cut off by the truncation before its pair
              ever appeared, which reads as a typo rather than as a quote. */}
          {!open ? (
            <p
              className="mt-[3px] truncate text-[11px] text-tertiary"
              style={scripted ? { fontStyle: 'italic' } : undefined}
            >
              {scripted ? chapter.script : chapter.concept}
            </p>
          ) : null}
        </div>
      </button>

      {open ? (
        <div className="mb-1 pr-2 pl-[34px]">
          {chapter.concept ? (
            <p className="text-[11px] leading-relaxed text-tertiary">{chapter.concept}</p>
          ) : null}
          {scripted ? (
            <>
              <blockquote
                className="mt-2 rounded-[6px] border-l-2 py-1.5 pr-2 pl-2.5 text-[11.5px] leading-relaxed whitespace-pre-wrap text-secondary"
                style={{ backgroundColor: 'var(--band)', borderColor: 'var(--accent-wash-strong)' }}
              >
                {chapter.script}
              </blockquote>
              <p className="mt-1 text-[10.5px] text-tertiary">
                <span className="tabular-nums">{count(chapter.scriptWords)}</span> words written
                {chapter.words > 0 ? (
                  <>
                    {' · '}
                    <span className="tabular-nums">{count(chapter.words)}</span> planned
                  </>
                ) : null}
              </p>
            </>
          ) : flagMissing ? (
            // Only when some other chapter has one. An ordinary blueprint has no
            // scripts anywhere and is not missing anything, so saying so against
            // every chapter of it would be thirty warnings about nothing.
            <p className="mt-2 text-[11px]" style={{ color: 'var(--running)' }}>
              No script — the model will write this chapter.
            </p>
          ) : null}
        </div>
      ) : null}
    </li>
  )
}

function Chip({ children }: { children: React.ReactNode }) {
  return (
    <span
      className="shrink-0 rounded-[4px] px-1.5 py-[1px] text-[10px] font-medium text-secondary"
      style={{ backgroundColor: 'var(--band)' }}
    >
      {children}
    </span>
  )
}

/**
 * A count and its unit, laid out inline.
 *
 * A box rather than the track the create dialog uses for the same two figures:
 * a slider wants horizontal room this bar does not have, and both of these are
 * two-digit numbers you set once for a channel and then leave alone.
 */
function Stepper({
  label,
  unit,
  value,
  onChange,
  min,
  max,
}: {
  label: string
  unit: string
  value: string
  onChange: (value: string) => void
  min: number
  max: number
}) {
  const id = useId()
  const bad = !within(value, { min, max })
  return (
    <label htmlFor={id} className="flex shrink-0 items-center gap-2">
      <span className="text-[11px] text-secondary">{label}</span>
      <Input
        id={id}
        type="number"
        min={min}
        max={max}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        className="w-[52px] text-right tabular-nums"
        style={bad ? { color: 'var(--failed)' } : undefined}
      />
      <span className="text-[11px] text-tertiary">{unit}</span>
    </label>
  )
}
