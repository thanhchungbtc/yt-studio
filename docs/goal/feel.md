# Goal: Same Feel

yt-studio should look, feel and behave exactly like ai-messenger: same family,
same polish, same fluid, professional, easy-to-use experience.

## Reference — read this first

`/Volumes/data/github/ai-messenger`. **Match it.** Read-only: nothing in
ai-messenger changes.

## Outcome

- Same UI kit: ai-messenger's components, styles and motion, brought into
  yt-studio as one unit, so it can be re-synced later.
- Same interaction model: command palette (⌘K), every action a command,
  configurable shortcuts, toasts, context menus, tooltips with shortcuts.
- Revamp every screen, panel and dialog where ai-messenger has an equivalent
  (settings, sidebars, editors, popovers, status bar…) to work and look the
  way ai-messenger's does.
- Screens unique to yt-studio (pipeline, chapters, upload, thumbnail…) are
  redesigned in the same language, as if ai-messenger had built them: same
  layout rhythm and density, and the same empty, loading, error and confirm
  patterns.

## Must hold

- 60fps everywhere; instant feedback.
- Everything that works today keeps working.
- Reduce Transparency / Reduce Motion respected.
- Small steps; the app runs after each one. Tests, lint and build pass.

## Non-goals

- Changing ai-messenger.
- Backend changes beyond what the UI needs.
