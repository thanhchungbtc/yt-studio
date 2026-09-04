import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'

import { api, qk } from '../../core/api'
import type { Video } from '../../core/types'
import { Button } from '../ui/button'
import { Caption } from '../ui/caption'
import { Dialog } from '../ui/dialog'
import { Input } from '../ui/field'
import { compose, gridShape, loadFont, loadImage, type Cell, type Report } from './compose'
import { emphasis, headlineKey, minorWords } from './text'
import { StyleControls } from './controls'
import {
  backgroundURL,
  defaultStyle,
  frameHeight,
  frameWidth,
  isDefault,
  type Style,
} from './style'

/**
 * The headline's words, each in the colour it will be drawn in, click to swap.
 *
 * Labelled "Minor" and not "Dim": which of the two colours is the darker one is
 * a palette decision that has already changed once, and a control that lies
 * about it the next time is worse than one that names the set instead.
 *
 * Because the storage format is an asterisk span and nobody should have to type
 * one -- it reads backwards against every other use of the character, and a
 * mis-paired mark is invisible until the picture is wrong. Chips make the
 * marking a thing you point at, and they are the *resolved* answer: a word the
 * settings list claims shows dim here too, so what is on screen is what
 * renders, not half of it.
 *
 * Clicking writes marks back into the headline, which is what carries the
 * choice to an unattended render. That is why the field above shows them: one
 * string, one source of truth, and hand-editable by anyone who prefers it.
 */
function Emphasis({
  headline,
  minor,
  colors,
  onChange,
}: {
  headline: string
  minor: string
  colors: Pick<Style, 'headlineColor' | 'headlineMinorColor'>
  onChange: (headline: string) => void
}) {
  const listed = useMemo(() => minorWords(minor), [minor])
  const { words, dim } = useMemo(() => emphasis(headline, listed), [headline, listed])
  if (words.length === 0) return null

  // Rebuilt from the flags rather than edited in place: the marks the operator
  // typed may be mis-paired, mid-word or redundant, and normalising to one span
  // per run of dim words is what keeps a dozen clicks from growing a dozen
  // asterisks.
  const write = (next: boolean[]) => {
    const out: string[] = []
    for (let i = 0; i < words.length; i += 1) {
      const opens = next[i] && !next[i - 1]
      const closes = next[i] && !next[i + 1]
      out.push(`${opens ? '*' : ''}${words[i]}${closes ? '*' : ''}`)
    }
    onChange(out.join(' '))
  }

  return (
    <div className="hairline-b flex shrink-0 items-center gap-3 px-6 py-2">
      <Caption className="w-[56px] shrink-0">Minor</Caption>
      <div className="flex min-w-0 flex-1 flex-wrap items-center gap-1">
        {words.map((word, i) => (
          <button
            key={`${word}-${i}`}
            type="button"
            onClick={() => write(dim.map((d, at) => (at === i ? !d : d)))}
            className="rounded-[4px] px-1.5 py-0.5 text-[11px] leading-[1.4] transition-opacity hover:opacity-70"
            style={{
              color: dim[i] ? colors.headlineMinorColor : colors.headlineColor,
              boxShadow: '0 0 0 0.5px var(--separator-strong)',
            }}
            // A word the settings list claims cannot be un-dimmed here -- the
            // two sources are unioned -- so say so rather than let the click
            // look broken.
            title={
              dim[i] && listed.has(headlineKey(word))
                ? `"${word}" is in the minor-words setting, so it stays dim`
                : undefined
            }
          >
            {word}
          </button>
        ))}
      </div>
      <Button
        onClick={() => write(words.map((word) => defaultMinor.has(headlineKey(word))))}
        className="shrink-0"
      >
        Auto
      </Button>
      <Button onClick={() => write(words.map(() => false))} className="shrink-0">
        None
      </Button>
    </div>
  )
}

/**
 * The words Auto marks: English function words, the ones a hook is never about.
 * Go's `entity.DefaultHeadlineMinorWords`, which is a suggestion there too --
 * the settings row seeds empty, and this is how it is offered instead.
 */
const defaultMinor = minorWords(
  'a,an,and,are,as,at,be,but,by,for,from,in,is,of,on,or,so,than,that,the,this,to,was,were,with',
)

