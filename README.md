# yt-studio

Local automation for long-form slideshow videos: one static binary that owns a
scheduler, a state machine, an HTTP API and an embedded operator UI.

A typical video is ~3 hours and 50+ chapters. Each chapter contributes a
narrated audio track and N stills; chapters are composed into clips,
concatenated into a final render, given metadata and uploaded. Every generative
backend sits behind an interface — this version ships local, deterministic mocks
that produce genuinely valid PNG, WAV and MP4 files.

The design is specified in [`goal.md`](goal.md). This README covers how to run
it and where things live.

---

## Quick start

```bash
make build          # builds the React app and embeds it in the binary
make run            # ...and serves it on http://127.0.0.1:8080
```

The database and the asset store are created on first run and seeded with two
demonstration channels and a complete settings table. There is nothing else to
install: no runtime, no sidecar, no database server.

```
make dev       hot reload for the daemon and the web UI together
make build     the single binary, web UI embedded  → var/bin/yt-studio
make run       build, then serve
make test      every test, with the race detector
make bench     the performance budgets
make lint      go vet, golangci-lint, and the web typecheck
make fmt       format Go and TypeScript
make generate  regenerate the sqlc query layer
make clean     delete everything generated
```

**Everything generated lives under `var/`** — the binary (`var/bin/yt-studio`),
the database (`var/yt-studio.db`), the content-addressed asset store
(`var/assets/`) and the hot-reload scratch directory. One line in `.gitignore`,
one directory to delete. Override with `--db`, `--assets` or `YTS_DB`,
`YTS_ASSETS`.

### Driving it from the shell

The UI has no privileged access — everything it can do is on the same API.

```bash
# create a video and enqueue its DAG
curl -sX POST localhost:8080/api/videos -H 'content-type: application/json' \
  -d '{"channel":"deep-sleep-stories","title":"The Long Winter of the Harbour",
       "topic":"a northern port town","chapterCount":50,"imagesPerChapter":2,"start":true}'

# the pipeline pauses after the blueprint
curl -sX POST localhost:8080/api/videos/DSS-1/approve -d '{"gate":"blueprint"}'

# ...and again before upload
curl -sX POST localhost:8080/api/videos/DSS-1/approve -d '{"gate":"upload"}'

curl -s localhost:8080/api/scheduler | jq   # pool utilisation and queue depth
```

Interactive API documentation is at `/api/docs`; the OpenAPI document itself is
at `/api/openapi.json` and is what the browser client's types are generated
from.

---

## How it fits together

```
cmd/server        wiring: config → adapters → app → delivery → serve
domain/entity     the model. no imports outside itself
domain/repository persistence ports, split reader/writer
domain/provider   the five generative ports plus the asset store
domain/service    domain services (typed, cached settings)
domain/scheduler  the DAG, the pools, the ready set, the dispatch loop
app/              use cases — one exported function per file
adapters/         sqlite, assetstore, eventbus, mock_provider
delivery/http     typed huma handlers on chi; SSE; asset streaming; the SPA
web/              React 19 + TypeScript, embedded via embed.FS
```

Layering is enforced by `depguard` in CI, not by convention: the domain cannot
import an adapter, a use case cannot import a concrete adapter, and delivery
cannot reach past the ports into SQLite.

### The pipeline

```
                         blueprint  (LLM)  ── gate ──┐
                              │                      │
              ┌───────────────┴────────────────┐     │
    (needs script)                    (needs blueprint ONLY)
              ▼                                ▼
        script_i  (LLM ×N)          prime_image_prompts  (LLM ×1)
              │                                ▼
              ▼                        prompts_i  (×N, cache reads)
         tts_i  (TTS ×N)                       ▼
              │                        images_i  (IMG ×M·N)
              └────────────────┬───────────────┘
                               ▼
                        clip_i  (compose ×N)
                               ▼
                     concat → final  (compose ×1)
                               ▼
              metadata (LLM) ── gate ── upload
```

Image prompts depend on the **blueprint alone**, never on a chapter script.
That is what gives the graph two independent branches and starts the image
pipeline — the longest pole — early. A property test asserts it stays that way.

For 50 chapters and 2 stills each that is 305 tasks. Every task starts the
moment its own dependencies are met; there are no stage barriers anywhere.

### The scheduler

Owning it is the point. It is a single goroutine over an in-memory ready set,
with SQLite as durable backing:

- **Event-driven.** A completed task signals its dependents directly. The task
  table is never rescanned to find work. A 30-second in-memory sweep exists only
  as a safety net.
- **Global pools.** `semaphore.Weighted`, one slot per task, held for the
  duration of the provider call. Video A's chapter 3 competes with video B's
  chapter 40. Limits change at runtime by moving ballast inside the semaphore.
