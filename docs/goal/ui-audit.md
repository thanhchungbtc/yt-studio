# UI/UX audit

## Goal

Find and fix every UI/UX bug in yt-studio until every screen feels as polished as ai-messenger.

## What to check

- Layout: nothing clipped, shifted, overlapping or scrolled out of place; panes and cards keep their shape at any window size.
- Every screen, panel, dialog, menu and state (empty, loading, error, long text, many items).
- Interactions: clicks, keyboard, focus, selection, scrolling, drag, resize, open/close.
- Consistency with ai-messenger: spacing, type, colour, motion, wording.
- Light and dark, all window materials, Reduce Motion, Reduce Transparency, interface sizes.
- Speed: 60 fps, no jank, no layout jumps.

## How

- Reproduce first, then fix the cause, not the symptom.
- Verify each fix in the running app.
- Small steps; tests, lint and build pass.

## Status

Done, verified in the running app.

- Fixed: editor card scrolled out of place; editor clipped after a tab switch while the console opens.
- Fixed: chapter outline highlighting the wrong chapter after a jump.
- Fixed: status colours inconsistent between sidebar, header and channel page.
- Fixed: settings log level as free text, numbers without units.
- Fixed: tabs and rows cut off without a fade or full-title hover at narrow widths.
- Checked: every screen, light and dark, Solid material, Reduce Motion, 960×600, 60 fps.

## Open

- The channel editor is still a placeholder (a new feature, out of scope).

## Non-goals

- Changing ai-messenger.
- New features.
