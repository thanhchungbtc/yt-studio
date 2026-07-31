# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`yt-studio` is a single-operator daemon that automates producing long-form slideshow videos: a blueprint (chapter outline) is generated, then per-chapter narration, stills and clips, then a final render, metadata and upload. One static Go binary (`cmd/server`) contains the HTTP API, the state machine, the scheduler and the embedded React UI. SQLite is the only datastore; `var/` holds everything generated.

## Commands

```bash
make dev          # air hot-reloads the daemon (:8080) + vite dev server (:5173, proxies /api,/events,/assets)
make build        # npm build → go build -o var/bin/yt-studio ./cmd/server (CGO_ENABLED=0)
make run          # build, then serve on 127.0.0.1:8080
make demo         # slow mocks + two seeded videos, one parked at its blueprint gate (DEMO_LATENCY/DEMO_FAILURES/DEMO_CHAPTERS)
make test         # go test ./... -race -count=1 -timeout 900s
make bench        # go test ./... -run '^$' -bench . -benchmem -benchtime 200x  (performance budgets)
make lint         # go vet + golangci-lint + web typecheck + the non-ASCII-SQL gate
make fmt          # gofmt + prettier on web/src
make generate     # sqlc generate, then go build ./...
make clean        # delete var/ and web/dist, restore the placeholder index.html
```

Single test / package:

```bash
go test ./domain/scheduler -run TestDispatchReleasesDependents -race -count=1
go test ./delivery/http -run TestIntegration -race -count=1   # end-to-end, wires the real adapters
go test ./domain/scheduler -run '^$' -bench BenchmarkDispatchDecision -benchmem -benchtime 200x
```

CLI subcommands: `serve` (default), `seed`, `sweep [--apply] [--force]`, `version`. Bootstrap flags are the only non-database config: `--db`, `--assets`, `--resources`, `--listen`, `--log-level`.

Regenerate the frontend's API types against a **running** daemon: `npm --prefix web run gen:api`.

## Layering

`domain/` → `app/` → `delivery/`, with `adapters/` plugged in only at `cmd/server/main.go`. This is compiler-enforced by the `depguard` rules in `.golangci.yml` — read them before adding an import:

- `domain/entity` imports **no** other yt-studio package. IDs are distinct named types (`VideoID`, `ChapterID`, …) so misuse is a compile error.
- `domain/` may not import `adapters/`, `delivery/`, `app/`, `database/sql` or `net/http`.
- `app/` may not import `adapters/` or `database/sql` — only ports.
- `delivery/` may not reach past ports into `adapters/sqlite`.
- No ORM (`gorm`, `ent`) and no cgo SQLite (`mattn/go-sqlite3`); cgo-free is the precondition for the static binary.

