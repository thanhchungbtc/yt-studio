import type { ReactNode } from 'react'

import { Caption } from '../ui/caption'
import { Select } from '../ui/field'
import { maxHeadlineWeight, type Style } from './style'

/**
 * The tunables, as a rail beside the picture.
 *
 * Every entry in `style.go` that describes a *look* is here, in the order and
 * the grouping that file uses, because the renderer's own notes are organised
 * around what raising each number does and reordering them by some other logic
 * would throw that away.
 *
 * Sliders rather than steppers, and this is the one real design decision in the
 * file. These are pixel values on a picture that redraws while you drag: what
 * an operator wants to know is "does 34 look better than 28", and a slider
 * answers that by sweeping through the answers where a number field makes you
 * guess, type, and look. The exact figure still matters afterwards, so it is
 * printed beside every one.
 *
 * The label column is narrow and the slider takes the rest. A rail is read down
 * its left edge, so the names line up and the controls are all the same length
 * regardless of how long the name is.
 */

/* ---------------------------------------------------------------- controls */

/** One tunable: its name, a track to drag, and the figure it currently is. */
function Knob({
  label,
  value,
  min,
  max,
  onChange,
}: {
  label: string
  value: number
  min: number
  max: number
  onChange: (value: number) => void
}) {
  return (
    <label className="flex items-center gap-2 py-[3px]">
      <span className="w-[86px] shrink-0 truncate text-[11px] text-secondary" title={label}>
        {label}
      </span>
      <input
        type="range"
        min={min}
        max={max}
        value={value}
        onChange={(event) => onChange(Number(event.target.value))}
        className="thumb-slider min-w-0 flex-1"
      />
      <span className="w-[38px] shrink-0 text-right text-[11px] tabular-nums text-tertiary">
        {value}
      </span>
    </label>
  )
}

/**
 * A colour well.
 *
 * The platform picker, not one built here: it arrives knowing about the system
 * palette, the eyedropper and the recently-used swatches, none of which is
 * worth reimplementing for four colours.
 */
function Swatch({
  label,
  value,
  onChange,
}: {
  label: string
  value: string
  onChange: (value: string) => void
}) {
  return (
    <label className="flex items-center gap-2 py-[3px]">
      <span className="w-[86px] shrink-0 truncate text-[11px] text-secondary">{label}</span>
      <input
        type="color"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        className="h-[20px] min-w-0 flex-1 cursor-pointer rounded-[4px] border-0 bg-transparent p-0"
        style={{ boxShadow: '0 0 0 0.5px var(--separator-strong)' }}
      />
      <span className="w-[38px] shrink-0 text-right font-mono text-[10px] text-tertiary">
        {value.replace('#', '')}
      </span>
    </label>
  )
}

/**
 * A list of words, for the one tunable that is language rather than geometry.
 *
 * Full width under its own label rather than in the label-track-figure column:
 * two dozen function words in the width a slider leaves is a list you cannot
 * read, let alone edit. Free text and not a tag field, because it mirrors a
 * settings row the operator types into and the two should look like the same
 * thing.
 */
function Words({
  label,
  hint,
  value,
  onChange,
}: {
  label: string
  hint: string
  value: string
  onChange: (value: string) => void
}) {
  return (
    <label className="block py-[3px]">
      <span className="mb-1 block text-[11px] text-secondary">{label}</span>
      <textarea
        value={value}
        onChange={(event) => onChange(event.target.value)}
        rows={2}
        spellCheck={false}
        className="w-full resize-y rounded-[4px] bg-transparent px-1.5 py-1 font-mono text-[10px] leading-[1.4] text-secondary"
        style={{ boxShadow: '0 0 0 0.5px var(--separator-strong)' }}
      />
      <span className="mt-1 block text-[10px] leading-[1.3] text-tertiary">{hint}</span>
    </label>
  )
}

/** A band of related tunables, with the renderer's own name for the group. */
function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="hairline-b px-4 py-3 last:border-b-0">
      <Caption className="mb-1.5 block">{title}</Caption>
      {children}
    </div>
  )
}

/* -------------------------------------------------------------------- rail */