- **Resumable.** Tasks and edges are persisted, so a crash 45 minutes in
  rebuilds the exact graph and continues. A task caught mid-flight is simply
  re-run — every step is idempotent and content-addressed.
- **Gated.** A gate is a row update. An open gate holds no resources and the
  daemon may restart freely while it is open.

Measured on an M1 Max:

| Budget                                 | Target       | Measured             |
| -------------------------------------- | ------------ | -------------------- |
| Dispatch decision                      | < 1 ms       | **47 ns, 0 allocs**  |
| Task transition committed to SQLite    | < 5 ms       | batched, one tx      |
| API response, p99                      | < 50 ms      | **305 µs**           |
| Server cold start to serving           | < 100 ms     | **8.8 ms**           |
| Migrations on an existing database     | < 50 ms      | **2.4 ms**           |
| Server RSS, 305 tasks in flight        | < 150 MB     | **49 MB**            |
| Server idle CPU                        | < 0.5 %      | **0.3 %**            |
| Production bundle, gzipped             | < 250 KB     | **167 KB**           |

A 50-chapter, 305-task render finishes against the mocks in about 8 seconds,
and `SIGKILL` partway through resumes on restart and completes with every
chapter intact.

### Storage

`modernc.org/sqlite` (pure Go — this is what makes the static cross-compiled
binary possible), WAL, `synchronous=NORMAL`, an explicit busy timeout, and every
statement prepared once at open. Writes are serialised through **one writer
goroutine**; reads use a separate pool. A burst of task completions commits as a
single transaction.

Queries are `sqlc`-generated from SQL we wrote. A test asserts with
`EXPLAIN QUERY PLAN` that no scheduler query falls back to a full table scan.

Generated files land in a content-addressed store: the sha256 **is** the id, so
re-running a task that produces identical bytes is a no-op, and asset URLs can
be cached forever.

---

## Standards

Every one of these is a CI gate, not an aspiration.

- `go test -race` over the whole tree, including property-based tests
  (`pgregory.net/rapid`) that assert the scheduler never exceeds a pool budget,
  never deadlocks, never starts a task before its dependencies and never loses a
  task.
- Deterministic golden tests: the same inputs produce the same task sequence and
  byte-identical artifacts.
- `golangci-lint` with `errcheck`, `exhaustive`, `nilerr`, `bodyclose`, govet's
  `nilness`, and `depguard` for layering.
- `benchstat` gate: no benchmark may regress more than 5 % against the base.
- The frontend typechecks against types generated from the daemon's OpenAPI
  document, so a drifted client type fails the build.

---

## Notes for the next person

A few things that are not obvious from the design document:

- **SQL files must stay ASCII.** sqlc's SQLite dialect miscomputes query spans
  when a `.sql` file contains multi-byte characters and silently truncates the
  generated statements — `RETURNING video_seq` became `RETURNING video_se`.
  `make check-sql-ascii` guards it. For the same reason the video-sequence
  counter reads back inside its transaction instead of using `RETURNING`.
- **The web bundle lives under `/app`, not `/assets`.** `/assets/{hash}` is the
  artifact route, and a Vite filename would have been read as a content address.
- **Coalescing needs both halves.** `singleflight` deduplicates only calls that
  overlap in time; the per-chapter prompt tasks do not all overlap, so the mock
  LLM keeps a cache behind the singleflight group. `Forget` drops it, which is
  what makes a chapter retry regenerate rather than replay.
- **Two pools beyond the four in the design.** `cache` (default 32) carries the
  per-chapter prompt reads, which is what the priming task exists to enable, and
  `upload` (default 1) keeps publishing off the compose pool. Both are ordinary
  settings rows.
- **The mock MP4 is real.** `ffprobe` reports a PNG video track and a 16-bit PCM
  audio track, and `ffmpeg` decodes it. Concatenation is a genuine stream copy of
  byte ranges, so the property that has to survive into the real composer is
  already exercised.
- `npm audit` reports advisories in `openapi-typescript`'s transitive tree. It is
  a dev-only code generator pointed at our own local document and none of it
  reaches the shipped bundle.
- **`web/dist/index.html` is a committed placeholder** (sourced from
  `web/placeholder.html`) so `go build ./...` works on a fresh clone before npm
  has ever run — `go:embed all:dist` needs the directory to exist. `make build`
  replaces it; `make clean` puts it back.

## Deferred

Real LLM, TTS and image backends; grammar-constrained structured output; real
composition via `os/exec` with explicit argv; resumable YouTube upload. The
interfaces are shaped for all four — adding one is a single type implementing a
single interface plus a settings row to select it.
