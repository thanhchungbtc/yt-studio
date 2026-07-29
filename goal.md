# yt-studio — Goal & Architecture

## 1. What we're building

An automation system that produces **long-form slideshow-style YouTube videos** end to end, across **multiple channels**, using **entirely local AI services**.

A typical video is **~3 hours long with 50+ chapters**. Each chapter contributes a narrated audio track and 2 still images (number configurable); chapters are composed into clips and concatenated into the final video, which is then given metadata and uploaded.

### Non-goals

- Not multi-tenant. Single operator, single box.
- Not cloud-hosted AI. Everything runs on local hardware.

---

## 2. Core principles

1. **The core owns all logic.** Domain model, DAG definition, state machine, and scheduling policy live in dependency-free Go packages. **We own the scheduler outright** — every third-party workflow engine costs a server process we don't want (§10).
2. **Everything is behind an interface.** LLM, TTS, image generation, composition, and upload are Go interfaces.
3. **Event-driven pipelining, never stage barriers.** Every task starts the moment _its own_ dependencies are met, regardless of what other chapters are doing (§4).
4. **Everything is idempotent and resumable.** A crash 45 minutes into a run must resume, not restart. Artifacts are content-addressed on disk; state lives in SQLite.
5. **Capacity is the binding constraint.** Not cost, not rate limits — one local box. The scheduler exists to saturate it without oversubscribing it.
6. **The binary is the deliverable.** One static file, cross-compiled, embedded UI, no runtime to install. Anything that breaks this — cgo, a required sidecar service — needs a strong justification.
7. **Performance is a top-level requirement, not a later optimization.** The system must be blazingly fast. Every layer we own has a stated latency, allocation, and throughput budget enforced by benchmarks. Waiting on a GPU later is not a licence for our own code to be slow.
8. **Battle-tested libraries when possible and avoid reinventing the wheels.**
9. **Standards are enforced, not intended.** Lint, layering, race detection, and benchmarks are CI merge gates from commit one.

---

## 3. Domain model

```
Channel ──< Video ──< Chapter ──< Asset
```

| Entity      | Owns                                                                     |
| ----------- | ------------------------------------------------------------------------ |
| **Channel** | Identity, style/voice config, YouTube credentials                        |
| **Video**   | Lifecycle state, blueprint, final artifact, upload record                |
| **Chapter** | Ordinal, title, script, audio, images, composed clip                     |
| **Asset**   | Content-addressed file on disk + metadata (kind, hash, size, provenance) |
| **Setting** | A single runtime configuration value, keyed by a stable key              |

Every generated file is an `Asset`. Re-running a task that produces an identical input hash is a no-op — this is what makes partial re-runs cheap.

### Stable human-readable keys on all master data

**Every master-data entity carries a unique, human-readable natural key in addition to its opaque id.** A `Channel` has a `slug` (`history-shorts`, `deep-sleep-stories`) with a `UNIQUE` index; a `Setting` has a `key`.

| Entity      | Natural key | Example              |
| ----------- | ----------- | -------------------- |
| **Channel** | `slug`      | `deep-sleep-stories` |
| **Setting** | `   `       | `pool.image.limit`   |

Rules:

- **Unique and immutable.** The slug is chosen at creation and never changes; renaming the display name does not touch it. It is enforced by a `UNIQUE` constraint in the schema, not by application code., the stable key might be slug or some thing like JIRA, for e.g HIS-1, etc...
- **Constrained shape.** Lowercase kebab-case, validated at the constructor — a slug is a domain type (`type Slug string`), not a bare string.
- **Seeds are upserts by natural key.** `INSERT ... ON CONFLICT (slug) DO UPDATE` — running the seed a second time updates in place instead of creating a duplicate. This is what makes **`make seed` stable and repeatable**: a fresh database and a ten-times-seeded database end up in the same state.
- **Fixtures and tests reference the slug**, never a generated id. Golden-file tests (§8.4) stay stable because their inputs don't contain ids that change per run.
- **The API accepts either.** `/api/channels/{slugOrID}` resolves a natural key or an id, so CLI usage and hand-written requests never need a lookup step first.