export function StyleControls({
  style,
  fonts,
  onChange,
}: {
  style: Style
  /** The typefaces on the machine, as the `thumbnail.font` setting offers them. */
  fonts: { value: string; label: string }[]
  onChange: (next: Style) => void
}) {
  // One setter, so a group is a list of controls rather than a list of
  // controls each carrying its own closure over the whole object.
  const set =
    <K extends keyof Style>(key: K) =>
    (value: Style[K]) =>
      onChange({ ...style, [key]: value })

  return (
    <div className="flex flex-col">
      <Group title="Type">
        {/*
          The only control here that is not a number, so it does not fit the
          label-track-figure column and does not try to: a face called "Cabin
          Sketch Bold" in the width a slider leaves is a name you cannot read.
        */}
        <div className="mb-1.5">
          <Select value={style.font} onChange={(event) => set('font')(event.target.value)}>
            {/* The configured face is always an option even when the scan did
                not find it: a font added to the directory after the server
                started is one the settings row still names. */}
            {fonts.some((font) => font.value === style.font) ? null : (
              <option value={style.font}>{style.font}</option>
            )}
            {fonts.map((font) => (
              <option key={font.value} value={font.value}>
                {font.label}
              </option>
            ))}
          </Select>
        </div>
        <Knob
          label="Headline max"
          value={style.headlineFontMax}
          min={40}
          max={220}
          onChange={set('headlineFontMax')}
        />
        <Knob
          label="Headline min"
          value={style.headlineFontMin}
          min={16}
          max={160}
          onChange={set('headlineFontMin')}
        />
        {/*
          Tenths, because the useful range is nought to one and a half and a
          slider in whole pixels would have two stops in it. The Knob prints an
          integer, so the figure beside it is tenths too -- 12 is 1.2px.
        */}
        <Knob
          label="Weight 1/10px"
          value={Math.round(style.headlineWeight * 10)}
          min={0}
          max={maxHeadlineWeight * 10}
          onChange={(value) => set('headlineWeight')(value / 10)}
        />
        {/* A divisor, so *lower* is looser -- which is the opposite of what a
            slider labelled "tracking" implies, hence the name and the note. */}
        <Knob
          label="Tracking 1/n"
          value={style.headlineTracking}
          min={6}
          max={80}
          onChange={set('headlineTracking')}
        />
        <Knob
          label="Max lines"
          value={style.headlineMaxLines}
          min={1}
          max={4}
          onChange={set('headlineMaxLines')}
        />
        <Knob
          label="Line gap"
          value={style.headlineLineGap}
          min={0}
          max={60}
          onChange={set('headlineLineGap')}
        />
        <Knob
          label="Caption max"
          value={style.captionFontMax}
          min={10}
          max={64}
          onChange={set('captionFontMax')}
        />
        <Knob
          label="Caption min"
          value={style.captionFontMin}
          min={6}
          max={48}
          onChange={set('captionFontMin')}
        />
        {/*
          Here rather than in Palette, though it decides a colour: what the
          operator is editing is which words the hook is not about, and the
          colour they land in is one swatch away in the group that holds every
          other colour. Mirrors `thumbnail.headline.minor_words`.
        */}
        <Words
          label="Minor words"
          hint="Drawn in the minor colour. Commas or spaces; empty draws the headline in one colour."
          value={style.headlineMinorWords}
          onChange={set('headlineMinorWords')}
        />
      </Group>

      <Group title="Grid">
        <Knob label="Rows" value={style.rows} min={1} max={6} onChange={set('rows')} />
        <Knob
          label="Tile spacing"
          value={style.tileSpacing}
          min={0}
          max={90}
          onChange={set('tileSpacing')}
        />
        <Knob
          label="Side margin"
          value={style.gridSideMargin}
          min={0}
          max={200}
          onChange={set('gridSideMargin')}
        />
        <Knob
          label="Bottom margin"
          value={style.gridBottomMargin}
          min={0}
          max={120}
          onChange={set('gridBottomMargin')}
        />
        <Knob
          label="Head top"
          value={style.headlineTopMargin}
          min={0}
          max={160}
          onChange={set('headlineTopMargin')}
        />
        <Knob
          label="Head sides"
          value={style.headlineSideMargin}
          min={0}
          max={300}
          onChange={set('headlineSideMargin')}
        />
        <Knob
          label="Head to grid"
          value={style.headlineToGridGap}
          min={0}
          max={140}
          onChange={set('headlineToGridGap')}
        />
        <Knob
          label="Tile to caption"
          value={style.tileToCaptionGap}
          min={0}
          max={60}
          onChange={set('tileToCaptionGap')}
        />
      </Group>

      <Group title="Tile">
        <Knob
          label="Border"
          value={style.tileBorderWidth}
          min={0}
          max={24}
          onChange={set('tileBorderWidth')}
        />
        <Knob
          label="Corner"
          value={style.tileCornerRadius}
          min={0}
          max={120}
          onChange={set('tileCornerRadius')}
        />
        <Knob
          label="Icon inset"
          value={style.tileIconPadding}
          min={0}
          max={80}
          onChange={set('tileIconPadding')}
        />
      </Group>

      {/* One section for all four, as `style.go` has it: colours are read
          against each other, and a plate colour two groups away from the
          caption colour that sits on it is a comparison nobody can make. */}
      <Group title="Palette">
        <Swatch label="Headline" value={style.headlineColor} onChange={set('headlineColor')} />
        {/*
          Directly under the headline it is read against, because the whole
          design is the *gap* between the two: a minor colour chosen against
          anything but the major one is chosen blind.
        */}
        <Swatch
          label="Headline minor"
          value={style.headlineMinorColor}
          onChange={set('headlineMinorColor')}
        />
        <Swatch label="Captions" value={style.captionColor} onChange={set('captionColor')} />
        <Swatch label="Tile plate" value={style.tileFillColor} onChange={set('tileFillColor')} />
        {/* Alpha is honoured everywhere: at 0 the backdrop shows through the
            tiles entirely, and at 255 they are solid plates. */}
        <Knob
          label="Plate opacity"
          value={style.tileFillAlpha}
          min={0}
          max={255}
          onChange={set('tileFillAlpha')}
        />
        <Swatch
          label="Tile border"
          value={style.tileBorderColor}
          onChange={set('tileBorderColor')}
        />
        <Knob
          label="Border opacity"
          value={style.tileBorderAlpha}
          min={0}
          max={255}
          onChange={set('tileBorderAlpha')}
        />
      </Group>

      <Group title="Image">
        {/* Out of 255, and lower is darker: white type over an undimmed
            photograph is unreadable, which is the whole reason it exists. */}
        <Knob
          label="Backdrop"
          value={style.backgroundBrightness}
          min={20}
          max={255}
          onChange={set('backgroundBrightness')}
        />
        {/* The two ends of the ramp that turns an icon's dark field into
            transparency. Raise the first to eat a background that is not quite
            black; lower the second to keep more of the darker shading. */}
        <Knob
          label="Key below"
          value={style.iconTransparentBelow}
          min={0}
          max={200}
          onChange={set('iconTransparentBelow')}
        />
        <Knob
          label="Opaque above"
          value={style.iconOpaqueAbove}
          min={1}
          max={255}
          onChange={set('iconOpaqueAbove')}
        />
      </Group>
    </div>
  )
}
