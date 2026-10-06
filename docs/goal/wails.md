# Goal: Wails

Rebuild yt-studio on Wails v3 with the same architecture and best practices as
ai-messenger, so both apps are built and maintained the same way.

## Reference — read this first

`/Volumes/data/github/ai-messenger`. **Match it** — architecture, layout,
conventions, build and release.

## Outcome

- One native Wails v3 macOS app.
- Wails bindings and events replace the HTTP API and event stream.
- `make release` ships a new version; the installed app updates itself.
  `make install` does the first install.
- Existing domain and app logic is kept, not rewritten.

## Must hold

- Everything that works today keeps working, including user data in `~/.yt-studio`.
- Migrate in small steps; the app runs after each one.
- Tests, lint and build pass.

## Non-goals

- Browser access, code signing, remote distribution.