### Settings live in a table, not a config file

**Runtime configuration is a `settings` table in SQLite**, one row per key, read through a `SettingRepository` like any other entity.

- **Pool limits (§5), provider selection, gate configuration, and default chapter/image counts are settings rows** — changing one is a row update applied without restarting the daemon, not a file edit and a redeploy.
- **The table is the single source of truth.** Flags and env vars cover only what is needed _before_ the database is open: db path, listen address, log level. Everything else is a row.
- **Seeded with defaults** by the same idempotent upsert as other master data, so a fresh database boots with a complete, valid settings set and no null-handling anywhere.
- **Typed accessors, not a string bag.** The value column is stored as text, but every setting is read through a typed getter that parses and validates it; an unparsable value fails loudly at startup, not at first use.
- This is what makes the **Settings screen (§9)** a plain CRUD surface over the API, with no privileged file access.

## 4. The pipeline

```
                         blueprint  (LLM)
                              │
              ┌───────────────┴────────────────┐
              │                                │
    (needs script)                    (needs blueprint ONLY)
              ▼                                ▼
        script_i  (LLM ×N)          prime_image_prompts  (LLM ×1)
              │                                ▼
              ▼                        prompts_i  (×N, cache reads)
         tts_i  (TTS ×N)                       ▼
              │                        images_i  (IMG ×2N)
              └────────────────┬───────────────┘
                               ▼
                        clip_i  (compose ×N)
                               ▼
                     concat → final  (compose ×1)
                               ▼
                      metadata (LLM) → upload
```

**The structurally important detail:** `prompts_i` depends on the **blueprint alone**, not on the chapter script. This gives the graph two independent branches that run in parallel, and it is what makes the image pipeline — the longest pole — start early.

### Image prompt coalescing

All image prompts for a video come from **one LLM call** — better cross-chapter visual coherence, and it avoids re-sending blueprint context N times.

**But the DAG keeps N clean per-chapter prompt tasks** — individually retryable, uniform with every other chapter task. The provider coalesces them behind the interface using `golang.org/x/sync/singleflight`: the first request for key `(video_id, image_prompts)` produces all chapters' prompts and caches them; concurrent and later requests return their own slice.

The explicit `prime_image_prompts` task exists because the daemon caps LLM concurrency at 2 — without it there would never be a batch to coalesce. It occupies a real LLM slot; the N per-chapter tasks then fan out as cheap cache reads on a separate high-concurrency tag.

---

## 5. Concurrency & resource model

Global pools, enforced **across all videos and all channels** — video A's chapter 3 competes with video B's chapter 40 for the same slots. All limits are `settings` rows (§3), changeable at runtime without a restart.

| Pool      | Default |
| --------- | ------- |
| `llm`     | 2       |
| `tts`     | 2       |
| `image`   | 2       |
| `compose` | 2       |

**The per-pool count is the admission control.** A task acquires exactly one slot in exactly one pool and holds it for the duration of the provider call. Pools are independent — an `image` task never waits on an `llm` slot.

**Mechanism:** `golang.org/x/sync/semaphore.Weighted` — acquire, release, context-cancellable, FIFO-fair. A supported package built for exactly this; we write no counter of our own.

Selection is **greedy-with-skip** and cannot deadlock: a task holds at most one slot and never acquires a second, so there is no hold-and-wait.

---

## 6. Approval gates

The pipeline **pauses and waits for a human** at configured stages — initially after **blueprint** and before **upload**.

Waits may last **days**, so state cannot live in memory. This is why every step is DB-backed and idempotent.

A gated task does not enqueue its successors; it sets state to `awaiting_approval`. The CLI/API enqueues them on approval. Because we own the scheduler there is no in-memory flow to suspend: the task table _is_ the state, an unscheduled successor consumes nothing, and the daemon can restart freely while a gate is open. **A gate is a row update.**

---

## 7. Architecture

The main binary is HTTP handling, a state machine, and a scheduler. It has no ML dependencies and, in this version, no external dependencies at all.

### Code layout (Clean Architecture)

- `cmd/` — the server entrypoint. In production, the built React web UI is embedded into the Go binary (`embed.FS`), so the whole app ships as a single file and a single click runs it.