/**
 * The thumbnail builder.
 *
 * A dialog rather than a document, which is the whole shape of the thing: this
 * is not a place you work in and come back to, it is a question about one
 * picture, asked and answered in front of the video it belongs to. It replaced
 * a dockview tab that could sit open beside an unrelated video, which is the
 * arrangement where you edit the wrong one.
 *
 * The canvas is 1280x720 and shown scaled. It is not a preview of an export --
 * it is the export, and `toBlob` on save hands over the pixels that were on
 * screen. There is no second code path that could disagree with what the
 * operator was looking at, which is the only reason a browser is allowed to be
 * the authority on what publishes.
 *
 * Everything is on screen at once and nothing scrolls, which is the point of
 * the layout: the picture at the top, the text that is on the picture beneath
 * it, and a keystroke in either changes the first. An earlier version stacked
 * twelve full-width caption rows down a scrolling body, so the field being
 * typed in and the tile it belongs to were never visible together -- and four
 * of the twelve were not visible at all.
 *
 * So the caption fields are laid out in the *same grid as the tiles*: six
 * across and two down, in reading order, where they are in the picture above.
 * Which field belongs to which tile stops being a question you answer by
 * counting. And a field ends up about as wide as the tile it captions, so text
 * that overruns the box is text that was going to overrun the tile.
 *
 * Two fields and no more. The icon prompts are not here: redrawing a cell
 * re-runs a backend task, which reopens the upload gate, and that is a
 * different kind of decision from correcting a typo.
 */