`app/` holds one exported use case per file, named after what it does. There is **no dependency container**: every function takes the narrow interfaces it uses as separate parameters, so the signature is the dependency list (`revive`'s `argument-limit` is set to 20 for this reason). `app.TaskRunner` is the sole stateful type there and contains no logic — every branch forwards to a use case.

`domain/repository` and `domain/provider` declare ports split reader/writer so a use case can name only the half it needs. `app/ports.go` declares consumer-defined scheduler ports (one operation each).

## The scheduler (domain/scheduler)

Hand-written and owned outright: an off-the-shelf engine would cost a second process. `dag.go` builds a per-video graph once; `dispatch.go` runs one event-driven loop; `pools.go` is `semaphore.Weighted` per pool; `readyset.go` and `retryqueue.go` are the in-memory dispatch structures. Key invariants:

- **The task table is the state.** An unscheduled successor consumes nothing; the daemon may restart while a gate is open. `DepsRemaining` is persisted so recovery is exact rather than recomputed.
- **Two-phase graph construction.** A video is enqueued with a `HeadSpec` — the blueprint node alone — because the chapter count is the blueprint's *output*, not its input. The per-chapter tail is spliced on at blueprint *acceptance*: `ApproveGate` when the blueprint gate is on, and `TaskRunner.runBlueprint` itself when it is off (see `app.ExpandVideoGraph`). Anything that fails before expansion leaves a one-node DAG whose blueprint can simply be re-run.
- **Generations.** Each node has a non-persisted generation counter; a dispatch carries the generation it started under and its completion is discarded on mismatch, so a task in flight when its input was retried cannot land an answer to the old question.
- **Polling is forbidden as a primary mechanism.** `Config.SafetyInterval` (≥30s) is a consistency net, nothing more.
- **`Stale` is a flag, not a state.** The useful combination is `succeeded` *and* stale: the artifact exists and may still be correct, so it waits for the operator to re-run or accept it.
- Cycle rejection (`ErrCycle`) is checked, not assumed.

DAG shape per video: `blueprint` → `prime_image_prompts` and each chapter's `script`; `prime → image_prompts[i]`; `script[i] → tts[i]`; `image_prompts[i] → image[i][j]`; `tts[i]` and every `image[i][j]` → `clip[i]`; all clips → `concat` → `metadata` → `thumbnail` → `upload`. Pools: LLM (blueprint, prime, script, metadata), cache (image_prompts), tts, image, compose (clip, concat, thumbnail), upload. The upload gate rides on `thumbnail`, the last node before the upload, so what the operator approves is the whole listing rather than its text alone.

## Persistence (adapters/sqlite)

One `Store` type over two pools: a multi-connection read pool and a **single-connection write pool fed by one writer goroutine** (`Store.Run`). Every write goes through `do`/`doTx`, which serialises rather than retries SQLite write-lock contention. Every statement is prepared at open. `main.go` gives the writer its own cancel scope so it outlives the request context long enough to flush the scheduler's final transitions.

- Queries live in `adapters/sqlite/queries/*.sql`, schema in `migrations/` (goose, numbered `0000N_name.sql`). Run `make generate` after touching either; `sqlcgen/` is generated and lint-excluded.
- **SQL files must be pure ASCII** — sqlc's SQLite dialect truncates generated statements on multi-byte characters. `make lint` fails on violations.
- Timestamps are INTEGER unix nanoseconds (UTC).

## Configuration is the settings table

`domain/entity/setting.go` defines every key (`pool.*.limit`, `gate.*.enabled`, `provider.*`, `task.retry_*`, `sse.coalesce_ms`, `mock.*`, …) with its defaults and validation. `domain/service.Settings` loads and validates the whole table at startup — an unparsable row is a startup error — and typed getters afterwards cannot fail. `Set` validates, persists and refreshes the cache, so edits apply to the next task rather than the next restart. Anything genuinely needed before the database opens is a CLI flag, and nothing else is.

## Providers

`domain/provider` declares six ports (LLM, TTS, image, composer, thumbnail, uploader). The thumbnail is its own port rather than a third composer method: what it renders is a listing artifact, and the backend that draws it is the one most likely to be swapped independently of the video encoder. **A provider call never spans more than one unit of work** — no multi-chapter calls, no fan-out inside a provider; orchestration belongs to the daemon. The one deliberate exception is image prompting: `prime_image_prompts` produces one batch behind the interface, and the N per-chapter `image_prompts` tasks stay individually retryable cache reads.

`adapters/registry` resolves the backend named by the settings row **per call**. An unregistered name is an error, never a silent fallback. Backends are registered in `main.go` before settings load, so a bad row fails at startup. Currently wired: `mock` (all ports), `sample` (tts, image), `ffmpeg` (composer). The thumbnail port has only the mock so far: the real renderer, and the rule for which still it draws on, are still open. `provider.ErrUnavailable` means "cannot run until someone changes something" — `app.classify` maps it to a non-retryable failure.

`adapters/ninerouter` is an in-progress LLM backend for a 9router gateway; it is **not yet registered in `main.go`**. Its prompts are `text/template` files embedded from `prompts/*.tmpl` so behaviour is reproducible from the binary. Two gateway quirks are load-bearing and documented in its package comment: `stream` must be explicitly `false`, and `response_format` is silently ignored so the output contract goes in the prompt.

Assets are content-addressed by sha256 (`adapters/assetstore`), streamed with a pooled buffer so a three-hour render is never read into memory. Ownership rows say which row may reclaim a file; `app.RepairAssetOwnership` runs unconditionally before `serve` and before `sweep`, because a file reachable only through a chapter's id list has no owning row until the repair gives it one and would otherwise read as garbage.

## Delivery and the frontend

`delivery/http` is thin: huma v2 over chi produces the typed API and OpenAPI at `/api/openapi`, docs at `/api/docs`. `Deps` is a wiring record only — handlers take narrow interfaces as parameters and nothing holds a reference to it. Lists are always `make()`d, never nil (`huma.DefaultArrayNullable = false`). `/events` is the SSE stream; `adapters/eventbus` coalesces per video within a window so a 50-chapter render does not emit hundreds of events per second. `spa.go` serves the embedded UI on 404.

`web/` is React 19 + TanStack Router/Query + Tailwind 4, built by vite into `web/dist` and `go:embed`ed by `web/embed.go`. A placeholder `index.html` is committed so `go build` works on a fresh clone. Vite's `assetsDir` is `app`, not `assets` — `/assets/{id}` belongs to the daemon's content-addressed artifact route. `web/src/lib/schema.d.ts` is generated from the OpenAPI document; `schema-contract.ts` type-asserts the hand-written types in `types.ts` against it, so a Go DTO change breaks the web typecheck.

## Conventions that the linters enforce

- **Every switch over a typed constant must be exhaustive** (`exhaustive`, `default-signifies-exhaustive`). Adding a `TaskKind`, `TaskState`, `VideoState` or `Pool` means the compiler/linter points at every site.
- `entity.TaskOutcome` is a sealed sum type (unexported marker method): `Success`, `Failed{Retryable}`, `AwaitingApproval`. Type switches over it end in a `default` that panics, and table-driven tests iterate `AllTaskOutcomes()` against every site.
- Errors wrap sentinels so layers above can classify without knowing details. `delivery/http/errors.go:mapError` is the single translation table (validation sentinels → 422, conflict sentinels → 409, not-found → 404, `scheduler.ErrSchedulerClosed` → 503); `app.classify` is the equivalent for task outcomes, where the only question is whether another attempt could land differently. `app.ErrBlueprintOffTarget` is deliberately *not* an `ErrValidation`: the input was fine, the roll was not, so it is retried.
- Comments here explain *why*, often naming the failure mode avoided. Match that register; don't add restating-the-code comments.
- `errcheck` with `check-type-assertions`, `nilerr`, `contextcheck`, `bodyclose`, `rowserrcheck` are all merge gates. Tests are exempted from a specific list (see `exclusions` in `.golangci.yml`) — production code is not.
- Benchmarks in `domain/scheduler/bench_test.go` are budgets, not measurements: some fail outright on an absolute miss, and `TestDispatchDecisionIsAllocationFree` asserts zero allocations on the dispatch decision path.
