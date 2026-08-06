# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`yt-studio` is a single-operator server that automates producing long-form slideshow videos: a blueprint (chapter outline) is generated, then per-chapter narration, slides and clips, then a final render, metadata and upload. One static Go binary (`cmd/server`) contains the HTTP API, the state machine, the scheduler and the embedded React UI. SQLite is the only datastore; `var/` holds everything generated, plus `var/resources` — operator-supplied media that is untracked like the rest but is input, not output, and so survives `make clean`.

"The server" throughout this document means that one process. It is more than an API: the scheduler dispatches tasks off its own event loop and the SQLite writer drains its queue, so it keeps working with every browser closed.

## Commands

```bash
make dev          # air hot-reloads the server (:8080) + the vite dev server (:5173, proxies /api,/events,/assets)
make build        # npm build → go build -o var/bin/yt-studio ./cmd/server (CGO_ENABLED=0)
make run          # build, then serve on 127.0.0.1:8080
make test         # go test ./... -race -count=1 -timeout 900s
make bench        # go test ./... -run '^$' -bench . -benchmem -benchtime 200x  (performance budgets)
make lint         # go vet + golangci-lint + web typecheck + the non-ASCII-SQL gate
make fmt          # gofmt + prettier on web/src
make generate     # sqlc generate, then go build ./...
make clean        # delete var/* except var/resources, and web/dist; restore the placeholder index.html
```

Single test / package:

```bash
go test ./domain/scheduler -run TestDispatchReleasesDependents -race -count=1
go test ./delivery/http -run TestIntegration -race -count=1   # end-to-end, wires the real adapters
go test ./domain/scheduler -run '^$' -bench BenchmarkDispatchDecision -benchmem -benchtime 200x
```

CLI subcommands: `serve` (default), `seed`, `sweep [--apply] [--force]`, `version`. Bootstrap flags are the only non-database config: `--db`, `--assets`, `--resources`, `--listen`, `--log-level`.

Regenerate the frontend's API types against a **running** server: `npm --prefix web run gen:api`.

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

- **The task table is the state.** An unscheduled successor consumes nothing; the server may restart while a gate is open. `DepsRemaining` is persisted so recovery is exact rather than recomputed.
- **Two-phase graph construction.** A video is enqueued with a `HeadSpec` — the blueprint node alone — because the chapter count is the blueprint's *output*, not its input. The per-chapter tail is spliced on at blueprint *acceptance*: `ApproveGate` when the blueprint gate is on, and `TaskRunner.runBlueprint` itself when it is off (see `app.ExpandVideoGraph`). Anything that fails before expansion leaves a one-node DAG whose blueprint can simply be re-run.
- **Generations.** Each node has a non-persisted generation counter; a dispatch carries the generation it started under and its completion is discarded on mismatch, so a task in flight when its input was retried cannot land an answer to the old question.
- **Polling is forbidden as a primary mechanism.** `Config.SafetyInterval` (≥30s) is a consistency net, nothing more.
- **`Stale` is a flag, not a state.** The useful combination is `succeeded` *and* stale: the artifact exists and may still be correct, so it waits for the operator to re-run or accept it.
- Cycle rejection (`ErrCycle`) is checked, not assumed.

DAG shape per video: `blueprint` → `prime_slide_prompts` and each chapter's `script`; `prime → slide_prompts[i]`; `script[i] → tts[i]`; `slide_prompts[i] → slide[i][j]`; `tts[i]` and every `slide[i][j]` → `clip[i]`; all clips → `concat` → `metadata` → `thumbnail_plan` → `thumbnail_icon[k]` → `thumbnail` → `upload`. Pools: LLM (blueprint, prime, script, metadata, thumbnail_plan), cache (slide_prompts), tts, image (slide, thumbnail_icon), compose (clip, concat, thumbnail), upload. The upload gate rides on `thumbnail`, the last node before the upload, so what the operator approves is the whole listing rather than its text alone.

A **slide** is one chapter image — one panel of the slideshow. It was called a *still* and, in the schema, an *image*; one concept under two names, one of which is also the commonest adverb in English and made the codebase ungreppable. `slide` says the role rather than the file format, and it distinguishes a chapter's artwork from a thumbnail icon, which `image` could not. Three things keep the word `image` on purpose: `entity.AssetKindImage`, because an asset kind pins an extension and a MIME type and icons and thumbnails are images too; `entity.PoolImage` / `pool.image.limit`, because that pool is image-*generation capacity* and slides and thumbnail icons share it; and the prompts in `adapters/provider/ninerouter/prompts/`, because what the model on the other end draws really is an image.

