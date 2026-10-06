# yt-studio

Native macOS app that turns a topic into a long-form slideshow video on
YouTube. Go + Wails v3, React + TypeScript + Vite, SQLite.

## Develop

```sh
make dev       # hot reload (Go and frontend), data in var/home
make test      # Go tests
make lint      # go vet, golangci-lint, typecheck, eslint
make generate  # sqlc queries and TypeScript bindings
```

## Release

```sh
make release   # next patch version; BUMP=minor|major or VERSION=x.y.z
make install   # first time only: puts the newest release in /Applications
```

The installed app picks up each release and offers to restart into it.
Data lives in `~/.yt-studio` (or `$YTS_HOME`) and is never touched by an
update.

## Layout

- `main.go` — composition root: builds the backend, registers services, opens the window
- `internal/domain`, `internal/app`, `internal/adapters` — the pipeline
- `internal/studio` — backend wiring
- `internal/services` — what the UI calls (Wails bindings and events)
- `internal/update`, `internal/cmd/release` — releases and self-update
- `frontend/` — the UI; `frontend/bindings` is generated