- `make dev` — development mode with hot reload, for both the Go server and the frontend.

- `domain/{entity,repository,service}` — the core domain model: entities, repository interfaces, and domain services. Dependency-free.

- `app/` — the application layer: use cases. Every exported function lives in its own file, named after the use case (`list_channels.go`, `approve_blueprint.go`, etc.). This is where the real logic of the app lives. naming convention verb\_... very descriptive, easy to distinguish by looking at.

- `adapters/` — concrete implementations of the interfaces declared in `domain/`, e.g. `sqlite`, `mock`.

- `delivery/http` — HTTP handlers. use chi library, with dependency-explicit function signatures.


- **Delivery is thin.** `delivery/http` do nothing but call the corresponding function in `app/`. No business logic lives in delivery.

- **Dependencies are explicit — never injected via an aggregate type.** A function must declare exactly the narrow interfaces it uses as separate parameters, not a container struct bundling many dependencies from which it only reads a few.

  ```go
  // Bad — SomeFunc only touches Repo1, but the signature hides that
  type AllRepos struct { Repo1 Repo1; Repo2 Repo2 }
  func SomeFunc(repos AllRepos) error { /* only uses repos.Repo1 */ }

  // Good — the signature is the whole dependency list
  func SomeFunc(repo1 Repo1) error { /* ... */ }
  ```

  A "god struct" of dependencies hides the real blast radius, forces tests to construct or mock things a function never touches, and makes it impossible to tell from the signature alone what a function can affect. This applies everywhere — `app/` use cases, `delivery` handlers, `domain/service` — not just at the top-level wiring in `cmd/`.

  pass dependency by function signature, not by state oof struct

  ```go
    // Good example: app/list_channels
    func ListChannels(channelReader ChannelReader) ...

    // or handler in deliver/http
    // channels.go: endpoint is GET /api/channels
    func getChannels(channelReader ChannelReader) HandlerFunc {
        return func (w, r) {
            app.ListChannels(channelReader)
            // ...
        }
    }
    // or videos.go: endpoint is GET /api/videos?channelId=
    func getVideos(...) HandleFunc {
        return func(w, r) {
            app.ListVideosByChannelID(...)
        }
    }
  ```
  as you can see from the example above
  - every dependencies is explicit as function signature
  - logic in side app/, the handler is just thin wrapper for calling app, so later on if we need cli interface, it just super easy to do so

```
yt-studio/
├── cmd/
│   └── server/
│       └── main.go                 # wires everything: config → adapters → app → delivery → serve
├── domain/
│   ├── entity/
│   │   ├── channel.go
│   │   ├── video.go
│   │   ├── chapter.go
│   │   ├── asset.go
│   │   ├── task.go                 # task/outcome types, TaskOutcome sealed interface
│   │   ├── setting.go              # Setting entity + typed keys
│   │   ├── slug.go                 # Slug domain type, validation
│   │   └── ids.go                  # VideoID, ChapterID, AssetID, ChannelID, TaskID
│   ├── repository/
│   │   ├── channel.go              # ChannelRepository interface (ByID + BySlug)
│   │   ├── video.go                # VideoRepository interface
│   │   ├── chapter.go              # ChapterRepository interface
│   │   ├── asset.go                # AssetRepository interface
│   │   ├── setting.go              # SettingRepository interface
│   │   └── task.go                 # TaskRepository interface (scheduler's durable backing)
│   ├── provider/
│   │   ├── llm.go                  # LLMProvider interface
│   │   ├── tts.go                  # TTSProvider interface
│   │   ├── image.go                # ImageProvider interface
│   │   ├── composer.go             # VideoComposer interface
│   │   └── uploader.go             # Uploader interface
│   └── scheduler/
│       ├── dag.go                  # DAG construction, precomputed per video
│       ├── pools.go                # semaphore-backed per-pool concurrency limits
│       ├── readyset.go             # in-memory ready-set, authoritative dispatch structure
│       └── dispatch.go             # event-driven dispatch loop
├── app/
│   ├── list_channels.go
│   ├── create_channel.go
│   ├── create_video.go
│   ├── approve_blueprint.go
│   ├── retry_chapter.go
│   ├── get_scheduler_status.go
│   └── ...                         # one exported use-case func per file
├── adapters/
│   ├── sqlite/
│   │   ├── channel.go              # implements repository.ChannelRepository
│   │   ├── video.go
│   │   ├── task.go
│   │   ├── setting.go              # implements repository.SettingRepository
│   │   ├── seed.go                 # idempotent upsert by natural key (slug / setting key)
│   │   ├── queries/                # sqlc-generated
│   │   └── migrations/             # goose, embed.FS
│   ├── mock_provider/
│   │   ├── llm.go                  # implements service.LLMProvider. nothing of the mock leak outside this package
│   │   ├── tts.go
│   │   ├── image.go
│   │   ├── composer.go
│   │   └── uploader.go
│   └── youtube/                    # deferred (§11), real Uploader impl
├── delivery/
│   ├── http/
│   │   ├── router.go
│   │   ├── channels.go             # calls app.ListChannels, app.CreateChannel
│   │   ├── videos.go
│   │   ├── events.go               # SSE stream
│   │   └── dto.go                  # request/response types (huma-tagged)
├── web/                            # React + Vite + TS frontend, built separately
│   ├── src/
│   └── ...
├── Makefile                        # make dev, make build, make test
└── goal.md
```