export function ThumbnailDialog({
  video,
  open,
  onOpenChange,
}: {
  video: Video
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const client = useQueryClient()

  /*
    The canvas in state rather than in a ref, which is the fix for a real bug:
    reopen the dialog and the picture was blank until you typed something.

    Radix unmounts the dialog's content when it closes, so reopening mounts a
    *new* canvas element. With the element in a ref, nothing the drawing effect
    depends on had changed -- same captions, same headline, same style -- so the
    effect did not re-run and the fresh canvas was never drawn into. Any edit
    changed a dependency and the picture appeared, which is exactly the shape
    the report described.

    The element was the missing dependency. Held in state, the ref callback fires
    with the new node, that is a state change, and the draw follows the canvas it
    draws into instead of hoping the data moved at the same time.
  */
  const [canvas, setCanvas] = useState<HTMLCanvasElement | null>(null)

  // The renderer reads both from settings, so the builder has to as well or it
  // would compose a grid the pipeline would not have produced. Already cached:
  // the settings screen fetches this list and keys it the same way.
  const settings = useQuery({ queryKey: qk.settings, queryFn: api.listSettings, enabled: open })
  const fontSetting = settings.data?.find((row) => row.key === 'thumbnail.font')
  const rowsSetting = settings.data?.find((row) => row.key === 'thumbnail.grid.rows')

  // What the picture is made of, as the operator has it. Seeded from the saved
  // document if there is one, so reopening shows the edits that are live rather
  // than the plan they were made from -- and from the plan when there is not.
  const saved = useMemo(() => readDesign(video.thumbnailDesign), [video.thumbnailDesign])
  const [headline, setHeadline] = useState('')
  const [captions, setCaptions] = useState<string[]>([])
  const [style, setStyle] = useState<Style>(defaultStyle)

  /*
    The two settings the renderer reads, as this style's starting point.

    `defaultStyle` carries the same two values, but as the *fallback* rather
    than the truth: an operator who set `thumbnail.grid.rows` to three would
    otherwise open the builder onto a two-row grid and see a picture the
    pipeline would never produce.
  */
  const configured = useMemo((): Style => {
    const rows = Number(rowsSetting?.value)
    return {
      ...defaultStyle,
      font: fontSetting?.value || defaultStyle.font,
      rows: Number.isFinite(rows) && rows >= 1 ? rows : defaultStyle.rows,
    }
  }, [fontSetting?.value, rowsSetting?.value])

  // Only on the way in. Re-seeding whenever the video refetches would throw
  // away whatever was half-typed, and a video refetches on every task delta.
  useEffect(() => {
    if (!open) return
    setHeadline(saved?.headline ?? video.metadata?.thumbnailText ?? '')
    setCaptions(saved?.captions ?? video.thumbnailPlan.map((cell) => cell.caption))
    setStyle(saved?.style ?? configured)
  }, [configured, open, saved, video.metadata?.thumbnailText, video.thumbnailPlan])

  // The font and the backdrop, both from `/resources`, which the server serves
  // precisely so that this composes against the same two files the Go renderer
  // does. The font is awaited before anything measures: see `loadFont`.
  const resources = useQuery({
    queryKey: ['v2', 'thumbnail', 'resources', style.font] as const,
    queryFn: async () => ({
      family: await loadFont(style.font),
      background: await loadImage(backgroundURL),
    }),
    enabled: open,
    staleTime: Infinity,
  })

  const icons = useQuery({
    queryKey: ['v2', 'thumbnail', 'icons', video.thumbnailIconIds] as const,
    queryFn: () =>
      Promise.all(
        video.thumbnailIconIds.map((id) =>
          id ? loadImage(`/assets/${id}`) : Promise.resolve(null),
        ),
      ),
    enabled: open && video.thumbnailIconIds.length > 0,
    staleTime: Infinity,
  })

  const ready =
    Boolean(resources.data) && (video.thumbnailIconIds.length === 0 || Boolean(icons.data))

  // What the fitting settled on, for the two readouts. Taken from the draw
  // rather than computed a second time here.
  const [report, setReport] = useState<Report | null>(null)

  // Redrawn on every keystroke. It costs a few million pixel operations, which
  // at this size is a couple of frames -- and a headline that resizes as you
  // type is the entire reason to build this in the browser at all.
  useEffect(() => {
    if (!canvas || !resources.data || !ready) return
    const cells: Cell[] = captions.map((caption, index) => ({
      caption,
      icon: icons.data?.[index] ?? null,
    }))
    setReport(
      compose(canvas, {
        headline,
        cells,
        style,
        family: resources.data.family,
        background: resources.data.background,
      }),
    )
  }, [canvas, captions, headline, icons.data, ready, resources.data, style])

  const settle = (next: Video) => {
    client.setQueryData(qk.video(video.ref), next)
    void client.invalidateQueries({ queryKey: qk.videos })
  }

  const publish = useMutation({
    mutationFn: async () => {
      if (!canvas) throw new Error('nothing has been drawn')
      const png = await toPNG(canvas)
      // The design after the image, and only if the image landed: a document
      // saved against a picture that failed to store would reopen describing
      // something that is not published anywhere.
      await api.applyThumbnailOverride(video.ref, png)
      // The *second* response is the one that goes in the cache, and getting
      // that wrong was a bug worth the comment: both calls return the whole
      // video, but the override's was taken before the design existed. Caching
      // it meant reopening the builder seeded from a document that had already
      // been replaced -- every slider snapped back to the renderer's defaults
      // while the published picture kept the edits.
      return api.saveThumbnailDesign(video.ref, { headline, captions, style })
    },
    onSuccess: (next) => {
      settle(next)
      onOpenChange(false)
    },
    // On both paths, because the two calls are not one transaction: an override
    // that stored and a design that did not leaves the cache describing neither
    // state, and the server is the only thing that knows which happened.
    onSettled: () => {
      void client.invalidateQueries({ queryKey: qk.video(video.ref) })
    },
  })

  const revert = useMutation({
    mutationFn: () => api.clearThumbnailOverride(video.ref),
    onSuccess: (next) => {
      settle(next)
      onOpenChange(false)
    },
  })

  const busy = publish.isPending || revert.isPending
  const failure = publish.error ?? revert.error ?? resources.error ?? icons.error
  const live = Boolean(video.thumbnailOverrideAssetId)

  // The tiles' own shape, so the fields can be laid out in it.
  const shape = gridShape(captions.length, style)
  const styled = !isDefault(style)

  return (
    // Fixed height as well as width. The caption grid is one, two or three rows
    // deep depending on a setting, and the canvas takes whatever is left -- so
    // the dialog is the same size either way instead of resizing as it opens.
    //
    // The height is chosen so the picture comes out about as wide as the dialog
    // allows: any shorter and the canvas is limited by height instead, and the
    // extra width goes to margins either side of it. On a screen too short for
    // it the shell's own `max-h` takes over and the canvas shrinks to fit,
    // which is the whole reason it is contained rather than sized.
    <Dialog open={open} onOpenChange={onOpenChange} width={1240} height={880}>
      <Dialog.Header title="Thumbnail" description={description(live, styled)} />
      <Dialog.Close />

      {/* `bare` because this body is a layout rather than a form: it wants no
          gutter insetting it from the edges its bands should meet, and no
          single scroller dragging the picture and the fields together. */}
      <Dialog.Body bare>
        <div className="flex min-w-0 flex-1 flex-col">
          {/*
            The headline on one line with its label, at the top, because it is
            one short string and the biggest thing in the picture. A stacked
            label over a full-width field spent forty pixels of height saying
            what one word says inline.
          */}
          <div className="hairline-b flex shrink-0 items-center gap-3 px-6 py-3">
            <Caption className="w-[56px] shrink-0">Headline</Caption>
            <Input
              data-autofocus
              value={headline}
              onChange={(event) => setHeadline(event.target.value)}
              placeholder="The all-caps hook"
            />
            {/*
              What the fitter chose, which answers the question the picture
              raises the moment a headline gets long: it did not ignore you, it
              ran out of room. Fixed width, so the field beside it does not
              resize as the number changes.
            */}
            <span className="w-[86px] shrink-0 text-right text-[11px] tabular-nums text-tertiary">
              {report && report.headlineSize > 0 ? fittedAs(report) : ''}
            </span>
          </div>

          <Emphasis
            headline={headline}
            minor={style.headlineMinorWords}
            colors={style}
            onChange={setHeadline}
          />

          {/* The picture takes every pixel the fields do not, and is contained
              rather than cropped: `width/height: auto` against both maxima is
              what makes a canvas with a 1280x720 intrinsic size shrink to fit
              whichever of the two runs out first. */}
          <div className="flex min-h-0 flex-1 items-center justify-center px-6 py-4">
            <canvas
              ref={setCanvas}
              width={frameWidth}
              height={frameHeight}
              className="rounded-[8px]"
              style={{
                maxWidth: '100%',
                maxHeight: '100%',
                width: 'auto',
                height: 'auto',
                backgroundColor: '#000',
                boxShadow: '0 0 0 0.5px var(--separator-strong)',
              }}
            />
          </div>

          {captions.length > 0 ? (
            <div className="hairline-t shrink-0 px-6 pt-3 pb-4">
              <div className="mb-2 flex items-baseline gap-2">
                <Caption>Captions</Caption>
                {/*
                  One size for all of them is the renderer's rule rather than a
                  limitation of this screen, and it is worth saying: otherwise
                  shortening one caption appears to do nothing, when what it
                  actually did was let every caption grow.
                */}
                <span className="text-[11px] tabular-nums text-tertiary">
                  {report && report.captionSize > 0 ? `all at ${report.captionSize}px` : ''}
                </span>
              </div>

              <div
                className="grid gap-2"
                // From the renderer's own geometry, so the fields sit where the
                // tiles do at whatever `thumbnail.grid.rows` is set to.
                style={{ gridTemplateColumns: `repeat(${shape.cols}, minmax(0, 1fr))` }}
              >
                {captions.map((caption, index) => (
                  <label key={index} className="flex min-w-0 items-center gap-1.5">
                    {/* The artwork itself rather than an ordinal. Twelve
                        numbered rows all read the same; twelve pictures do
                        not, and the picture is what is on the tile. */}
                    <span
                      className="size-[26px] shrink-0 overflow-hidden rounded-[5px]"
                      style={{ backgroundColor: 'var(--band)' }}
                    >
                      {video.thumbnailIconIds[index] ? (
                        <img
                          src={`/assets/${video.thumbnailIconIds[index]}`}
                          alt=""
                          className="size-full object-cover"
                        />
                      ) : null}
                    </span>
                    <Input
                      value={caption}
                      onChange={(event) =>
                        setCaptions((all) =>
                          all.map((value, at) => (at === index ? event.target.value : value)),
                        )
                      }
                      aria-label={`Caption ${index + 1}`}
                    />
                  </label>
                ))}
              </div>
            </div>
          ) : null}
        </div>

        {/*
          The rail, and it scrolls on its own. Twenty-five tunables is more than
          a pane holds, and the picture beside it must not move while they are
          being dragged -- which is the whole reason this is a second scroller
          rather than one body scrolling everything.
        */}
        <div className="hairline-l flex w-[296px] shrink-0 flex-col">
          <div className="hairline-b flex shrink-0 items-center gap-2 px-4 py-2">
            <Caption>Style</Caption>
            {/*
              What the renderer would have used, one click away. Shown only once
              something has moved, and it says *renderer* rather than "default"
              because that is the fact that matters: this is the way back to an
              image the pipeline could have produced on its own.
            */}
            {styled ? (
              <button
                type="button"
                onClick={() => setStyle(configured)}
                className="ml-auto text-[11px] text-[var(--accent)] hover:underline"
              >
                Reset
              </button>
            ) : null}
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto">
            <StyleControls
              style={style}
              fonts={fontSetting?.suggestions ?? []}
              onChange={setStyle}
            />
          </div>
        </div>
      </Dialog.Body>

      {/*
        Above the footer rather than at the end of the body, and the difference
        is not cosmetic: a message inside the body was a message that could be
        scrolled away from, so a save that failed looked exactly like a save
        that did nothing.
      */}
      {failure ? (
        <p
          className="hairline-t shrink-0 px-6 py-2 text-[12px] text-[var(--failed)]"
          style={{ backgroundColor: 'var(--failed-wash)' }}
        >
          {(failure as Error).message}
        </p>
      ) : null}

      <Dialog.Footer>
        {/* Only where there is one to drop. Reverting is a decision about which
            of two images publishes, so it belongs beside the one that decides
            the other way -- and away from it, on the leading edge. */}
        {live ? (
          <Button onClick={() => revert.mutate()} disabled={busy}>
            {revert.isPending ? 'Reverting…' : 'Revert to rendered'}
          </Button>
        ) : null}
        <span className="flex-1" />
        <Button onClick={() => onOpenChange(false)} disabled={busy}>
          Cancel
        </Button>
        <Button primary onClick={() => publish.mutate()} disabled={busy || !ready}>
          {publish.isPending ? 'Saving…' : 'Use this thumbnail'}
        </Button>
      </Dialog.Footer>
    </Dialog>
  )
}

/**
 * What the dialog is showing, in one sentence.
 *
 * The style being untouched is worth stating outright rather than leaving to be
 * inferred from an absence: it is the difference between an image the pipeline
 * would have produced by itself and one only this screen can make. Both are
 * legitimate -- an override exists to depart from the renderer -- but which of
 * the two you are looking at should not be a thing you have to work out.
 */
function description(live: boolean, styled: boolean): string {
  const source = styled
    ? 'Restyled here, so the renderer would not produce this on its own.'
    : 'Drawn exactly the way the renderer draws it.'
  return live
    ? `${source} This video already publishes a thumbnail built here; saving replaces it.`
    : `${source} Saving makes this the one that publishes.`
}

/** The size the headline settled at, and whether it had to wrap to get there. */
function fittedAs(report: Report): string {
  const lines = report.headlineLines > 1 ? `${report.headlineLines} lines` : '1 line'
  return `${report.headlineSize}px · ${lines}`
}

/** The builder's document, as it comes back off the server. */
interface Design {
  headline: string
  captions: string[]
  /** Absent in documents written before the controls existed. */
  style?: Style
}

/**
 * Narrow the stored document, or decide there isn't one.
 *
 * The server keeps this as opaque JSON and guarantees only that it comes back
 * the way it went in, so anything could be in the column -- including a
 * document written by an older build of this dialog. Checked rather than cast:
 * a bad shape means fall back to the plan, which is always correct.
 */
function readDesign(raw: unknown): Design | null {
  if (!raw || typeof raw !== 'object') return null
  const value = raw as Partial<Design>
  if (typeof value.headline !== 'string') return null
  if (!Array.isArray(value.captions)) return null
  if (!value.captions.every((caption) => typeof caption === 'string')) return null
  return { headline: value.headline, captions: value.captions, style: readStyle(value.style) }
}

/**
 * The stored style, key by key, over the renderer's own values.
 *
 * Overlaid rather than accepted whole, and that is what makes the document
 * survive both directions of change: one written before the controls existed
 * has no style at all and gets the defaults, and one written by a later build
 * that added a tunable keeps every value it does carry while the new one falls
 * back. A key of the wrong type is dropped rather than trusted -- the server
 * never looked inside this, so nothing upstream has checked it.
 */
function readStyle(raw: unknown): Style | undefined {
  if (!raw || typeof raw !== 'object') return undefined
  const stored = raw as Record<string, unknown>
  const style = { ...defaultStyle }
  for (const key of Object.keys(defaultStyle) as (keyof Style)[]) {
    const value = stored[key]
    if (typeof value === typeof defaultStyle[key]) {
      // Safe by the check above: the stored value has the field's own type.
      ;(style[key] as unknown) = value
    }
  }
  return style
}

/** The canvas as PNG bytes. `toBlob` is async and can hand back nothing. */
function toPNG(canvas: HTMLCanvasElement): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (blob) resolve(blob)
      else reject(new Error('the picture could not be encoded'))
    }, 'image/png')
  })
}