The thumbnail branch is its own artifact, independent of the chapter slides: `thumbnail_plan` writes one caption and one icon prompt per cell, `thumbnail_icon[k]` draws each cell, `thumbnail` composes them under the headline. Its width is `videos.thumbnail_cells`, fixed at creation — one icon task exists per cell from expansion onward and the graph can never grow, so the plan's cell count is a contract (short → `ErrThumbnailPlanOffTarget`, retried; long → truncated) rather than a target. The shared style clause lives in `thumbnail.icon.style` and is appended when an icon is generated, so restyling the grid re-runs a dozen cheap generations instead of re-rolling the captions. Icons land in `videos.thumbnail_icon_ids_json`, sized when the plan is written and written by index with `json_set`, because they finish in whatever order the image pool returns them.

## Persistence (adapters/sqlite)

One `Store` type over two pools: a multi-connection read pool and a **single-connection write pool fed by one writer goroutine** (`Store.Run`). Every write goes through `do`/`doTx`, which serialises rather than retries SQLite write-lock contention. Every statement is prepared at open. `main.go` gives the writer its own cancel scope so it outlives the request context long enough to flush the scheduler's final transitions.

- Queries live in `adapters/sqlite/queries/*.sql`, schema in `migrations/` (goose, numbered `0000N_name.sql`). Run `make generate` after touching either; `sqlcgen/` is generated and lint-excluded.
- There is one migration, and until the first release there should stay one: nothing has shipped, so an incremental history describes upgrades no installed database will ever perform. Fold a schema change into `00001_init.sql` and delete your local database rather than adding `00002_`. The moment a build ships this stops being true, and the next change is a migration like any other.
- **SQL files must be pure ASCII** — sqlc's SQLite dialect truncates generated statements on multi-byte characters. `make lint` fails on violations.
- Timestamps are INTEGER unix nanoseconds (UTC).

## Configuration is the settings table

`domain/entity/setting.go` defines every key (`pool.*.limit`, `gate.*.enabled`, `provider.*`, `task.retry_*`, `sse.coalesce_ms`, `mock.*`, …) with its defaults and validation. `domain/service.Settings` loads and validates the whole table at startup — an unparsable row is a startup error — and typed getters afterwards cannot fail. `Set` validates, persists and refreshes the cache, so edits apply to the next task rather than the next restart. Anything genuinely needed before the database opens is a CLI flag, and nothing else is.

The rows are grouped into the sections the settings screen shows, one section mounted at a time, and a group is **the task the operator is doing** rather than the subsystem that reads the row: `pools`, `gates`, `providers`, then the pipeline in the order it runs — `writing`, `narration`, `slides`, `thumbnail` — then `video` (what a new video is created with, read once and then frozen), `retries` and `server`. Three consequences are load-bearing. `providers` holds the seven port rows plus `upload.dry_run`, which is an argument to the uploader rather than a rail of its own — the port says who publishes, the flag says whether the publish is real — and nothing else, so the group grows when `domain/provider` gains a port rather than every time a backend is registered; a backend's own knobs live with the stage they shape and carry `Setting.Backend`, which is stamped at load like `Options` and is what lets the screen mark `runware.width` idle while the slide port points at `mock`. The mock uploader records every receipt as dry whatever the flag says, since it runs none of the publishing code and a receipt claiming otherwise would be the one row in the database that lies about reaching YouTube. And the order rows appear in is `DefaultSettings`' own, not the key order — `SettingOrder` is what `service.Settings.All` sorts by, since "voice, language, speed" is how narration is thought about and "chunk, chunk, language, speed, voice" is only how the names happen to sort.

A value is `int`, `bool`, `string` or `float`. `Setting.Min`/`Max` are `float64` so one pair of bounds serves both numeric types — a second pair for floats alone would be one more thing to keep in step — and they are persisted as SQLite `REAL`. Float validation rejects NaN and infinity explicitly, because NaN fails every comparison and would otherwise slip through a bounds check written as "outside the range".

Those flags all carry an `env:` tag, and `cmd/server/env.go` loads `.env` from the working directory into the environment before `kong.Parse` — kong reads the tags at parse time, so the load has to happen in `main`, not in a command's `Run`. Mechanism and policy are split: `cmd/server/internal/dotenv` parses and applies a file and knows nothing about yt-studio, while `env.go` decides which file, what a missing one means, and what to log. It is under `cmd/server/internal` rather than a top-level `internal/` because `Load` mutates the process environment — that is the command's business, and from there a use case or an adapter calling it does not compile. A variable already set always wins over the file, a missing default file is silence but a missing `YTS_ENV_FILE` is an error, and a malformed line fails the boot without applying any of it. The format is the small one: no `${VAR}` interpolation, no inline comments (a secret may contain `#`), a key set twice is an error. `.env.example` is committed and is the inventory; `.env` is ignored. **It is not a second configuration tier** — it supplies the bootstrap flags and nothing that has a settings row.