- repository is divided by reader and writer, for e.g, interface VideoReader, or interface VideoWriter, etc...
### Provider interfaces

Five small, consumer-defined interfaces. Real adapters slot in later without the daemon noticing:

```go
type LLMProvider interface {
    Blueprint(ctx context.Context, req BlueprintRequest) (Blueprint, error)
    Script(ctx context.Context, req ScriptRequest) (Script, error)
    ImagePrompts(ctx context.Context, videoID VideoID) ([]ImagePrompt, error)
    Metadata(ctx context.Context, req MetadataRequest) (Metadata, error)
}

type TTSProvider   interface { Speak(ctx context.Context, req SpeakRequest) (AssetID, error) }
type ImageProvider interface { Generate(ctx context.Context, req ImageRequest) (AssetID, error) }
type VideoComposer interface { Clip(...) (AssetID, error); Concat(...) (AssetID, error) }
type Uploader      interface { Upload(ctx context.Context, req UploadRequest) (UploadRecord, error) }
```

### Mock requirements

The mocks are the deliverable, so they are held to real standards:

- **Produce real files.** Small but valid PNG, WAV, and MP4 outputs, so content-addressing, the asset store, and the composition path are all genuinely exercised.

**The rule that survives into the real implementation:** a provider call never spans more than one unit of work. No multi-chapter calls, no fan-out inside a provider. All orchestration — lifecycle, the cross-chapter DAG, resource pools, retries, persistence, gates — belongs to the daemon.

- nothing of mock leak out side of the package.

### Why we own the scheduler

Every off-the-shelf Go option costs a server process, which breaks principle #6 outright:

| Option   | Requires               | Verdict                                              |
| -------- | ---------------------- | ---------------------------------------------------- |
| Temporal | ~4 services + Postgres | Absurd ops weight for a single-user local tool.      |
| River    | **Postgres**           | Adds a server we don't otherwise need.               |
| Asynq    | **Redis**              | Same.                                                |
| **Ours** | Nothing                | ~600 lines over a SQLite task table, zero processes. |

`semaphore.Weighted` supplies the concurrency control an engine would have supplied, and principle #1 already says the core owns all logic. **The scheduler is the interesting part of this system** — owning it is the point, not the burden.

---

## 8. Engineering standards

Requirements, not aspirations. Each has an enforcement mechanism, because a standard that isn't checked in CI is a preference that decays.

### 8.1 Battle-tested libraries only

**Admission criteria — a dependency must meet all five:**

1. **Solves a problem we genuinely have.** A wrapper that saves ten lines but obscures the underlying tool is a net loss.
2. **Actively maintained** — releases or meaningful commits within the last 12 months, and issues that get answered.
3. **Widely adopted** — in broad production use. We are not anyone's early adopter.
4. **Stable API** — v1 or later, or a documented compatibility promise.
5. **Justifiable weight** — check the transitive tree first. A library that pulls in twenty packages to do one thing is rejected.

