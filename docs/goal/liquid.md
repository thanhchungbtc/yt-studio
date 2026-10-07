# Goal: Liquid Glass

Revamp the entire yt-studio UI/UX to the macOS 26/27 Liquid Glass design language.

## Reference — read this first

`/Volumes/data/github/ai-messenger` already has exactly the look and feel we
want. **Match it.** Not "inspired by" — the same design language, the same
material, the same geometry, the same motion, so the two apps look like they
came from the same company.

Study it before writing anything.

## Outcome

The app should look and feel like it shipped with the OS — a native macOS 26/27
window, not a web app in a native frame. Professional, beautiful, obvious to use,
instant to respond.

## Must hold

- 60fps or better on every interaction, 
- Keyboard-first: shortcuts, focus order and focus rings survive the redesign.
- Accessible contrast; honours Reduce Transparency and Reduce Motion.
- `npm run lint` and `npm run build` pass. No new runtime errors.

## Non-goals

- No backend or API changes beyond what the UI genuinely needs.
- Not a feature release: existing functionality stays, it just gets a new body.