## Providers

`domain/provider` declares seven ports (LLM, TTS, slide, composer, thumbnail-icon, thumbnail, uploader). The thumbnail is its own port rather than a third composer method: what it renders is a listing artifact, and the backend that draws it is the one most likely to be swapped independently of the video encoder. Icons are a port of their own for the same reason in reverse — the same capability as a slide, but selected apart from it, and a `SlideRequest` here would be two thirds empty. **A provider call never spans more than one unit of work** — no multi-chapter calls, no fan-out inside a provider; orchestration belongs to the server. The one deliberate exception is slide prompting: `prime_slide_prompts` produces one batch behind the interface, and the N per-chapter `slide_prompts` tasks stay individually retryable cache reads.

`cmd/server/internal/registry` resolves the backend named by the settings row **per call**. An unregistered name is an error, never a silent fallback. Backends are registered in `main.go` before settings load, so a bad row fails at startup. Currently wired: `mock` (all ports), `sample` (tts, slide, thumbnail icons), `ffmpeg` (composer), `builtin` (thumbnail), `9router` (llm), `runware` (slide, thumbnail icons), `xtts` (tts).
`adapters/provider/runware` is the paid image backend: one `imageInference` POST per image, then a download, because the API answers with a URL rather than bytes. Slides and thumbnail icons are separate types over one client so the two ports can be pointed at different backends. It asks for PNG rather than JPEG because `entity.AssetKind` pins the extension and MIME per kind. Sizes come from `runware.width`/`runware.height` (slides, defaulting to the composer's 1344x768) and from the request (icons, which are square by the port's definition); the API's own size grid is deliberately not duplicated, since a stale copy of it would refuse a size the checkpoint would have drawn. The key is `--runware-key`/`RUNWARE_KEY`, a flag rather than a row because the settings table is served to the UI. `Check` reads the configuration and makes no request: the cheapest probe still costs a generation.

`adapters/provider/sample` reads `<resources>/sample/`: `*.wav` narration, `img*.jpg` slides, `icon*.jpg` thumbnail tiles. The icons are optional — they arrived after the rest, so a library without them still serves the other two and only `Icons()` reports the absence. They are also the one sample that is transformed on the way in: an icon is square by the port's definition and the samples are 16:9, so they are centre-cropped and scaled to the requested size rather than reaching the renderer stretched.

`adapters/provider/thumbnail` is the `builtin` renderer: pure Go over `x/image/font`, drawing the headline and the icon grid onto `resources/background.jpg`. Layout constants live in the package rather than in settings — the backend that exists to be tweaked without a rebuild is the HTML one still to come, and two places to change a layout is one too many. Only the typeface (`thumbnail.font`) and the row count (`thumbnail.grid.rows`) are rows. Two things are load-bearing and were measured rather than guessed: PNG **default** compression, because best costs seven times the CPU to save six percent on a photographic frame, and one caption size for the whole grid, because per-tile fitting makes a dozen tiles read as a dozen unrelated pictures. `provider.ErrUnavailable` means "cannot run until someone changes something" — `app.classify` maps it to a non-retryable failure.

`adapters/provider/tts` is the `xtts` narration backend, for an AllTalk/XTTS server. A chapter is split on sentence boundaries into chunks of at least `xtts.chunk.min_chars`, each chunk synthesised on its own and the WAVs joined with `xtts.chunk.silence_ms` of silence between them — the splitting is not an optimisation, since XTTS degrades on long inputs and a chapter is thousands of words. Every chunk costs two requests, because the server answers a generation with a URL rather than bytes. `--xtts-url` is the **server root**: the endpoints are appended, and a value carrying `/api/tts-generate` is rejected at startup, because the Python this replaces configured the full endpoint and pasting that value across would double the path. The chunking, WAV concatenation and tail trim are ported one for one from that Python, so the output keeps sounding the same. Only the chunking pair is this backend's own and named for it: `tts.voice`, `tts.language` and `tts.speed` cross the port on the request instead, because a voice belongs to the video being narrated rather than to the server speaking it — global today, a channel's the day one wants its own. `tts.speed` is the settings table's one `float` — the useful range is 0.5..2.0 and the interesting steps inside it are tenths.

`adapters/provider/ninerouter` is the LLM backend for a 9router gateway. Its prompts are `text/template` files embedded from `prompts/*.tmpl` so behaviour is reproducible from the binary. Two gateway quirks are load-bearing and documented in its package comment: `stream` must be explicitly `false`, and `response_format` is silently ignored so the output contract goes in the prompt.