**Prefer the battle tested libraries instead of reinventing the wheel.**

| Concern            | Choice                                      | Why                                                                                                                                                |
| ------------------ | ------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| Language           | **Go 1.23+**                                |                                                                                                                                                    |
| Logging            | `log/slog`                                  | Stdlib, structured, zero dependencies                                                                                                              |
| Concurrency        | `golang.org/x/sync`                         | `semaphore.Weighted` (§5), `singleflight` (§4), `errgroup`                                                                                         |
| HTTP client/server | `net/http`                                  | Stdlib routing is sufficient since 1.22                                                                                                            |
| API framework      | `danielgtaylor/huma`                        | Strongly typed `func(ctx, *Input) (*Output, error)` handlers on top of `net/http`; generates OpenAPI from Go structs — no hand-written schema (§9) |
| Database           | `database/sql` **+** `sqlc`                 | Typed Go generated from real SQL — no ORM, no runtime reflection                                                                                   |
| SQLite driver      | `modernc.org/sqlite`                        | **Pure Go.** cgo-free is what makes cross-compilation and a static binary work                                                                     |
| Migrations         | `pressly/goose`                             | Embeds via `embed.FS` — ships inside the binary                                                                                                    |
| CLI                | `alecthomas/kong`                           | Struct-tag driven and typed                                                                                                                        |
| Bootstrap config   | `kong` env tags                             | Flags + env for db path, listen address, log level only — everything else is a `settings` row (§3)                                                 |
| Retries            | `cenkalti/backoff/v4`                       | Context-aware; don't hand-roll backoff                                                                                                             |
| Validation         | Constructors + `go-playground/validator`    | Tags for mechanical checks; hand-written for cross-field                                                                                           |
| Testing            | **stdlib** `testing` + `pgregory.net/rapid` | `rapid` for property-based scheduler/DAG invariants                                                                                                |
| Benchmark analysis | `benchstat`                                 | The CI performance gate (§8.3)                                                                                                                     |
| Lint               | `golangci-lint`                             | `errcheck`, `exhaustive`, `nilerr`, `bodyclose`, govet's `nilness`                                                                                 |
| Layering           | `internal/` **packages** + `depguard`       | Compiler-enforced (§8.4)                                                                                                                           |
| YouTube            | `google.golang.org/api/youtube/v3`          | Official; resumable/chunked upload                                                                                                                 |
| Web UI embedding   | `embed.FS`                                  | The built frontend ships inside the binary                                                                                                         |

**Anti-pattern: no ORM.** `sqlc` gives typed results from SQL we wrote. ORMs hide the query — the thing we need to read when something is slow — and add runtime reflection to every call.

**Anti-pattern: no media wrapper libraries.** When the real composer lands, build `[]string` argv directly and invoke via `os/exec`, logging the full argv. Wrappers hide the exact command, and when a composition fails at 2am you need to paste the precise invocation into a shell.

### 8.2 Idiomatic, strongly typed Go

- **Named types for identifiers.** `type VideoID string`, `type ChapterID string`. Passing a chapter id where a video id belongs must be a **compile error**.
- **Typed string constants for states and task kinds**, never bare literals, with the `exhaustive` linter on every switch.
- **Consumer-defined interfaces**, declared in the package that _uses_ them. Go interfaces are structural, so mocks need no inheritance and no registration — this is what makes principle #2 cheap.
- **Small interfaces.** One method beats six.
- `context.Context` **first parameter, everywhere.** Cancellation is how a shutdown or an aborted video actually stops work.
- **Errors are values.** Wrap with `%w`, inspect with `errors.Is`/`errors.As`. `errcheck` is a merge gate — no ignored returns.
- **No** `any` **in domain packages.**
- **Goroutines are owned.** Every goroutine has an owner that waits for it (`errgroup`) and a context that can stop it. No fire-and-forget.
- `go vet` **+** `-race` **on every CI run.** The race detector is not optional for a scheduler.

