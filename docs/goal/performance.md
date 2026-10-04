# Goal: Instant

The app feels laggy. Make it feel instant.

## Outcome

Switching pages, tabs, videos and chapters is instant — even with many videos,
large chapter lists, and pipelines running. Typing, scrolling, opening dialogs
and resizing panes never stutter. It should feel like a native macOS app.

## Audit (2026-10-04)

Measured on the production build in Chromium against a copy of the real library
(SML-7: 50 chapters, 317 tasks), with Event Timing for the press-to-paint delay
and Long Tasks for what blocks the next input. Figures are at 4× CPU throttle,
which is what makes a busy or older Mac visible; unthrottled, every interaction
below now acknowledges within ~24 ms, the floor of the measurement.

| Interaction                | Before                      | After                  |
| -------------------------- | --------------------------- | ---------------------- |
| Switch tab                 | 150–200 ms, 130–175 ms task | 24–40 ms, no task      |
| Toggle inspector / console | 64–80 ms                    | 48–64 ms, no task      |
| Return to a visited mode   | 90–280 ms task              | 0 ms task (Chapters)   |
| Open a video from the list | on mouse-up                 | on mouse-down          |
| Regenerate / retry a dot   | dot moves after round trip  | dot moves on the press |

What was slow, and the rule each fix follows:

- **Tab switches re-laid out the whole document.** Dockview's default renderer
  detaches a hidden tab's content and re-attaches it on the way back, then
  `_scrollTabIntoView` forced a synchronous layout of all of it inside the
  click. The editor area now uses `defaultRenderer="always"`, and a hidden
  overlay gets `content-visibility: hidden` so it costs nothing while hidden
  but keeps its rendering state. *Rule: a document, once built, is shown and
  hidden — never rebuilt.*
- **Modes were rebuilt on every visit.** Visited modes stay mounted, memoised,
  and hidden the same way (`.mode-pane`). *Same rule, one level down.* Before
  the memo, every switch also re-rendered every kept mode.
- **Rows selected on release.** A click waits for mouse-up; Finder and Mail
  select on mouse-down. Rows now select (and open their preview) on the press,
  with no colour transition. *Rule: acknowledge on the press.*
- **Nothing showed a press.** Buttons now take a pressed state at once (darker
  in light, brighter in dark), eased out on release. Rows and segments are
  excluded because their selection already moves on the press.
- **Server-backed actions waited for the server.** Regenerate, retry and
  "Keep As Is" patch the task cache optimistically; the stream overwrites it
  with the real state, and a refusal refetches. *Rule: draw the answer, then
  let the stream correct it.*
- **A layout read inside a state updater.** The segmented control's thumb read
  `offsetLeft` inside `setBox(…)`, which React runs during its next render, so
  the read forced style for the whole document committed under it. Reads now
  happen before the update.
- **Word counts allocated every word.** `wordsIn` split each script into an
  array to read its length, twice per chapter per pass. It now counts in place.

Checked and left alone: server responses (1–4 ms), idle main-thread work (the
infinite status animations are composited), the bundle (v2 only), and the
first visit to a mode or a new video, which still spends ~30–60 ms
(unthrottled) on style and layout — after the press has already painted.

## Audit, part two: WebKit (2026-10-04)

The first audit measured in Chromium, on a video with no narration and no
asset files, and missed what the desktop app actually suffers from. Measured
again in WebKit — the engine behind the desktop window — on SML-6 (80
chapters, all narrated, 160 slides), as the longest frame after each action.
An idle frame here is ~18 ms, so anything near 30 ms is one frame of work.

| Interaction                   | Before   | After    |
| ----------------------------- | -------- | -------- |
| First visit to Chapters       | 2,100 ms | 40–47 ms |
| First visit to Upload         | 86 ms    | 23 ms    |
| Return to a mode              | 73–130 ms | 24–39 ms |
| Switch tab                    | 53–77 ms | 28–35 ms |
| Open a video from the library | 51–86 ms | 26–43 ms |
| Show / hide inspector, console | 95–100 ms | 35–36 ms |

What WebKit does differently, and the rule each fix follows:

- **Native media controls are built in script.** Eighty `<audio controls>`
  cost 357 ms bare and ~2 s in the reader; Chromium does the same in 14 ms.
  Narration is a stand-in bar until pressed, and the press mounts the player
  and starts it. The cut's `<video>` stays mounted (the chapter rail needs its
  duration) but gets `controls` only on play or seek. *Rule: never build a
  platform control nobody has asked for.*
- **`content-visibility: auto` is ignored.** Every chapter was laid out on
  every visit (49 ms vs 10 ms for what is on screen). The reader and the
  pipeline table now draw only items within a screen and a half of the
  viewport (`ui/nearby.tsx`); the rest are spacers at their last height.
  Observer reports from a hidden scroller are ignored, so hiding a mode does
  not tear its rows down. *Rule: draw what can be seen.*
- **`content-visibility: hidden` caches nothing.** A revealed pane is laid out
  again regardless, so hidden modes use `display: none` (scroll positions are
  kept), which also drops the `inert` that cost another 40 ms per switch.

Feedback fixes made alongside:

- The body waits for chapters and tasks as well as the record, so a cold open
  no longer flashes "No chapters yet" over a video that has eighty. A spinner
  fades in only if the wait passes 400 ms.
- The library's selection follows the front tab, and ↑/↓ walk the list (the
  document opens in the preview tab); ⇧ extends.

How to measure: the production build in Playwright's WebKit, a video with
narration, and the longest `requestAnimationFrame` gap after the action.
Chromium numbers alone are not evidence for the desktop app.