Adapters are grouped by **whether the registry selects them by name**: `adapters/provider/*` is exactly the set a settings row can name, one directory per backend, mirroring the ports in `domain/provider`. Inside a backend, **the file is named after the port it implements** — `llm.go`, `xtts.go`, `slide.go`, `icon.go`, `composer.go`, `thumbnail.go`, `uploader.go` — and holds that port's type, its `var _ provider.X` assertion and its constructor. A large implementation may spill its methods into sibling files named after the operation (`ninerouter`'s `blueprint.go`/`script.go`, `ffmpeg`'s `clip.go`/`concat.go`); what stays fixed is that finding an implementation is a directory listing, never a search. `assetstore`, `eventbus` and `sqlite` sit at the root because nothing chooses them — they are constructed once in `main.go` and never resolved from a row. That is the line, not the capability served: `sample` answers three ports and `runware` two, so grouping by capability put multi-port backends nowhere and split the mock for a reason that applied to half the tree.

The tree stays under `adapters/` rather than a top-level `provider/` because the `depguard` rules key on the literal prefix `github.com/tbui/yt-studio/adapters` — a backend outside it would be importable from `domain/` and `app/` with nothing to catch it. A second directory named `provider` would also read badly from inside one: `provider.SlideGenerator` refers to the ports, not to the neighbours.

The mock is the one backend that nests further: one name in the settings table, two packages, because a backend that implements every port has no single capability to file it under. `main.go` imports the halves as `llmmock` and `mediamock`, since two packages named `mock` cannot both go unaliased. They share no code: what a mock needs beyond generating its bytes is `seedOf` and `deterministic`, eleven lines each half keeps privately, because deriving the same output from the same request is what lets content addressing dedupe a re-run. A mock returns as soon as its bytes are written.

Assets are content-addressed by sha256 (`adapters/assetstore`), streamed with a pooled buffer so a three-hour render is never read into memory. Ownership rows say which row may reclaim a file; `app.RepairAssetOwnership` runs unconditionally before `serve` and before `sweep`, because a file reachable only through a chapter's id list has no owning row until the repair gives it one and would otherwise read as garbage.

## Delivery and the frontend

`delivery/http` is thin: huma v2 over chi produces the typed API and OpenAPI at `/api/openapi`, docs at `/api/docs`. `Deps` is a wiring record only — handlers take narrow interfaces as parameters and nothing holds a reference to it. Lists are always `make()`d, never nil (`huma.DefaultArrayNullable = false`). `/events` is the SSE stream; `adapters/eventbus` coalesces per video within a window so a 50-chapter render does not emit hundreds of events per second. `spa.go` serves the embedded UI on 404.

`web/` is React 19 + TanStack Router/Query + Tailwind 4, built by vite into `web/dist` and `go:embed`ed by `web/embed.go`. A placeholder `index.html` is committed so `go build` works on a fresh clone. Vite's `assetsDir` is `app`, not `assets` — `/assets/{id}` belongs to the server's content-addressed artifact route. `web/src/lib/schema.d.ts` is generated from the OpenAPI document; `schema-contract.ts` type-asserts the hand-written types in `types.ts` against it, so a Go DTO change breaks the web typecheck.

## Conventions that the linters enforce

- **Every switch over a typed constant must be exhaustive** (`exhaustive`, `default-signifies-exhaustive`). Adding a `TaskKind`, `TaskState`, `VideoState` or `Pool` means the compiler/linter points at every site.
- `entity.TaskOutcome` is a sealed sum type (unexported marker method): `Success`, `Failed{Retryable}`, `AwaitingApproval`. Type switches over it end in a `default` that panics, and table-driven tests iterate `AllTaskOutcomes()` against every site.
- Errors wrap sentinels so layers above can classify without knowing details. `delivery/http/errors.go:mapError` is the single translation table (validation sentinels → 422, conflict sentinels → 409, not-found → 404, `scheduler.ErrSchedulerClosed` → 503); `app.classify` is the equivalent for task outcomes, where the only question is whether another attempt could land differently. `app.ErrBlueprintOffTarget` is deliberately *not* an `ErrValidation`: the input was fine, the roll was not, so it is retried.
- Comments here explain *why*, often naming the failure mode avoided. Match that register; don't add restating-the-code comments.
- `errcheck` with `check-type-assertions`, `nilerr`, `contextcheck`, `bodyclose`, `rowserrcheck` are all merge gates. Tests are exempted from a specific list (see `exclusions` in `.golangci.yml`) — production code is not.
- Benchmarks in `domain/scheduler/bench_test.go` are budgets, not measurements: some fail outright on an absolute miss, and `TestDispatchDecisionIsAllocationFree` asserts zero allocations on the dispatch decision path.