**Closed task outcomes.** Go has no sum types, so outcomes use a sealed interface — an unexported marker method means no other package can add a case:

```go
type TaskOutcome interface{ isTaskOutcome() }

type Success          struct{ Assets []AssetID }
type Failed           struct{ Err error; Retryable bool }
type AwaitingApproval struct{ Gate GateKind }

func (Success) isTaskOutcome()          {}
func (Failed) isTaskOutcome()           {}
func (AwaitingApproval) isTaskOutcome()  {}
```

`exhaustive` does not check type switches, so two rules are mandatory: every `switch o := outcome.(type)` ends with `default: panic(fmt.Sprintf("unhandled outcome %T", o))`, and a table-driven test asserts every outcome type is handled at every switch site.

### 8.3 Performance

**Performance is a top-level requirement (principle #7).** Every budget below is enforced by a benchmark or a test, and a PR that regresses one does not merge.

| Budget                                                    | Target       |
| --------------------------------------------------------- | ------------ |
| Scheduler dispatch decision (pick next runnable task)     | **< 1 ms**   |
| Task state transition, committed to SQLite                | **< 5 ms**   |
| API response, p99                                         | **< 50 ms**  |
| SSE event from state change to client receipt (localhost) | **< 20 ms**  |
| Server cold start to serving requests                     | **< 100 ms** |
| Server idle CPU (no active video)                         | **< 0.5 %**  |
| Server RSS, 10 concurrent videos, 500+ tasks in flight    | **< 150 MB** |

**Enforcement**

- `testing.B` **benchmarks with** `-benchmem` for the scheduler hot loop, DAG construction, task-table queries, and JSON boundaries.
- `benchstat` **gate in CI:** no benchmark may regress more than **5 %** against the `main` baseline. Regressions block the merge.
- **Zero allocations per iteration** in the dispatch loop in steady state, asserted via `testing.AllocsPerRun`.

**Scheduling**

- The scheduler is **event-driven via channels**. Polling is forbidden as the primary mechanism; a DB poll may exist only as a safety net at 30 s or longer.
- A completed task signals its dependents directly. **Never rescan the task table to find work.**
- Maintain an **in-memory ready-set** as the authoritative dispatch structure, with SQLite as durable backing. Never query to answer "what can run now?"
- Precompute the dependency graph once per video at enqueue time.
- A cancelled video frees its slots within **100 ms**.

**Database**

- **WAL mode**, `synchronous=NORMAL`, explicit `busy_timeout`.
- **All statements prepared once and reused.** No string-built SQL in a hot path.
- **A single writer goroutine** owns all writes, serialised through a channel; readers use a separate pool. This eliminates lock contention rather than retrying it.
- **Batch transitions** — N tasks completing together write in one transaction.
- Index every column the scheduler filters or orders by, and assert with `EXPLAIN QUERY PLAN` in tests that no scheduler query does a full scan.

**Memory**

- **Preallocate with known capacity** — `make([]T, 0, n)`, which for a 50-chapter DAG is always known.
- `sync.Pool` **for reusable buffers** on hot paths.
- **No** `fmt.Sprintf` **in hot paths.** Use `strconv` or `strings.Builder`.
- Pass large structs by pointer, small ones by value. Avoid `defer` in tight loops.

**I/O**

- **Stream, never buffer.** Multi-GB files move with `io.Copy` and sized buffers. Reading a final render into memory is a bug.
- **One shared** `http.Client` with a tuned `Transport`: keepalives on, `MaxIdleConnsPerHost` at the pool ceiling, explicit timeouts. Never construct a client per request.
- `encoding/json` by default. A faster encoder is permitted **only after a profile shows JSON is a real cost**, and must remain a drop-in. No `map[string]any` on hot paths.

**Startup**

- Migrations run at startup in **< 50 ms** on an existing database.
- No work in `init()` beyond registering constants. Lazy-load anything expensive.
- Build with `-trimpath -ldflags="-s -w"`. Track binary size in CI; flag growth over 10 % in a single PR.

**The discipline:** measure first, optimize second, keep the benchmark. Speed that isn't in a benchmark is speed that will be lost.

### 8.4 Layering, testing, observability

**Testing**

- Fast pure unit tests over domain logic and state transitions.
- **Property-based tests (`rapid`) over scheduler invariants:** never exceed the budget, never deadlock, never start a task before its dependencies, never lose a task.
- **Deterministic golden-file tests:** the same seed and inputs produce an identical task sequence and identical artifact hashes.
- Benchmarks as tests — the `benchstat` gate runs on every PR.
- `-race` on all of it.

**Observability**

- `slog` structured logs with correlation attributes (`video_id`, `chapter_id`, `task`) on **every** line, via a context-carried logger. Level configurable at runtime without a restart.

**Scale** is _vertical_: one box, more channels, more concurrent videos, more chapters. Global pools (§5) absorb that — queue depth grows while resource usage stays flat.

---

## 9. Web UI

The web UI is the **product surface** — where an operator reviews a blueprint, sees chapter images side by side, edits a script before approving it, and watches a render progress. It is a client of the daemon's API, never a privileged component.

### Stack

| Concern      | Choice                                                  | Why                                                                |
| ------------ | ------------------------------------------------------- | ------------------------------------------------------------------ |
| Framework    | **React 19 + TypeScript (strict)**                      | `strict` mirrors the Go side's typing discipline                   |
| Build        | **Vite**                                                | Fast dev server, small production output                           |
| Styling      | **Tailwind CSS**                                        | No runtime CSS-in-JS cost; styles compile away                     |
| Components   | **shadcn/ui** (Radix primitives)                        | Copied into the repo, not a dependency; accessible by default      |
| Server state | **TanStack Query**                                      | Caching, dedup, background refetch — the whole app is server state |
| Routing      | **TanStack Router**                                     | Typed routes                                                       |
| Live updates | **SSE via** `EventSource`                               | See below                                                          |
| Long lists   | **TanStack Virtual**                                    | 50+ chapters × multiple tasks must scroll at 60 fps                |
| Client state | Query cache first; **Zustand** only if genuinely needed | Most "state" here is server state                                  |
| Delivery     | `embed.FS` in the Go binary                             | No separate web server, no separate deploy                         |

**Rejected: SSR / meta-frameworks.** This is a single-user local app served from a binary. SSR has no network round trip to optimize and breaks the single-binary story. A static SPA is the correct shape.

- UI/UX: Enterprise graded UI UX beautiful, modern, elegant, easy to use, vscode inspired beautiful

### Live updates: SSE, not WebSockets

Task state flows **one way** — daemon to browser. `EventSource` gives reconnection, event IDs, and resume-from-last-id for free over plain HTTP, with no upgrade handshake. WebSockets would add a bidirectional protocol we have no use for.

- **One SSE stream per client**, multiplexing all task and video events — not one stream per video.
- Events carry the **delta**, not a full state dump; the client applies them to the Query cache.
- The daemon **coalesces bursts**: at most one event per **50 ms** per video, batched. A 50-chapter render must not emit hundreds of events per second.
- Reconnect resumes from `Last-Event-ID`; the client never needs a full reload to recover.

### API contract

- **REST + JSON**. Mutations are `POST` and idempotent by request key.
- **TypeScript types are generated from the Go structs**, never hand-written: huma emits an OpenAPI spec from the Go request/response types, and `openapi-typescript` turns that into the client's types. A drifted client type is a build failure, not a runtime surprise.
- The UI has **no privileged access** — every action it can take is available via the CLI, and vice versa. This keeps the API honest.

### Screens

1. **Channels** — identity, style/voice config, credential status.
2. **Video list** — state, progress, bottleneck pool at a glance.
3. **Video detail** — the important one: chapter grid with **images side by side**, inline script editing, per-chapter retry, approval action.
4. **Scheduler view** — live task table, pool utilisation, queue depth per pool. The operator console.
5. **Settings** — pool limits, provider selection, gate configuration. A CRUD surface over the `settings` table (§3); edits apply without a restart.

### Performance budgets

Principle #7 applies to the frontend too:

| Budget                             | Target                |
| ---------------------------------- | --------------------- |
| Production bundle, gzipped         | **< 250 KB**          |
| First contentful paint (localhost) | **< 300 ms**          |
| Scheduler view scroll and update   | **60 fps sustained**  |
| Re-render on a single task event   | Only the affected row |

- Route-level code splitting; the scheduler view is not in the initial bundle.
- Images served with content-addressed URLs and immutable cache headers — the hash _is_ the cache key, so they cache forever for free.
- Thumbnails served instead of full-resolution stills in grid views.

### Development

The Vite dev server proxies `/api` and `/events` to the running daemon. `go generate` builds the frontend and stages it for `embed.FS`; the production binary contains the built assets and serves them from `/`.

---

## 10. Decisions & rationale

| Decision                                           | Rationale                                                                                                                      |
| -------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| **Go**                                             | The daemon is a state machine, a scheduler, and I/O. Delivers one static binary with an embedded UI and no runtime to install. |
| **Own the scheduler**                              | Every alternative requires a server process, breaking principle #6. `semaphore.Weighted` supplies the concurrency control.     |
| `semaphore.Weighted` **for pool limits**           | Context-cancellable and FIFO-fair; we write no counter of our own.                                                             |
| **Pure-Go SQLite (`modernc.org/sqlite`)**          | cgo-free is the precondition for cross-compilation and a static binary.                                                        |
| `sqlc`**, no ORM**                                 | Typed results from SQL we can read, with no runtime reflection.                                                                |
| **Single writer goroutine for SQLite**             | Eliminates write-lock contention instead of retrying it (§8.3).                                                                |
| **SSE, not WebSockets**                            | Updates are one-way; `EventSource` gives reconnect and resume for free.                                                        |
| **Static React SPA, no SSR**                       | Single-user local app served from a binary.                                                                                    |
| **Image prompts from blueprint only**              | Starts the bottleneck early — the core of the 2× speedup.                                                                      |
| **Coalescing behind the provider, not in the DAG** | Keeps the workflow observable and per-chapter retryable.                                                                       |
| **CLI first**                                      | Forces the daemon API to be complete and well-shaped; the web UI is a second client in the same binary.                        |

### Rejected

- **Temporal / River / Asynq / Machinery** — all require a server process. Rejected on ops weight, not capability.
- **Rust** — the app is ~95 % waiting on I/O. Go reaches the §8.3 budgets comfortably; Rust's additional complexity buys nothing measurable here.
- **An ORM (GORM/ent)** — hides the query and adds runtime reflection to every call.

### Known costs

- **No declarative boundary validation.** A DTO is a plain struct with a `Validate()` or `Into()` method returning `(T, error)`; cross-field checks are hand-written.
- **We maintain a scheduler.** ~600 lines of genuinely concurrent code that must be correct. Mitigated by property-based tests, deterministic golden-file tests, and `-race`.
- **We build our own operator UI.** No framework hands us one. Mitigated by `yt-studio watch` first, §9 thereafter.

---

## 11. Deferred to the real implementation

Recorded so the thinking isn't lost, and so the interfaces stay shaped correctly. **None of this is in scope for this version.**

- **Real LLM, TTS, and image backends** behind the existing interfaces. Adding one is a single type implementing a single interface, plus config to select it.
- **Structured LLM output must be grammar-constrained.**
- **Real composition** via `os/exec` with explicit argv (§8.1). Must use stream copy wherever a re-encode is not required — particularly the final concat, via the demuxer with a file list. Re-encoding 3 hours of already-correct video is the single largest avoidable cost in the pipeline.
- **Real resumable YouTube upload**, streaming from disk in chunks, retrying the failing chunk rather than the file, with the session URI persisted on the video. Dry-run stays the default.

---

## 12. Roadmap

| Phase | Deliverable                                                                                        |
| ----- | -------------------------------------------------------------------------------------------------- |
| **1** | The scheduler: per-pool caps, event-driven dispatch, crash/resume. Property tests over invariants. |
| **2** | Approval gates. Full 50-chapter pipeline end to end against mocks.                                 |
| **3** | Web UI (§9) — React SPA, SSE live updates, embedded via `embed.FS`.                                |
| **4** | Real backends provider (§11).                                                                      |
