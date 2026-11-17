# Implementation plan: TaroDomain for Go

Status: Phase 0 done except CI's first green run; Phase 1 done (Steps 1.1–1.4), Steps 2.1–2.3 done, next is Step 2.4. Written 2026-11-17; revised the same day: the one platform adapter for now is Cloudflare (Phase 11), and the SQL provider and the native worker loop moved out of scope; every tool runs in a dev container on Kiri, none on the host (Step 0.3). Targets the [TaroDomain spec v1.0](../spec/README.md) at commit `a9243e3` (the `spec/` submodule), the same commit the [Rust port](https://github.com/Taro-Teknologi-Indonesia/tarodomain-rust) implements. The Rust port's [plan](https://github.com/Taro-Teknologi-Indonesia/tarodomain-rust/blob/main/docs/IMPLEMENTATION_PLAN.md) records the design problems this plan reuses; it in turn reuses the [Dart port's](https://github.com/Taro-Teknologi-Indonesia/tarodomain-dart).

## How to use this plan

- Work one step at a time, in order. Each step creates or edits **1–5 files, tests included**. If a step turns out to need more files, split it here first, then write the code.
- Tick a step's boxes as each item lands, and update the status line above when a phase finishes.
- A test that needs the local provider goes in the external test package (`package tarodomain_test`), because `local` imports the core (see Risks: import cycles). Other unit tests may sit in `package tarodomain`.
- Every command in this plan runs inside the dev container: `./dev <command>`, or a shell from `./dev`. Nothing is installed on the host (Decisions).
- If the code has to depart from the spec, add a row to [Go design decisions](#go-design-decisions) in the same step.

### Definition of done (every step)

- [ ] Only the files listed for the step were touched
- [ ] `gofmt -l .` prints nothing
- [ ] `go vet ./...`
- [ ] `staticcheck ./...`
- [ ] `go test -race ./...`
- [ ] `GOOS=js GOARCH=wasm go build ./...`, so everything that runs on Cloudflare Workers still builds for it (Step 0.4)
- [ ] The five commands above pass in every module of the repo: `./dev make check` runs them, inside the dev container, in each directory that has a `go.mod`
- [ ] Every new exported identifier has a doc comment
- [ ] The root module's `go.mod` still has no `require` line
- [ ] The step's boxes are ticked here

## Goal

1. **`tarodomain`** (the root package): the spec's platform-agnostic parts (§2–23, §25, §28–29), on the standard library only. No database driver, HTTP framework or cloud SDK.
2. **`local`**: the spec's LocalProvider (§24), in memory, for tests and local development.
3. **`tarotest`**: the contract test suites every provider must pass.
4. **`tarohttp`**: a `net/http` handler over the core's HTTP API (§26). A Cloudflare Worker serves the same handler.
5. **`cloudflare`**: the spec's CloudflareProvider (§24.2–24.3) and CloudflareWorker (§26.1), for Go compiled to WebAssembly. D1 stores everything, Queues deliver events in eventual mode, and Cron Triggers run catch-up, saga timeouts and cleanup. This is the one platform adapter for now.
6. **`examples`**: the spec's BankAccount / TransferMoney example, as a library, a command-line scenario and a Cloudflare Worker.

## Decisions

- **Every tool lives in a container; the host stays clean (decided 2026-11-17).** Go, staticcheck, make, Node.js and wrangler are installed in one dev image (`Dockerfile.dev`), never on the laptop. The host needs only a container engine with the `docker` CLI, git and an editor. The engine on the dev machine is [Kiri](https://runkiri.com) (the `kiri` Docker context); the repo files stay engine-neutral (a plain `compose.yaml` and the docker CLI), so any engine that speaks the Docker Engine API runs them. `./dev` is a small POSIX shell script that runs a command, or an interactive shell, in the `dev` service with the repo mounted. No dev-container integration (decided 2026-11-17): the editor edits the repo folder on the host, which the container mounts, and every compile, test and lint runs through `./dev`. CI builds the same image and runs the same commands in it, so there is one definition of the toolchain.
- **The root module depends on the standard library only.** UUIDv7, JSON, hashing, logging (`log/slog`), SQL (`database/sql`) and HTTP (`net/http`) are all there. Code that needs a third-party module (the Cloudflare adapter, the examples) lives in a nested module with its own `go.mod` and a `replace` directive pointing at the repo root. An application that imports `tarodomain` therefore pulls in nothing else.
- **Two targets: native and `GOOS=js GOARCH=wasm`.** Go has no first-party Workers runtime; a Go Worker is a WebAssembly module started by JavaScript glue. The core, `local`, `tarotest` and `tarohttp` must build for both, and CI checks that on every push. Tests run natively; the wasm target is only built, except for the contract harness, which runs inside workerd (Step 11.8).
- **Workers glue: [`workers-go`](https://github.com/syumai/workers-go), standard Go compiler.** It serves an `http.Handler` as the fetch handler and wraps Queues and Cron Triggers. It calls itself experimental, so the adapter uses a small part of it (`workers.Serve`, `cloudflare.GetBinding`, `queues`, `cron`) and pins its version. Its D1 `database/sql` driver has no transactions and no batch (checked 2026-11-17), so the adapter talks to D1 itself through `syscall/js`. TinyGo is the fallback if the standard compiler's module is too large or too slow to start (Risks).
- **Adapters are separate modules.** A platform adapter implements `Provider` in its own nested module, so its dependencies never reach the core. Cloudflare is the first; others (SQL on `database/sql`, AWS) can follow the same shape.
- **Go 1.24 is the minimum**, as `go.mod` already says. The plan uses generics, generic type aliases and `log/slog`. It does not use `testing/synctest`, which is still experimental in 1.24.
- **`context.Context` is the request-scoped storage (§5.5).** Every method that does I/O, raises an event or reads the time takes a `ctx` first. The `ExecutionContext`, the active unit of work, the clock and the saga mailbox travel in it. This replaces the Rust port's task-locals.
- **One flat core package.** Everything in Goal 1 is `package tarodomain` at the repo root, one file per concept. Go forbids import cycles, the builder needs every layer, and users get a single import. The spec's layers (§1.3) are kept by review, not by package boundaries. See Open questions.
- **Generic functions where the spec has generic methods.** Go methods can't have type parameters, so registration is `tarodomain.On(routes, handler)`, `tarodomain.StartsWhen(routes, handler)`, `tarodomain.AddCommand(builder, factory)` and so on.
- **No code generation and no reflection-based dispatch.** Names are declared by methods (`EventName()`, `CommandName()`, `AggregateType()`), so renaming a Go type never orphans stored events. Reflection is used only to reject pointer event types at registration.
- **Errors are one `*tarodomain.Error` type** with a `Code`, working with `errors.Is` and `errors.As` (§22, and the design table).
- **Time is `time.Time` in UTC, cut to milliseconds**, behind an injectable `Clock`. JSON is `encoding/json`; event data, snapshot and saga state are `json.RawMessage` at the storage boundary.
- **Publishing.** A Go module is consumed through its git tags, so there is nothing to publish separately. While the repo is private, consumers set `GOPRIVATE=github.com/Taro-Teknologi-Indonesia/*`. Tags stay `v0.x` until the contract suites pass on every provider and one application runs on the framework.
- **Out of scope for now:** a SQL provider for servers outside Cloudflare (SQLite / PostgreSQL on `database/sql`), the native background worker loop that would go with it, the AWS provider (§24.4, §26.2, §27.3), and the §26.3 template generators, which `examples/cmd/bank-worker` replaces as the template to copy. `Provider.ProviderType()` returns a string, so each can be added later as its own nested module.

## Repository layout

| Path | Module | Contents | Phase |
|---|---|---|---|
| `spec/` | git submodule | The specification, pinned | 0 |
| `*.go` | root | `package tarodomain`: the core framework | 1–9 |
| `local/` | root | In-memory provider | 4 |
| `tarotest/` | root | Contract test suites | 4 |
| `tarohttp/` | root | `net/http` handler | 10 |
| `cloudflare/` | nested | Cloudflare provider and Worker entry points: D1, Queues, Cron Triggers | 11 |
| `cloudflare/harness/` | nested (same module) | A Worker that runs the contract suites inside workerd against a local D1 | 11 |
| `examples/` | nested | `bankaccount` (library), `cmd/bank` (scenario), `d1balances` and `cmd/bank-worker` (the same domain on Cloudflare) | 9, 11 |

Import direction: `local → tarodomain`, `tarotest → tarodomain`, `tarohttp → tarodomain`, `cloudflare → tarodomain` (and `tarohttp`, `tarotest`). The core imports none of them; its external tests (`package tarodomain_test`) import `local` and `tarotest`.

Core file map (root package), following the spec's layers (§1.2–1.3):

```
Kernel        errors.go error_codes.go error_report.go uuid.go clock.go backoff.go sleeper.go
Domain        metadata.go envelope.go execution_context.go value_object.go aggregate_id.go
              event.go serialized_event.go command.go aggregate.go
Persistence   stream_ids.go event_store_adapter.go snapshot_store.go saga_store.go
              idempotency.go dead_letter_store.go unit_of_work.go provider.go
Event bus     event_bus.go in_memory_bus.go
Event sourcing event_registry.go event_store.go
Repository    repository.go event_sourced_repository.go state_repository.go
Command       command_handler.go command_registry.go command_result.go
              concurrency_checker.go pipeline.go dispatcher.go
Projection    projection.go shard_projection.go
Saga          saga_state_machine.go compensation.go process_manager.go
              saga_event_handler.go saga_timeout.go
Catch-up      subscription_manager.go cron_dispatcher.go backpressure.go
Dead letters  retry_policy.go dead_letter_writer.go
Rebuild       projection_rebuilder.go saga_rebuilder.go
Trace, DI     trace.go di.go
Framework     validation.go aggregate_config.go builder.go framework.go http_api.go
```

## Go design decisions

The spec assumes JavaScript in places. These rows are where the Go port deliberately differs. The behaviour stays the same; only the mechanism changes. Rows marked **(Rust)** carry over a fix the Rust or Dart port found necessary; the Rust plan has the full reasoning.

| Spec mechanism | Go replacement | Why |
|---|---|---|
| Class inheritance: `extends AggregateRoot` (§1.1, §5.1) | An aggregate is a struct that embeds `tarodomain.AggregateRoot` and implements `Aggregate`: `AggregateType()`, `AggregateID()`, `Apply(Event) error`. `Raise(ctx, aggregate, event)` is a package function. | Go has no inheritance, and an embedded struct can't call the outer struct's `Apply`, so `Raise` must receive the aggregate. |
| `on[EventName]` dispatch on aggregates (§5.2) | `Apply` is a type switch. Its `default` case returns `tarodomain.MissingEventHandler(event)`. | No reflection. The compiler can't check exhaustiveness, so Step 2.5 adds a test helper that applies every registered event of an aggregate. |
| Event class with frozen fields, `eventName` defaulting to the class name (§2.1) | A plain struct of business fields with `EventName() string` on a value receiver, and optionally `EventVersion() string` (default `"1.0.0"`). The envelope is a separate `EventEnvelope`. Buses and handlers see a `DomainEvent` (envelope plus payload); `Recorded[E]` is the typed view, from `EventAs[E](event)`. | An explicit name survives a rename of the Go type. Events travel by value, so a handler can't change another handler's copy. Slices and maps inside an event are shared: treat them as read-only (Risks). |
| Rehydration without the constructor (§2.1) | `encoding/json` into a new `E`, through factories registered in `EventRegistry` by `RegisterEvent[E](registry)`. | Standard Go serialization. |
| `on[EventName]` dispatch on projections (§12.2) | `Routes(r *ProjectionRoutes)` registers `tarodomain.On(r, p.onMoneyDeposited)`, where a handler is `func(ctx, Recorded[E]) error`. Unknown events are skipped and marked as processed. | No reflection. The route table gives validation (§29) the list of handled events. |
| `startsWhen…` / `continueWhen…` / `compensateWhen…` / `completedWhen…` (§13.1) | `StartsWhen`, `ContinueWhen`, `CompensateWhen`, `CompletedWhen` generic functions on `*SagaRoutes[S]`. Saga handlers are **synchronous and take no `ctx`**: `func(c *SagaCtx[S], e Recorded[E]) ([]Command, error)`. **(Rust)** | Sagas stay pure decision logic, so a rebuild that replays them has no side effects. Without a `ctx` a handler can't do I/O by accident. |
| `ExecutionContext` in pluggable, process-wide context storage (§5.5) | `WithExecutionContext(ctx, ec)` and `ExecutionContextFrom(ctx)`. The active unit of work and the clock use the same mechanism. | Each request has its own `ctx`, which removes the §5.5 caveat about concurrent requests. A goroutine gets the context only if the `ctx` is passed to it (Risks). |
| Events carry `correlationId` and `causationId` (§28.1) | Plus an optional `CommandCausationID`: for an event raised by a command, that command's own causation ID. Omitted from JSON when empty. **(Rust)** | Commands aren't stored, so without it `CorrelationTrace` can't link a command to the event that caused it. |
| `ValueObject<T>`: deep freeze, `equals`, `compareTo`, `with(mutator)` (§2.3) | `NewValueObject[T](value, validate)` returns an immutable `ValueObject[T]` with `Value()`, `Equal` and `With(f)`, which validates again. `Compare` exists only through `cmp.Ordered` helpers. | Unexported fields give immutability. `INCOMPARABLE_VALUE` stays in the code list for completeness. |
| `AggregateId` subclasses (§2.4) | `AggregateID` wraps a `UUID`. `ID[Tag]` is a generic typed ID; a domain declares `type AccountID = tarodomain.ID[accountTag]`. Repositories take the untyped `AggregateID` (`id.Untyped()`). | Mixing two ID types in commands, events and domain methods is a compile error, without code generation. |
| A `FrameworkError` subclass per code (§22) | One `*Error { Code, Category, Message, Context, cause }` with `Unwrap`. `ErrorCode` lists the 51 codes; user codes are made with `NewDomainError("INSUFFICIENT_FUNDS", …)` and its siblings per category. `errors.Is(err, tarodomain.ErrCode(…))` matches on the code. | Idiomatic Go. Callers can match on `Code`, and user code can still add its own. |
| `shouldRetry` for errors outside the hierarchy (§22.1) | Same: any error that isn't an `*Error` is infrastructure and retried. **Departure:** `context.Canceled` and `context.DeadlineExceeded` are never retried and never dead-lettered; retry loops and sleeps return them at once. | In Go, cancellation is how a caller stops work. Retrying or quarantining a cancelled call would be wrong. |
| `toSnapshot()` from own enumerable properties (§5.4) | An optional `Snapshotter` interface: `SnapshotState() (any, error)` and `RestoreSnapshotState(json.RawMessage) error`. Validation fails the build if `SnapshotEvery` applies to an event-sourced aggregate that doesn't implement it, unless that aggregate sets `SnapshotEvery(0)`. **(Rust)** | No reflection over fields, and the global `SnapshotEvery` never silently skips an aggregate. |
| `loadFromHistory(events, snapshotVersion?)` (§5.1) | `LoadFromHistory(aggregate, events)` continues from the current version, which `RestoreSnapshot` sets. **(Rust)** | Only one place sets the version. |
| Snapshot when `newVersion % snapshotEvery == 0` (§6.2) | Snapshot when a save *crosses* a multiple of `SnapshotEvery`. **(Rust)** | A save of several events can jump over the exact multiple. |
| `repo.save()` publishes after the append, and a failure fails the save (§6.2, §11.2) | Inside a unit of work, a publish failure fails the save and the rollback undoes the append. Outside one, the append is already durable, so the failure goes to the repository's `OnPublishError` hook and the save succeeds; catch-up delivers the events. **(Rust)** | §11.2: "If enqueue fails, events are in store — cron catches up." Failing the save would make the caller retry a change that is already stored. |
| `StateRepository<T>` base class (§6.3) | The user implements `StateRepository[A]`: `Load` (returning `(A, bool, error)`), `SaveState`, and optionally `Exists`. The framework's `StateStoredRepository[A]` wraps it with a bus and is the `Repository[A]`. **(Rust)** | One user repository serves both pipelines' buses (§11.3), and the user never handles a bus. |
| `CommandHandler<C, A>` with `aggregateType` and an event bus constructor parameter (§4.1) | `CommandHandler[C]` with only `Handle(ctx, CommandMessage[C]) error`. Repositories come from `RepositoryFor[A](commandContext)`, built on the running pipeline's bus. **(Rust)** | A handler may touch several aggregates or none, and which bus applies depends on the pipeline (§11.3). |
| `CommandDispatcher.resolve` returns a handler (§4.2) | `Resolve` returns the registration. The pipeline creates a fresh handler for each attempt. **(Rust)** | The handler instance depends on the pipeline's bus. |
| `Projection<TDb>` with a `db` property (§12.1) | `Projection` has no type parameter; each projection owns its storage. **(Rust)** | The framework never touches the read model. |
| `CompensationStack` of closures, not persisted (§13.5, §17) | Compensations are defined by name in `Routes` (`DefineCompensation(r, "refund", …)`). A step pushes the name, and the stack of names is saved with the saga state. Factories still run at compensation time. **(Rust)** | `SagaEventHandler` loads the saga from the store for every event, so a stack of closures registered for an earlier event would be empty. |
| `loadStream(streamId, fromVersion?)` and `loadAll(fromEventId?)` (§3.2) | `LoadStream(ctx, streamID, afterVersion, maxCount)` and `LoadAll(ctx, afterEventID, maxCount)`. Both are exclusive. **(Rust)** | A repository that restored a snapshot at version 5 passes 5, and a subscription passes its last seen event. |
| Saga commands get fresh IDs | Each command a saga handler returns gets a deterministic UUIDv7: the timestamp of the event's `OccurredAt`, and the remaining bits from SHA-256 of (saga type, event ID, position). **(Rust)** | A redelivered event recreates the same command IDs, and command idempotency skips them. |
| `SagaEventHandler` dispatches before saving (§13.3) | Same, plus a per-saga mailbox carried in the `ctx`. An event for a saga that is still being handled in the same call chain waits until that saga is saved. **(Rust)** | With synchronous delivery, a dispatched command publishes the next event before the saga is saved. |
| Subscription `lastSeen` only moves forward (§18.2) | Adapters order by an append **position**, not by event ID, and refuse to move a cursor back. **(Rust)** | Event IDs don't order events within one millisecond or across servers. |
| Catch-up retries a failing event forever (§18.3) | Same by default. The opt-in `CatchUpMaxAttempts(n)` dead-letters the event after `n` runs. **(Rust)** | A permanently stuck subscription is otherwise invisible. |
| A timeout handler that throws marks the saga `FAILED` (§19.1) | Only a non-retryable error does. A retryable one leaves the saga for the next run. The `SagaTimeout` ID is derived from the saga's type, ID and `UpdatedAt`. A saga already `COMPENSATING` when it times out is failed without calling the handler. **(Rust)** | A transient error shouldn't fail a saga for good, a retry must dispatch the same command IDs, and a compensating saga could otherwise never end. |
| `loadTimedOut(threshold, { maxCount })` (§19.1) | `LoadTimedOut(ctx, before, SagaQuery{SagaType, Limit})`. **(Rust)** | With one scheduler per saga type (§19.3), filtering after the query would let other types fill the limit. |
| `UnitOfWork` `begin` / `commit` / `rollback` (§11.4) | Kept, plus `ExecuteInUnit(ctx, unit, func(ctx) error)`, which puts the unit in the `ctx` it passes to the work. Adapters find it with `ActiveUnit(ctx)` and a type assertion. A nested call, or a strong dispatch inside one, joins the active unit and isn't retried on its own. **(Rust)** | A saga dispatching from inside a strong command has to share its transaction. |
| `Token<T>` / `Container` (§23) | `NewToken[T](name)` and generic functions `Bind`, `BindSingleton`, `BindScoped`, `BindTransient`, `BindInstance`, `Resolve` over a `*Container`. | Type-safe resolution without reflection or casts in user code. |
| `Framework.build() → Promise<Framework>` (§25) | `builder.Build(ctx) (*Framework, error)`. One runtime per process; the provider (for example a `*sql.DB`) is shared. | |
| `fetchHandler` / `queueHandler` / `scheduledHandler` (§25.2, §26) | The core exposes `Dispatch`, `HandleEvents` (queue consumer, taking `QueueMessage{Event, Attempts}`), `RunScheduled` (catch-up, then saga timeouts), `RunCleanup` and `HTTPAPI`, which maps plain request and response structs to the §26.1 routes. `tarohttp` serves `HTTPAPI` as an `http.Handler`, and the Cloudflare adapter connects fetch (through `tarohttp`), queue and scheduled to them. **(Rust)** | The core never imports `net/http` or platform code, and every host shares one route table. |
| TS type helpers `defineEvents`, `ProjectionOf`, `SagaOf` (§29.3) | Not needed: route tables cover them. | |
| `TransactionalDatabase` write-collecting proxy (§11.4) | `cloudflare.UnitOfWork` stages every write and commits them in one `db.batch()`. It also keeps an overlay of the staged events, saga records and idempotency markers, which the D1 adapters read before D1. The local provider copies its tables when a unit begins. **(Rust)** | D1 has no interactive transactions. With the spec's proxy alone, reads inside a unit can't see the unit's own writes, and a strong-mode transfer creates the transfer aggregate and the saga record and reads both again in the same unit. |
| Unit of work reads its own writes (§11.4) | The unit of work contract requires only what the framework reads back inside a unit: a stream's version and events, a saga, idempotency marks. D1 shows no more than that before the commit. **(Rust)** | Snapshots, subscriptions, dead letters and counts are never read back inside a unit, and staging them for reading would copy half of SQL. |
| D1 `loadAll … ORDER BY event_id` (§24.3) | D1 orders by an `AUTOINCREMENT` position column. **(Rust)** | Event IDs don't order events within one millisecond. D1 runs one write at a time, so insert order is commit order. |
| D1 append: `UPDATE streams … WHERE version = expected` inside the batch (§24.3) | The unique `(stream_id, stream_version)` key detects the conflict: a stale writer inserts a version that already exists, so the batch fails and rolls back. A guard statement also fails the batch when the expected version is ahead of the stream. The error becomes `OPTIMISTIC_CONCURRENCY`. **(Rust)** | An `UPDATE` that matches no row isn't an error, so on its own it would let the batch commit. |
| `stream_events` and `schema_migrations` tables (§27.1) | `stream_events` is folded into `events` (`stream_id`, `stream_version`, unique together). `subscriptions` adds `last_position`, `failed_event_id` and `failed_attempts`. One extra table, `locks`, holds job leases. The schema is the Rust `tarodomain-cloudflare` crate's, table for table. **(Rust)** | One D1 database can then be served by a Worker of either port. |
| `initialize()` runs D1 migrations; `POST /migrate` (§24.2, §26.1) | The migrations are embedded in the package (`embed`) and tracked in `schema_migrations`. `Initialize` applies any missing ones once per isolate. `POST /migrate` is off unless the app turns it on. **(Rust)** | Deploying needs no separate migration step, and an open migrate endpoint isn't exposed by default. |
| Queue transports and the strong bus on Cloudflare (§10.5, §24.2) | Transport `"queue"` is the queue bus (the default for eventual mode) and `"in_memory"` the in-process bus (the default for strong mode). **(Rust)** | Strong mode must deliver inside the command's unit of work, which a queue can't do. |
| Queue consumer: `message.retry({ delaySeconds })` (§10.5) | Same, with the delay from `RetryPolicy` and the attempt count from the message itself. **(Rust)** | Queues already count attempts, so no extra table is needed. |
| `CloudflareWorker.export → { fetch, queue, scheduled }` (§26.1) | `cloudflare.Serve(framework, options)` registers the queue consumer and the cron task without blocking, then serves the `tarohttp` handler. It is called from the Worker's `main`. | A Go Worker is a `main` package that registers handlers with the glue; there is no export object. |
| JavaScript promises (D1, Queues) | A goroutine that calls D1 parks on a channel until the promise settles (`await` in the adapter), and takes a `ctx`. | Go on `js/wasm` runs every goroutine on one thread, so blocking a goroutine is how it yields to the JavaScript event loop. |

## Phases and steps

### Phase 0: Bootstrap

#### Step 0.1: Repository and spec
Files: `.gitmodules` (plus the spec submodule's gitlink), `.gitignore`, `LICENSE`, `README.md`, `go.mod`, `doc.go`
- [x] `git init` on branch `main`
- [x] Spec added as the `spec/` submodule, pinned to `a9243e3`
- [x] `.gitignore` for build output, local SQLite files and `.env*`
- [x] `LICENSE`: MIT, the same as the spec
- [x] `go.mod` (`github.com/Taro-Teknologi-Indonesia/tarodomain-go`, Go 1.24) and `doc.go`
- The skeleton's first build is in Step 0.3, once the dev container exists

#### Step 0.2: Plan
Files: `docs/IMPLEMENTATION_PLAN.md`, `README.md`
- [x] This plan
- [x] README status links to it

#### Step 0.3: Dev container
Files: `Dockerfile.dev`, `compose.yaml`, `dev`, `README.md`
- [x] `Dockerfile.dev`: a Debian-based Go image at a pinned version (Debian, not Alpine: the race detector needs cgo and a C compiler, and workerd needs glibc), plus `make`, `git` and a pinned `staticcheck`. Node.js and wrangler are added in Step 11.8, when they are first used
- [x] The image's default Go is the newest stable release; `go.mod` keeps `go 1.24` as the minimum. Go 1.27.4 and staticcheck 2026.2.2, checked 2026-11-17
- [x] `compose.yaml`: one `dev` service built from `Dockerfile.dev`, the repo mounted at `/workspace`, named volumes for the Go module and build caches (so they survive the container and stay off the host), and no published ports yet
- [x] The container runs as a user whose UID and GID match the host's, so files it writes in the mounted repo aren't owned by root. Verify on Kiri
- [x] `dev`: `./dev` opens a shell, `./dev <command>` runs one (`docker compose run --rm dev …`). It uses the active Docker context and names no engine
- [x] `./dev go build ./...` and `./dev go vet ./...` pass on the skeleton (its first build)
- [x] README: prerequisites (a container engine, git), `./dev`, and that nothing else is installed on the host
- [x] The host has no `go`, `node`, `npm` or `wrangler` on its PATH (true on 2026-11-17)

#### Step 0.4: Tooling and CI
Files: `Makefile`, `.editorconfig`, `.github/workflows/ci.yml`, `README.md`
- [x] `make check`: `gofmt -l`, `go vet`, `staticcheck`, `go test -race` and `GOOS=js GOARCH=wasm go build ./...`, in every directory that has a `go.mod`. Nested modules added later are picked up without editing the Makefile. It is run as `./dev make check`; `make` on the host is not required
- [x] `make test` (tests only) and `make fmt`
- [x] `.editorconfig`: tabs for Go, LF line endings
- [x] CI: checkout with submodules, build the dev image (with layer caching), run `make check` in it. A second job runs the tests on the oldest supported Go (1.24) by overriding the image's Go version build argument (staticcheck is built in its own stage on the newest Go, because 2026.2.2 doesn't build on 1.24)
- [x] README: the development commands
- [ ] CI green on the first push (needs a push; the workflow is untested on GitHub)

### Phase 1: Kernel and errors (§1.3 Kernel, §22)

#### Step 1.1: Error categories and codes
Files: `error_codes.go`, `errors.go`, `errors_test.go`
- [x] `ErrorCategory` (`Domain`, `Concurrency`, `Infrastructure`, `Configuration`) with `Retryable()` and `HTTPStatus()` (400 / 409 / 500 / 500)
- [x] `ErrorCode`: all 51 codes of §22.2 as constants, with `String()`, `ParseErrorCode` and `Category()`
- [x] `*Error { Code, Category, Message, Context map[string]any }` with `Error()`, `Unwrap()`, `Retryable()`, `HTTPStatus()`, `WithContext(key, value)` and `WithCause(err)`. `With…` return a copy, so a shared error value is never changed
- [x] One constructor per code that carries context, e.g. `OptimisticConcurrency(streamID, expected, actual)`; `NewError(code, message)` for the rest
- [x] User codes: `NewDomainError(code, message)` and one sibling per category
- [x] `ErrCode(code)` as a target for `errors.Is`; `CodeOf(err) (ErrorCode, bool)`; `AsError(err) (*Error, bool)`
- [x] `ShouldRetry(err)`: an `*Error` answers with `Retryable()`; `context.Canceled` and `context.DeadlineExceeded` are `false`; anything else is infrastructure and `true`. `Infrastructure(err)` wraps a foreign error
- [x] Tests: 19 / 3 / 18 / 11 codes per category; the retryable and HTTP status matrix for every code; `String` ↔ `ParseErrorCode` round trip; a custom code keeps its category; `errors.Is` and `errors.As` through a wrapped error; cancellation isn't retried

#### Step 1.2: Error reports
Files: `error_report.go`, `error_report_test.go`
- [x] `ToJSON()`: `code`, `category`, `message`, `retryable`, `httpStatus`, `context`
- [x] `ToHTTPResponse() HTTPErrorResponse{Status, Body}`. `body.error` holds `code`, `category`, `message` and `retryable`, and adds `context` only for 4xx. The cause is never included
- [x] `ToLogEntry()`: level warn for domain errors and error otherwise, with the cause chain. It implements `slog.LogValuer`
- [x] Tests: 4xx includes context, 5xx omits it, the cause never leaks, log levels

#### Step 1.3: UUIDs
Files: `uuid.go`, `uuid_test.go`
- [x] `UUID [16]byte` with `String()` (lower case, hyphenated), `ParseUUID` (any version, case-insensitive, hyphenated form only), `IsV7()`, `IsZero()`, text and JSON marshalling
- [x] `NewUUIDv7()`: 48-bit Unix milliseconds plus random bits from `crypto/rand`
- [x] `DerivedUUIDv7(seed, index, unixMs)`: the timestamp from `unixMs` and the other 74 bits from SHA-256(seed, index). The same input always gives the same ID, and it is still a valid v7
- [x] Tests: version and variant bits, v7 detection, upper and lower case parse equal, other text forms rejected, derived IDs are deterministic, carry their timestamp and depend on every input, JSON round trip

#### Step 1.4: Clock, backoff and sleeper
Files: `clock.go`, `backoff.go`, `sleeper.go`, `clock_test.go`, `backoff_test.go`
- [x] `Clock` interface (`Now() time.Time`), `SystemClock`, and `ManualClock` (set and advance, safe for concurrent use)
- [x] `WithClock(ctx, clock)` and `Now(ctx)`: the clock in the `ctx`, else the system clock, always UTC and cut to milliseconds
- [x] `Backoff { Initial, Multiplier, Max, Jitter }` with `Delay(attempt, random)`: `Initial * Multiplier^attempt`, ±`Jitter`, never more than `Max`. `RandomSource` interface (`Float64()` in [0, 1)), defaulting to `math/rand/v2`
- [x] `Sleeper` interface (`Sleep(ctx, d) error`, returning the context's error if it ends first), `TimerSleeper`, and `RecordingSleeper` for tests
- [x] Tests: 10 → 20 → 40 ms with zero jitter, the cap, jitter within ±10%, the manual clock, the clock in a `ctx`, a cancelled sleep returns at once

### Phase 2: Domain primitives (§2, §5, §5.5, §28)

#### Step 2.1: Metadata, envelopes, ExecutionContext
Files: `metadata.go`, `envelope.go`, `execution_context.go`, `metadata_test.go`, `envelope_test.go`
- [x] `Metadata`: an immutable map from string to a primitive (nil, bool, number, string). `With(key, value)` returns a copy. Serialized as a JSON object in key order; decoding rejects arrays and objects and keeps integers exact (`json.Number`, see Risks)
- [x] `EventEnvelope { ID, CorrelationID, CausationID, CommandCausationID, Metadata, OccurredAt }` and `CommandEnvelope { ID, CorrelationID, CausationID, Metadata, Timestamp }`. Timestamps are cut to whole milliseconds
- [x] Parsing IDs from strings fails with `INVALID_EVENT_ID` / `INVALID_CORRELATION_ID` / `INVALID_CAUSATION_ID`, or the `INVALID_COMMAND_*` codes
- [x] `ExecutionContext { CorrelationID, CausationID, CommandCausationID, Metadata }` with `NewExecutionContext(correlationID)` (a root), `ForCommand`, `ForEvent`, `WithExecutionContext(ctx, ec)` and `ExecutionContextFrom(ctx)`
- [x] Envelope defaults: a new v7 ID; the context's correlation ID or a new root; the context's causation ID or none
- [x] Tests: defaults outside any context, inheritance inside one, the right code for each invalid ID, a derived `ctx` doesn't change its parent, the §28.2 propagation chain

#### Step 2.2: Value objects and aggregate IDs
Files: `value_object.go`, `aggregate_id.go`, `value_object_test.go`, `aggregate_id_test.go`
- [x] `NewValueObject[T](value, validate func(T) (bool, error))`. A `false` becomes `VALUE_OBJECT_INVALID`, an `*Error` passes through, and any other error becomes `VALUE_OBJECT_CONSTRUCTION_FAILED` with the error as its cause
- [x] `ValueObject[T]`: `Value()`, `Equal(other)`, `With(f)` (validates again), JSON marshalling of the inner value
- [x] `AggregateID` over `UUID`: `NewAggregateID()`, `AggregateIDFrom(string)`, `AggregateIDFromV7(string)`, `String()`, JSON as a string. An invalid UUID is `VALUE_OBJECT_INVALID`
- [x] `ID[Tag]`: a typed ID with the same factories (`NewID[Tag]()`, `IDFrom[Tag](string)`) and `Untyped() AggregateID`
- [x] Tests: an invalid value is rejected, a framework error passes through, a foreign error keeps its cause, `With` validates, equality ignores case, the v7-only factory rejects v4, typed ID JSON round trip

#### Step 2.3: Events
Files: `event.go`, `serialized_event.go`, `event_test.go`
- [x] `Event` interface (`EventName() string`) and the optional `VersionedEvent` (`EventVersion() string`); `EventVersionOf(e)` defaults to `"1.0.0"`
- [x] `DomainEvent`: envelope, name, version and payload, with `Payload() Event`. `NewDomainEvent(envelope, payload)` rejects a pointer payload (`INVALID_CONFIGURATION`)
- [x] `Recorded[E] { Envelope, Event E }`, `EventAs[E](DomainEvent) (Recorded[E], bool)` and `EventIs[E](DomainEvent) bool`
- [x] `SerializedEvent` with the §2.1 storage fields (`event_id`, `event_type`, `event_version`, `causation_id`, `correlation_id`, `data`, `metadata`, `occurred_at`) plus `command_causation_id`, typed (`UUID`, `Metadata`, `time.Time`, `json.RawMessage`). `DomainEvent.Serialize()` fails with `SERIALIZATION_ERROR` for a payload without a JSON form
- [x] `FormatTimestamp` / `ParseTimestamp`: the ISO 8601 form with exactly three fractional digits that every store writes
- [x] Tests: field names in the serialized JSON, `EventAs` to the wrong type is `false`, `occurred_at` is ISO 8601 with milliseconds, JSON round trip, a pointer payload is rejected

#### Step 2.4: Commands
Files: `command.go`, `command_test.go`
- [ ] `Command` interface (`CommandName() string`)
- [ ] `CommandMessage[C] { Envelope CommandEnvelope; Command C }`. `NewCommandMessage(ctx, c)` takes correlation, causation and metadata from the `ctx` and the timestamp from `Now(ctx)`. `WithID`, `WithCorrelationID`, `WithCausationID`, `WithMetadata` return copies
- [ ] `AnyCommand`: a type-erased command (`Name`, `Envelope`, `Payload`), from `Erase(message)`; `CommandAs[C](AnyCommand) (CommandMessage[C], bool)`
- [ ] `SerializedCommand` with the §2.2 fields (`command_id`, `command_type`, `causation_id`, `correlation_id`, `data`, `metadata`, `timestamp`)
- [ ] `AnyCommand.WithDerivedID(seed, index, at)` on top of `DerivedUUIDv7`
- [ ] Tests: defaults from the context, the timestamp from the clock in the `ctx`, the `With…` methods, `CommandAs`, serialized shape, derived IDs are stable and carry the event's time

#### Step 2.5: Aggregate root
Files: `aggregate.go`, `aggregate_test.go`
- [ ] `AggregateRoot`: version, uncommitted `[]DomainEvent`, and the aggregate's own execution context. Fields are unexported; `Root()` returns the embedded value, so embedding it satisfies the interface
- [ ] `Aggregate` interface: `AggregateType() string`, `AggregateID() AggregateID`, `Root() *AggregateRoot`, `Apply(Event) error`
- [ ] `Raise(ctx, aggregate, event) error`: apply, version + 1, queue with an envelope from the aggregate's own context, else the one in the `ctx`, else a new root, timed by `Now(ctx)`. A failed `Apply` queues nothing
- [ ] On `AggregateRoot`: `Version()`, `CommittedVersion()`, `HasUncommittedChanges()`, `UncommittedChanges()`, `MarkChangesAsCommitted()`
- [ ] `LoadFromHistory(aggregate, events)` continues from the current version and refuses to run with uncommitted changes (`INVALID_AGGREGATE_STATE`). `RestoreVersion(aggregate, version)` for state-stored aggregates (§6.3)
- [ ] `Snapshotter` interface, `ToSnapshot(aggregate) (SnapshotData, error)` and `RestoreSnapshot(aggregate, data, version)`. A snapshot that doesn't fit is `SERIALIZATION_ERROR`
- [ ] `MissingEventHandler(event)` for the `default` case of `Apply`, and `AssertAppliesAll(t, aggregate, events...)`, a test helper that fails if `Apply` rejects any of them
- [ ] Tests: version arithmetic, `Raise` inside a context carries its correlation and causation, the aggregate's own context comes first, `Raise` outside one starts a root, the time comes from the clock in the `ctx`, history replay doesn't queue, an unhandled event, snapshot round trip

### Phase 3: Persistence and bus contracts (§3.2, §8–10, §11.4, §12.3, §14, §21.1, §24.1)

#### Step 3.1: Event store adapter contract
Files: `stream_ids.go`, `event_store_adapter.go`, `stream_ids_test.go`
- [ ] `EventStoreAdapter` with every §3.2 method, each taking a `ctx`, plus `RecordFailedAttempt(ctx, name, eventID) (int, error)`
- [ ] `AppendResult`, `StreamMetadata`, `Subscription { Name, StreamID, LastSeen, CreatedAt, FailedEventID, FailedAttempts }`
- [ ] `StreamAll` (`$all`), `StreamForAggregate(type, id)`, `CategoryOf(streamID)` and `ParseStreamID` (§3.1). An aggregate stream ID ends in `-` and a lower-case UUID, so a type may contain hyphens
- [ ] Doc comments pin the semantics the contract suites test: versions count from 1, a missing stream has version 0, cross-stream reads come back in global **append** order, deleting a missing row is a no-op, a stale expected version fails with `OPTIMISTIC_CONCURRENCY`, appends are atomic and go only to aggregate streams, an empty append still checks the version, `Subscribe` is idempotent
- [ ] Tests: stream ID kinds, including hyphenated types and non-canonical IDs

#### Step 3.2: Snapshot and saga store contracts
Files: `snapshot_store.go`, `saga_store.go`, `saga_store_test.go`
- [ ] `SnapshotStoreAdapter` (§8.1), `SnapshotInput`, `Snapshot`. The key is `(source_uuid, source_version)`, and `Save` is an upsert
- [ ] `SagaStoreAdapter` (§9.1), `SagaStatus` (`RUNNING`, `COMPENSATING`, `COMPLETED`, `FAILED`, with `IsActive` / `IsTerminal`), `SagaSaveInput`, `SagaRecord`, `SagaQuery { SagaType, Limit }`
- [ ] `SagaRecord.Saved`, `Complete`, `Fail` and `IsTimedOut` state the write rules once (`StartedAt` kept, `UpdatedAt` moved, `CompletedAt` set on reaching a terminal status), for the in-memory adapter
- [ ] Tests: status text round trip, the timestamp rules, ending a saga only once, the timed-out filter

#### Step 3.3: Idempotency, dead letter and unit of work contracts
Files: `idempotency.go`, `dead_letter_store.go`, `unit_of_work.go`, `unit_of_work_test.go`
- [ ] `ProcessedEventStore` (§12.3), `CommandIdempotencyStore` (§14.1) and `ProcessedCommand`. Marking twice keeps the first record
- [ ] `DeadLetterStore` (§21.1), `DeadLetterInput`, `DeadLetterEntry`. `Write` returns the new ID; IDs increase in write order and are never reused
- [ ] `UnitOfWork`: `Begin(ctx)`, `Commit(ctx)`, `Rollback(ctx)`, `IsActive()`. `ExecuteInUnit(ctx, unit, work)` rolls back on error or panic (and re-panics), and returns the work's error unchanged, with `rollbackError` in its context if the rollback also failed. `ActiveUnit(ctx)`; a nested call joins the unit already in the `ctx`
- [ ] Tests (a fake unit): commit and rollback, the error unchanged, a failed begin, commit or rollback, a panic rolls back, a nested call joins and its error rolls back the outer unit

#### Step 3.4: Event bus contracts and InMemoryEventBus
Files: `event_bus.go`, `in_memory_bus.go`, `in_memory_bus_test.go`
- [ ] `EventBus` (`Name()`, `PublishAll(ctx, []DomainEvent) error`), `EventHandler` (`Name()`, `Handle(ctx, DomainEvent) error`), `SubscribableEventBus` (`Subscribe(handler) error`, `Unsubscribe(name) bool`)
- [ ] `NewInMemoryEventBus(name)`: delivers each event to every handler in subscription order before the next event. A second handler with a taken name is `INVALID_CONFIGURATION`. No lock is held while a handler runs, so a handler may publish to the same bus
- [ ] `PublishAll` is fail-fast and returns the handler's error unchanged. `PublishAllWithResults` continues and returns a `Delivery` per handler and event
- [ ] Tests (with `-race`): delivery order, fail-fast skips later handlers, with-results collects everything, duplicate names, unsubscribe, a handler publishing to its own bus, concurrent publishes

#### Step 3.5: Provider contract
Files: `provider.go`, `provider_test.go`
- [ ] `ConsistencyMode` (`Strong`, `Eventual`; `"strong"` / `"eventual"` as text and in JSON) and `EventBusConfig { Consistency, Transport }`
- [ ] `Provider` (§24.1): `ProviderType()`, a `Create…` method for each of the six stores, `CreateEventBus(config) (EventBus, error)` (an unknown transport is `INVALID_CONFIGURATION`), `CreateUnitOfWork() UnitOfWork` (a new unit per call, `nil` if unsupported), and `Initialize(ctx)` (safe to repeat)
- [ ] Optional interfaces `ClockProvider` and `SleeperProvider`; the framework falls back to `SystemClock` and `TimerSleeper`
- [ ] Tests: consistency text round trip, the fallbacks

### Phase 4: Local provider and contract suites (§24 LocalProvider)

#### Step 4.1: Local package and shared database
Files: `local/doc.go`, `local/database.go`, `local/database_test.go`
- [ ] `Database`: tables behind a mutex shared by every adapter of one provider, and an injectable `Clock`. The mutex is held only inside a synchronous closure (`read`, `write`), never across a call that can block
- [ ] Writes also take the database's turn (a one-slot channel, so waiting respects `ctx`), which a unit of work holds for its whole life (Step 4.4)
- [ ] `Table[T](db, name)`, so an application can keep in-memory read models in the same database and have them roll back with a unit
- [ ] Tests: empty default, shared handles, the clock, a write waits for the turn and gives up when its `ctx` ends, a copy of the tables is deep

#### Step 4.2: In-memory event store adapter
Files: `local/event_store.go`, `local/event_store_test.go`
- [ ] The expected-version check and the append happen in one `write`. A conflict is `OPTIMISTIC_CONCURRENCY` with the stream, expected and actual versions as context. An ID already stored, or twice in one append, is `EVENT_APPEND_FAILED` and writes nothing
- [ ] Cross-stream reads are ordered by append position. An `afterEventID` cursor resolves to its position, falling back to comparing IDs only if that event was deleted
- [ ] Category streams and `$all`
- [ ] Subscriptions: `Subscribe` is idempotent, `Ack` only moves forward in append order, and `RecordFailedAttempt` counts per event and resets on ack
- [ ] Tests: versions and conflicts, empty appends, non-aggregate streams, duplicate IDs, `afterVersion` and `maxCount`, append order across streams with same-millisecond IDs in reverse, the deleted-cursor fallback, correlation, subscriptions

#### Step 4.3: In-memory stores
Files: `local/snapshot_store.go`, `local/saga_store.go`, `local/idempotency.go`, `local/dead_letter.go`, `local/stores_test.go`
- [ ] One adapter per contract of Steps 3.2–3.3, each with a table of its own. Timestamps come from the database's clock
- [ ] Sagas: `LoadByStatus` ordered by `UpdatedAt`, then type and ID; `LoadTimedOut` filters to active sagas, by type, before the limit
- [ ] Dead letters get increasing IDs, never reused after a delete
- [ ] Tests: snapshot upsert and per-type reads and deletes; saga timestamps, ending once, list order, filter before limit; idempotent marks keep their first time; dead-letter IDs and the latest entry, `DeleteOld`

#### Step 4.4: Unit of work and LocalProvider
Files: `local/unit_of_work.go`, `local/provider.go`, `local/unit_of_work_test.go`, `local/provider_test.go`
- [ ] The unit takes the database's turn and copies the tables on `Begin`. Reads and writes whose `ctx` carries that unit use the copy, so the unit sees its own writes and other goroutines don't. `Commit` swaps the copy in; `Rollback` throws it away
- [ ] `Begin` on an active unit and `Commit` on an inactive one are `INVALID_CONFIGURATION`; `Rollback` on an inactive one does nothing
- [ ] `local.NewProvider()`, `local.NewProviderWithDatabase(db)` and `Database()`. `CreateEventBus` returns a new `InMemoryEventBus` for both modes; a transport other than `in_memory` is `INVALID_CONFIGURATION`. `ProviderType()` is `"local"`
- [ ] Tests (with `-race`): commit and rollback over several tables, others see writes only after commit, a write outside waits, units run one at a time, a unit over another database is ignored, state checks; the provider's shared storage, new units, buses, clock

#### Step 4.5: Testing package and event store contract
Files: `tarotest/doc.go`, `tarotest/fixtures.go`, `tarotest/event_store_contract.go`
- [ ] Each suite is `func XContract(t tarotest.T, newProvider Factory)`. `tarotest.T` is a small interface (`Helper`, `Logf`, `Errorf`, `Fatalf`, `Skipf`, `Run(name, func(T))`), because the Cloudflare harness runs the suites inside workerd, where there is no `go test` (Step 11.8). `tarotest.Wrap(t *testing.T)` adapts the standard one. `Factory` is `func(ctx) (tarodomain.Provider, error)` returning a provider with empty storage; each case asks it for a fresh one
- [ ] The package builds for `js/wasm` and imports `testing` only in the file that holds `Wrap`. If that import bloats or breaks the Worker build, move `Wrap` to a `tarotest/gotest` subpackage and record it here
- [ ] Fixtures: fixed times from a documented epoch, event IDs that increase with `n`, a same-millisecond pair that sorts in reverse, aggregate and stream IDs, a serialized event with every optional field set
- [ ] Event store contract: versions from 1 and 0 for a missing stream; a stale expected version conflicts (with its context), including two racing appends; an empty append; non-aggregate streams and duplicate IDs fail atomically; events round-trip unchanged; `afterVersion` and `maxCount`; category and `$all` reads; two same-millisecond IDs come back in append order; a cursor whose event was deleted; load by correlation ID; delete stream; subscription create, ack, list, delete and failed attempts

#### Step 4.6: Store contracts
Files: `tarotest/snapshot_contract.go`, `tarotest/saga_store_contract.go`, `tarotest/idempotency_contract.go`, `tarotest/dead_letter_contract.go`
- [ ] The suites never compare with fixed times: they compare with the timestamps the store returned, or with a cut-off long ago or far in the future, and check list order as a property
- [ ] Snapshots: the stored fields, upsert, latest (the highest version), at a version, per-type reads, list newest first, `DeleteBefore` keeps the cut-off and returns the count, `DeleteAll`, `Count`
- [ ] Sagas: the stored fields and timestamp rules, idempotent `Complete` / `Fail` (also on a missing saga), `LoadByStatus` order and the type filter before the limit, `LoadTimedOut`, `Delete`, `Count`
- [ ] Processed events and commands: idempotent marks keep the first record, `Exists` per instance, `Count`, `DeleteByHandler`, the command list order, `DeleteOld` with an exclusive cut-off
- [ ] Dead letters: the stored fields, IDs increase and aren't reused, `List` in ID order, `Get` returns the latest, `GetByID`, `Delete`, `Count`, `DeleteOld`

#### Step 4.7: Unit of work and concurrency contracts
Files: `tarotest/unit_of_work_contract.go`, `tarotest/concurrency_contract.go`
- [ ] Unit of work: commit keeps writes; rollback discards the writes of **every** store; the work's error comes back unchanged; reads inside the unit see its writes and readers outside don't; a nested call joins and never begins; a stale append inside a unit rolls back its other writes; two racing units on one stream let exactly one win. A provider without units skips the suite (`t.Skip`)
- [ ] Concurrency: four appenders plus a reader looping `LoadAll` with a cursor (IDs deliberately out of append order); the reader sees every event exactly once, in append order. Four writers retrying on conflict lose no update. 16 concurrent readers never fail and agree

#### Step 4.8: Provider contract and the local provider's run
Files: `tarotest/provider_contract.go`, `tarotest/run.go`, `local/contract_test.go`, `README.md`
- [ ] Provider: `ProviderType`, adapters share storage, `Initialize` can run again without losing data, buses (the strong one subscribable, an unknown transport rejected), a new unit per call that hasn't begun
- [ ] `tarotest.RunAll(t, newProvider)` runs every suite as a named subtest, so a provider's test file is three lines. `tarotest.Suites()` lists them by name, and `tarotest.NewRecorder()` is a `T` that collects failures, for hosts without `go test`
- [ ] The local provider passes every suite, also with its database's clock stopped. Mutations (cursor reads by ID instead of position; a rollback that commits) each fail a suite
- [ ] README: package table with status

### Phase 5: Event sourcing (§3.3–3.4, §6, §7)

#### Step 5.1: Event registry and upcasters
Files: `event_registry.go`, `event_registry_test.go`
- [ ] `RegisterEvent[E](registry)` at the event's version. A pointer type is `INVALID_CONFIGURATION`; a second registration of the same name is `DUPLICATE_EVENT_REGISTRATION`, naming both Go types
- [ ] `RegisterUpcaster(name, from, to, func(json.RawMessage) (json.RawMessage, error))`, in any order relative to `RegisterEvent`. Upcasters apply one step at a time up to the registered version; a gap, or a stored version newer than the registered one, is `EVENT_VERSION_MISMATCH`. An upcaster from `v` to `v`, a second one from the same version, or one that would close a loop is `INVALID_CONFIGURATION`
- [ ] `Deserialize(SerializedEvent) (DomainEvent, error)`: an unknown type is `UNKNOWN_EVENT_TYPE`, and a JSON failure is `SERIALIZATION_ERROR`
- [ ] `Knows(name, version)`, `RegisteredVersion(name)` and `Names()` (alphabetical). Safe for concurrent reads after build
- [ ] Tests: the v1.0.0 → v1.1.0 → v1.2.0 chain (upcasters registered out of order), a missing step, a newer stored version, an unknown type, a JSON failure (also after an upcaster), an upcaster error, a duplicate, invalid upcasters

#### Step 5.2: EventStore wrapper
Files: `event_store.go`, `event_store_test.go`
- [ ] `EventStore` wraps an adapter and a registry. It refuses to append an event whose type isn't registered (`UNKNOWN_EVENT_TYPE`) or is registered at another version (`EVENT_VERSION_MISMATCH`). Every event is checked and serialized before the adapter is called, so one bad event stores none
- [ ] Every load method deserializes through the registry. An unreadable event fails the whole load, with `streamId` (or `correlationId`) added. Subscriptions and raw reads go through `Adapter()`, so catch-up can skip one unreadable event (Step 8.1)
- [ ] Tests on the local provider (`package tarodomain_test`): typed round trip with envelopes; `$all`, category and correlation reads; unknown type, old version and unserializable payload refused with nothing stored; events an older build stored upcast on read; two goroutines appending, one conflicting

#### Step 5.3: Repositories
Files: `repository.go`, `event_sourced_repository.go`, `state_repository.go`, `repository_test.go`
- [ ] `Repository[A]`: `Load(ctx, id)` (`AGGREGATE_NOT_FOUND`), `LoadOrNil(ctx, id) (A, bool, error)`, `Save(ctx, aggregate)`, `Exists(ctx, id)`
- [ ] `NewEventSourcedRepository(eventStore, bus, factory)`, with options `WithSnapshots(store, every)`, `OnSnapshotError(hook)`, `OnPublishError(hook)`. Load follows §6.2: a snapshot that fails to load or restore goes to the hook and falls back to full replay, and the aggregate gets the execution context of the `ctx`
- [ ] Save: append at `CommittedVersion()`, then `PublishAll`, then `MarkChangesAsCommitted`, then snapshot (non-critical) when the save crosses a multiple of `every`. A publish failure fails the save only inside a unit of work (design table)
- [ ] `StateRepository[A]` (user-implemented) and `StateStoredRepository[A]`, which wraps it with a bus
- [ ] Tests: not found, the published events are the appended ones, the crossing-a-multiple rule, snapshot plus tail replay (a doctored snapshot proves only the tail was replayed), fallback after a snapshot failure, a concurrent save conflicts and keeps its events, a publish failure outside a unit (reported) and inside one (rolled back); state-stored save, load, conflict. Mutations (exact multiple instead of crossing; always swallowing publish errors) each fail a test

### Phase 6: Commands and pipelines (§4, §11, §14, §16)

#### Step 6.1: Handlers, registry and results
Files: `command_handler.go`, `command_registry.go`, `command_result.go`, `command_registry_test.go`
- [ ] `CommandHandler[C]`: `Handle(ctx, CommandMessage[C]) error`, and `CommandHandlerFunc[C]`
- [ ] `CommandContext`: the pipeline's consistency and event bus. `RepositoryFor[A](commandContext)` for registered aggregates (`AGGREGATE_NOT_REGISTERED` otherwise), through a `RepositoryRegistry` of factories from the pipeline's bus to a `Repository[A]`
- [ ] `CommandRegistry`: `RegisterCommand[C](registry, factory, options...)` by name, with an optional consistency override; a second handler for a command is `INVALID_CONFIGURATION`. `Resolve` returns a `CommandRegistration`; `Deserialize(SerializedCommand) (AnyCommand, error)` (§4.3). An unknown command is `COMMAND_HANDLER_NOT_REGISTERED`, and data that doesn't fit is `COMMAND_VALIDATION`
- [ ] `CommandDispatchResult { CommandID, Status (Dispatched | Duplicate), Events }` and `BatchDispatchResult { Outcomes, Skipped }`
- [ ] Tests: registration, duplicate, unknown command, deserialize, a pointer command type is rejected

#### Step 6.2: ConcurrencyChecker
Files: `concurrency_checker.go`, `concurrency_checker_test.go`
- [ ] `Check(expected, actual, streamID)`, `WithRetry(ctx, op)`, `ComputeDelay(attempt)`
- [ ] Defaults: 3 retries after 10 / 20 / 40 ms, ±10% jitter, capped at 1 s. Only `OPTIMISTIC_CONCURRENCY` is retried, and the last conflict comes back unchanged. An `OnRetry` hook. The sleeper is a required argument
- [ ] Tests with `RecordingSleeper`: the delays, giving up after 3 retries, domain and infrastructure errors aren't retried, zero retries, the hook, a cancelled `ctx` stops the loop

#### Step 6.3: Pipelines and dispatcher
Files: `pipeline.go`, `dispatcher.go`
- [ ] One `CommandPipeline` type for both modes (§11.1–11.2). Strong: retry → unit of work → execution context for the command → idempotency check → fresh handler → `Handle` → mark processed, all in one unit. Eventual: the same without the unit. Inside an active unit it joins and doesn't retry on its own
- [ ] A command already processed returns `Duplicate` without running the handler. Idempotency is checked again on every attempt
- [ ] `Events` in the result come from a recording wrapper around the bus, made fresh for each attempt, so a failed attempt reports nothing
- [ ] `CommandDispatcher`: routes by the registration's consistency, else the default (§11.3); `Dispatch(ctx, command)`, `DispatchAll(ctx, commands, continueOnError)`, `CanDispatch`
- Behaviour tests come with Step 6.4

#### Step 6.4: Pipeline tests
Files: `bank_fixture_test.go`, `pipeline_test.go`
- [ ] Shared fixture domain (`package tarodomain_test`): a small bank (account aggregate, `OpenAccount` and `Deposit`) and a harness that wires the local provider, a strong bus with a read model, a failing-on-demand eventual bus, both pipelines and the dispatcher
- [ ] A forced conflict is retried and then succeeds, in both modes; giving up after 3 retries; a domain error isn't retried
- [ ] Strong mode: a failing projection rolls back the events, the projection's own write and the idempotency record, and the same command runs once the projection recovers
- [ ] A duplicate command ID is a no-op in both modes
- [ ] Eventual mode: the append survives a failing bus, and the result still reports the stored events
- [ ] A consistency override routes a command to the other pipeline; an unknown command has no handler; `DispatchAll` with and without `continueOnError`
- [ ] Mutations (no unit of work in strong mode; no idempotency check) each fail tests

### Phase 7: Projections and sagas (§12, §13, §17)

#### Step 7.1: Projections
Files: `projection.go`, `shard_projection.go`, `projection_test.go`
- [ ] `Projection` interface: `Name()`, `Routes(*ProjectionRoutes)`, and optional `InstanceID(DomainEvent) string`, `OnRebuildStart(ctx)`, `OnRebuildComplete(ctx)`
- [ ] `On(routes, handler)` for `func(ctx, Recorded[E]) error`. A second route for the same event is `INVALID_CONFIGURATION`, reported by `NewProjectionHandler`
- [ ] `ProjectionHandler` implements `EventHandler` (§12.1): skip if processed; route by name; an unknown event is marked processed and reported to an `OnUnhandled` hook; a failed event is not marked
- [ ] `ShardProjection`: FNV-1a 32-bit (`hash/fnv`) of the shard key modulo the shard count. The handler is named `<name>-shard-<i>` and records events under the projection name with the shard as instance (§12.3)
- [ ] Tests (on a fake `ProcessedEventStore`): a redelivered event is skipped, an unknown event is marked and reported, a failed event isn't marked and runs again, a duplicate route, FNV-1a reference vectors, each event is handled by exactly its shard, zero shards is rejected

#### Step 7.2: Saga state machine and compensations
Files: `saga_state_machine.go`, `compensation.go`, `saga_state_machine_test.go`, `compensation_test.go`
- [ ] `SagaState` (`Initial`, `Running`, `Compensating`, `Completed`, `Failed`), `HandlerKind`, and `Transition()` for every §13.2 row. `Compensating` + `startsWhen`, which the table leaves out, is `INVALID_SAGA_TRANSITION`
- [ ] `CompensationStack`: a stack of names with `Push`, `Pop` and `TakeAll` (LIFO), serializable. The factories live in the route table and run at compensation time
- [ ] Tests: every state × kind cell, LIFO order, JSON round trip

#### Step 7.3: Process manager
Files: `process_manager.go`, `process_manager_test.go`
- [ ] `ProcessManager[S]` interface: `SagaType() string` and `Routes(*SagaRoutes[S])`. `S` is the JSON-serializable state
- [ ] `StartsWhen`, `ContinueWhen`, `CompensateWhen`, `CompletedWhen` and `DefineCompensation` on `*SagaRoutes[S]`. Priority is completed > compensate > continue > starts. A duplicate handler (same kind and event) and a saga without `StartsWhen` are rejected
- [ ] `SagaCtx[S]`: the state, `RegisterCompensation(name)`, `CompensationCommands()`, `PopCompensation()`
- [ ] `SagaInstance[S]`: saga ID, state, status, current step, failure reason, compensations. `Handle(DomainEvent) ([]AnyCommand, error)` follows §13.1: the commands carry the saga ID as correlation and the event as cause, with IDs derived from `"<saga type>/<event id>"`, the command's index and the event's `OccurredAt`
- [ ] `ToSaveInput()` and `Restore(SagaRecord)` store the state together with the compensation names (`{ "state": …, "compensations": […] }`, as the Rust port does)
- [ ] Tests: derived IDs and context, the state machine through `Handle`, a save/restore round trip, priority

#### Step 7.4: Saga event handler
Files: `saga_event_handler.go`
- [ ] The §13.3 flow, steps 1–9. Commands go through a `SagaCommandDispatch` function, so the rebuilder can pass one that discards them
- [ ] An event for a completed or failed saga is recorded and skipped, not an error. An event a running saga has no handler for is skipped without saving, so it doesn't push back the timeout
- [ ] Per-saga mailbox carried in the `ctx` (design table). Events that wait in the mailbox are dropped if the saga's handling fails: the failure reaches the publisher, which retries or leaves them to catch-up
- Behaviour tests come with Step 7.5

#### Step 7.5: Projection and saga tests
Files: `bank_fixture_test.go`, `transfer_fixture_test.go`, `projection_integration_test.go`, `saga_test.go`
- [ ] Fixtures: the bank gains `Withdraw` (raising `WithdrawalRejected` when funds are short); `transfer_fixture_test.go` has the `MoneyTransfer` aggregate and `TransferSaga`; the harness wires the saga on the strong bus, delivers queued events to it in eventual mode, and has a crash switch that fails a saga dispatch after its commands commit
- [ ] Projections: a redelivered event is skipped, an unknown event is marked, a failing handler is not marked, shards split deterministically
- [ ] TransferSaga end to end in both modes: happy path; one- and two-step compensation in reverse order; a rejected first step; a redelivered event whose commands are skipped as duplicates; a crash between the withdrawal commit and the saga save moves the money only once; a strong-mode rollback of the whole saga
- [ ] Every invalid transition of §13.2 through the handler
- [ ] Mutation: without the mailbox, saga tests fail

### Phase 8: Catch-up and operations (§18–21, §28.3)

#### Step 8.1: Subscriptions and cron dispatcher
Files: `subscription_manager.go`, `cron_dispatcher.go`, `catch_up_test.go`
- [ ] `SubscriptionManager` (§18.2), including `GetLagReport(countLimit)`. `Register` of a name on another stream is `INVALID_CONFIGURATION`
- [ ] `CronDispatcher`: `Register(handler, stream)`, `CatchUp(ctx, name) CatchUpResult { Processed, DeadLettered, HasMore, Failed }`, `RunAll(ctx)`. `HasMore` is exact (it reads one event past the batch)
- [ ] It reads raw events and deserializes them one at a time, so an unreadable event is dead-lettered instead of blocking the batch
- [ ] A retryable error stops the subscription until the next run and counts an attempt. A non-retryable one is dead-lettered and the cursor moves past it. A failed dead-letter write stops the run, so the cursor stays put. A cancelled `ctx` stops the run without counting an attempt
- [ ] Options: units of work (each event's handler and cursor move commit together), `MaxAttempts(n)`, `BatchSize(n)` (default 100)
- [ ] Tests on the local provider: batches and `HasMore`, a category stream, a retryable stop and resume, a non-retryable skip, `MaxAttempts`, an unreadable event, units of work, lag report, cancellation. Mutations (unreadable events left to their category; no unit of work) each fail a test

#### Step 8.2: Backpressure
Files: `backpressure.go`, `backpressure_test.go`
- [ ] `BackpressureController` (§18.4): one batch from each subscription in turn (name order), a time budget (default 25 s), an event budget (default 10 000), passes per subscription (default 100), a throttle after batches that leave more, and an `OnProgress` callback
- [ ] A subscription leaves the rotation when it catches up, fails or reaches its pass limit. `StopReason`: `Completed`, `TimeBudget`, `EventBudget`, `PassLimit`
- [ ] Tests with `ManualClock` and `RecordingSleeper`: each budget stops the run, round-robin order, the pass limit and resuming, the throttle, progress, a failing subscription leaves the rotation

#### Step 8.3: Retry policy and dead-letter writer
Files: `retry_policy.go`, `dead_letter_writer.go`, `retry_policy_test.go`, `dead_letter_writer_test.go`
- [ ] `RetryPolicy` (§21.3): 3 retries after 100 / 200 / 400 ms with ±10% jitter, only for retryable errors. `Run` also returns the attempt count
- [ ] `DeadLetterWriter.ExecuteWithRetry` (§21.2) dead-letters after the last attempt, or at once for a non-retryable error, and returns the operation's error. If the dead-letter write also fails, it returns a `DEAD_LETTER` error with `originalError` in its context, so the caller doesn't ack
- [ ] The cron dispatcher (Step 8.1) may now write through the writer; if it changes, `cron_dispatcher.go` is this step's fifth file
- [ ] Tests: the delays, a non-retryable error returned at once, success after a transient failure, jitter bounds, cancellation is neither retried nor dead-lettered; the writer's entries, a failed write

#### Step 8.4: Saga timeouts
Files: `saga_timeout.go`, `saga_timeout_test.go`
- [ ] `SagaTimeout` event (`SagaType`, `SagaID`, `LastStep`) with the saga ID as correlation ID, and its ID and timestamp derived from the saga's type, ID and `UpdatedAt`. It is never stored, so it isn't registered
- [ ] `SagaTimeoutScheduler[S]`, one per saga type with its own timeout (§19.3): `Run(ctx) TimeoutRun { Examined, Handled, Failed, RetryLater }`. A `RUNNING` saga with a `CompensateWhen` route for `SagaTimeout` gets the event through its handler; one without is failed with the reason `"timeout"`; one already `COMPENSATING` is failed without calling the handler (design table)
- [ ] A non-retryable error from the handler fails the saga. A retryable one leaves it for the next run, with the same event and command IDs
- [ ] Unit tests: the event's envelope, its ID depends on the saga and `UpdatedAt`. Behaviour tests are in Step 8.7

#### Step 8.5: Rebuilders
Files: `projection_rebuilder.go`, `saga_rebuilder.go`, `cron_dispatcher.go`, `projection.go`
- [ ] `ProjectionRebuilder` (§20.1): `Rebuild`, `RebuildAll`, `RebuildShards`, with `RebuildOptions { ContinueOnError, ResetIdempotency, CallHooks, BatchSize, MaxPasses }`. By default a failing event stops the rebuild. Under `ContinueOnError` the cursor moves past the failed event without a dead letter and it is listed in `Skipped`
- [ ] `SagaRebuilder` (§20.2): `RebuildInstance`, `RebuildType` (finds sagas by scanning the raw log for their start events), `RebuildAll`, with a dispatch that discards commands
- [ ] Small additions to `cron_dispatcher.go` (a batch size per call) and `projection.go` (a fixed instance for a shard)
- [ ] Limits, in doc comments: `SagaTimeout` events aren't stored, so a timed-out saga comes back in the state its last stored event left it in

#### Step 8.6: Correlation trace
Files: `trace.go`, `trace_test.go`
- [ ] `CorrelationTrace.Trace` (raw events, so an unreadable one still shows), `BuildTree` and `FormatTree` (§28.3)
- [ ] The tree infers command nodes between events through `CausationID` and `CommandCausationID`. `BuildTree` returns a `CausationTree { Roots }`, since independent commands under one correlation ID have several roots. Cyclic data from a bad store doesn't loop
- [ ] Tests: a single chain, several roots, the saga chain, a cycle, the text form

#### Step 8.7: Operations tests
Files: `rebuild_test.go`, `timeout_integration_test.go`, `catch_up_test.go`, `transfer_fixture_test.go`
- [ ] A projection rebuilt in place, or fresh from the store, matches the live one after a transfer saga, in both modes; also shards, `RebuildAll`, `ContinueOnError`, `BatchSize` and `MaxPasses`, and no replay without `ResetIdempotency`
- [ ] A timed-out saga compensates (the refund runs), and fails when it times out again; one without a timeout handler is failed; a crash while saving the compensating saga, followed by a retry, refunds only once
- [ ] A handler that keeps failing is dead-lettered after `MaxAttempts`
- [ ] Sagas are rebuilt as they were (completed, compensating) without new events or money moving
- [ ] The transfer saga traces as a single chain from the root command to `TransferCompleted`, in both modes
- [ ] Fixtures: `TransferSaga` gains a `SagaTimeout` route, and a `QuietSaga` has none
- [ ] Mutations (a compensating saga sent to the handler; no idempotency reset on rebuild) each fail tests

### Phase 9: DI, builder, validation, runtime and HTTP API (§23, §25, §26.1, §29)

#### Step 9.1: DI container
Files: `di.go`, `di_test.go`
- [ ] `Token[T]` and `Container` with `Bind`, `BindSingleton`, `BindTransient`, `BindScoped`, `BindInstance`, `Resolve`, `ResolveOrNone`, `Has` and `CreateScope` (§23.2)
- [ ] A singleton is created with the container it was bound on; scoped and transient values with the container they are resolved from
- [ ] `MISSING_DEPENDENCY`; `CIRCULAR_DEPENDENCY` with the path, detected across scopes
- [ ] No lock is held while a factory runs; a singleton raced by two goroutines keeps the first result
- [ ] Tests (with `-race`): every lifetime, singleton owner vs. scope, later bindings seen by scopes, interface-typed tokens, cycle path, a failed factory isn't cached

#### Step 9.2: Validation
Files: `validation.go`, `validation_test.go`
- [ ] `FrameworkValidator` (§29.1) reports every error at once in one `INVALID_CONFIGURATION`: duplicate aggregates, commands, sagas or projections; a state aggregate without a repository; a saga without `StartsWhen`; a projection that routes no event; a non-positive timeout; a handler for an unregistered event; `SnapshotEvery` on an aggregate that isn't a `Snapshotter`
- [ ] Warnings go to a callback: a saga that never completes, a `SagaTimeout` handler without a timeout
- [ ] `DispatchGuard` (§29.2): `ValidateCommand`, `ValidateEvent`, and `ValidateProjectionDispatch` in `Skip`, `Warn` or `Strict` mode (`MISSING_EVENT_HANDLER`)
- [ ] Tests: each rule alone, several problems reported together, the three guard modes

#### Step 9.3: Builder
Files: `builder.go`, `aggregate_config.go`
- [ ] The §25.1 DSL: `NewBuilder()`, `UseProvider`, `Consistency(mode, transport)`, `SnapshotEvery`, `Validation(mode)`, `SkipValidation`, plus `OnWarning` (default `slog`), `Concurrency`, `Clock`, `Sleeper`, `CatchUpBatchSize`, `CatchUpMaxAttempts`
- [ ] Generic registration functions: `AddAggregate[A](b, factory, options...)` with `EventSourced()`, `StateStored(repositoryFactory)`, `WithSnapshotEvery(n)` and `WithEvents(prototypes...)`; `AddEvent[E](b)`; `AddUpcaster`; `AddCommand[C](b, handlerFactory, options...)`; `AddProjection(b, factory)`; `AddSaga[S](b, factory, SagaOptions{Timeout})`; the `Bind…` functions on `b.DI()`
- [ ] Registration errors are collected and returned by `Build`, so calls can be chained without checking each one
- [ ] The default consistency is **strong** (a provider always has the stores it needs; eventual needs a queue)
- `Build` is in `framework.go` (Step 9.4)

#### Step 9.4: Runtime
Files: `framework.go`, `framework_test.go`
- [ ] `Build(ctx) (*Framework, error)`: initializes the provider and wires two buses, two repositories per aggregate, two pipelines, projections and sagas on the strong bus, the cron dispatcher, the timeout schedulers, and a container with the framework and provider bound
- [ ] `Dispatch`, `DispatchAll`, `HandleEvents(ctx, []QueueMessage)` (§10.5, with an `Ack` / `Retry{Delay}` / `DeadLettered` outcome per message), `RunCatchUp`, `RunTimeouts`, `RunScheduled` (both), `ConsistencyFor[C](f)`, and accessors for `DI`, `EventStore`, both buses, `Commands` and `Cron` (§25.2)
- [ ] `RunCleanup(ctx)`: deletes processed commands older than 7 days, and processed events and dead letters older than 30 (§14, §21). `Migrate(ctx)` runs `Provider.Initialize` again
- [ ] A provider without a unit of work, when any command needs strong consistency, is `INVALID_CONFIGURATION`
- [ ] Projections and sagas also register with the cron dispatcher, so catch-up delivers what the queue didn't. Every call runs with the framework's clock in its `ctx`
- [ ] Tests: strong and eventual flow, queue consumer retry and dead letter, catch-up without double counting, cleanup with a manual clock, validation, no unit of work, strict projections

#### Step 9.5: HTTP API
Files: `http_api.go`, `http_api_test.go`
- [ ] `NewHTTPAPI(framework)` handles a plain `HTTPRequest { Method, Path, Headers, Body }` and returns `HTTPResponse { Status, Headers, Body }`. It doesn't import `net/http`
- [ ] The §26.1 routes: `POST /commands/{name}` (JSON body through `CommandRegistry.Deserialize`; `x-request-id` becomes the correlation ID and `x-command-id` the command ID), `GET /health`, `POST /migrate` (off unless `AllowMigrate(true)`)
- [ ] Errors are rendered with `ToHTTPResponse` (§22.3). An unknown route or command is 404, and a wrong method is 405
- [ ] Tests on the local provider: a dispatch, a retried request is `duplicate`, a domain error with its context, an infrastructure error without it, migrate off by default

#### Step 9.6: Bank example, domain
Files: `examples/go.mod`, `examples/bankaccount/account.go`, `examples/bankaccount/transfer.go`, `examples/bankaccount/account_test.go`
- [ ] A nested module (`…/tarodomain-go/examples`) with a `replace` directive to the repo root; `make check` picks it up
- [ ] `BankAccount` (open, deposit, withdraw, which raises `WithdrawalRejected` when funds are short) and `MoneyTransfer` (request, complete, fail), with typed IDs, commands and handlers
- [ ] A custom domain error code (`INSUFFICIENT_FUNDS`), plus `INVALID_AMOUNT` and `ACCOUNT_ALREADY_EXISTS`
- [ ] Events carry their account ID, because an event doesn't know its stream and a projection needs the key
- [ ] Tests: the aggregates' rules, and `AssertAppliesAll` on both

#### Step 9.7: Bank example, saga, projection and scenario
Files: `examples/bankaccount/saga.go`, `examples/bankaccount/projection.go`, `examples/bankaccount/register.go`, `examples/bankaccount/bank_test.go`, `examples/cmd/bank/main.go`
- [ ] `TransferSaga` with a `refund` compensation and a `SagaTimeout` handler; `BalanceProjection` over a small `BalanceStore` interface with idempotent upserts and increments (§14.3) and an in-memory implementation
- [ ] `Register(builder, store)` registers everything; the caller picks the provider and mode
- [ ] `go run ./cmd/bank` runs the spec scenario: open, deposit, transfer saga, projection query, correlation trace
- [ ] Tests in both modes, including a failed transfer, a projection rebuild and the timeout refund

### Phase 10: HTTP adapter (§26)

#### Step 10.1: `net/http` handler
Files: `tarohttp/handler.go`, `tarohttp/handler_test.go`
- [ ] `tarohttp.Handler(framework) http.Handler` (and `HandlerFor(api)`, to allow `POST /migrate`): converts the request and response, so the routes, the `x-request-id` handling and the error rendering stay in the core. The request's `Context()` is passed through, so a client disconnect cancels the dispatch
- [ ] Request bodies over 1 MiB are `413` and non-UTF-8 bodies `400`, in the core's error shape
- [ ] Builds for `js/wasm`: the Cloudflare adapter serves this handler
- [ ] Tests with `net/http/httptest`

### Phase 11: Cloudflare provider, `cloudflare` (§24.2–24.3, §26.1, §27.2)

Node.js and wrangler join the dev image in Step 11.8. Every D1 adapter step is tested natively on SQLite through the backend of Step 11.2, and Step 11.8 runs the same suites on D1 itself. Files named `*_js.go` carry `//go:build js && wasm`; everything else also builds natively, so `go test` covers the adapter's logic.

#### Step 11.1: Module and platform glue
Files: `cloudflare/go.mod`, `cloudflare/doc.go`, `cloudflare/await_js.go`, `cloudflare/values.go`, `cloudflare/values_test.go`
- [ ] A nested module (`…/tarodomain-go/cloudflare`) with a `replace` directive to the repo root, requiring `workers-go` at a pinned version. Check its current release and Go requirement when adding it (v0.36.0 on pkg.go.dev was already not the latest on 2026-11-17)
- [ ] `await(ctx, promise)`: parks the goroutine until a JavaScript promise settles, returns a rejected promise as an infrastructure error, and releases its `js.Func` callbacks
- [ ] `values.go`: the conversions between Go values and what D1 binds and returns (text, integers, floats, NULL), as pure functions over `any`, so they are tested natively; `await_js.go` holds only the `syscall/js` part
- [ ] The module builds natively and for `js/wasm`

#### Step 11.2: Statement layer
Files: `cloudflare/backend.go`, `cloudflare/d1.go`, `cloudflare/d1_backend_js.go`, `cloudflare/internal/sqlitetest/backend.go`, `cloudflare/d1_scan_test.go`
- [ ] `Backend` interface (`Run`, `All`, `Batch`) over `Statement { SQL, Args }`, with two implementations: the D1 binding through `syscall/js` (`prepare` / `bind` / `run` / `all` / `batch`), and SQLite through `database/sql` for native tests, where `Batch` is one transaction. The SQLite driver is a pure-Go one, imported only by `internal/sqlitetest`, so a Worker build never links it; check its current version when adding it
- [ ] `DB` is the only code that runs statements. Reads run at once. Writes run at once outside a unit of work and are staged inside one (§11.4)
- [ ] Multi-row inserts are split to fit D1's bound-parameter limit per statement. Check the current limits in Cloudflare's documentation here
- [ ] A constraint violation in a batch comes back as a typed error the adapters can map
- [ ] Source-scan test: no other file in the package calls a backend

#### Step 11.3: Unit of work and schema
Files: `cloudflare/unit_of_work.go`, `cloudflare/overlay.go`, `cloudflare/migrate.go`, `cloudflare/migrations/0001_init.sql`, `cloudflare/unit_of_work_test.go`
- [ ] `UnitOfWork`: `Begin`, `Commit` (every staged statement in one `Batch`), `Rollback` (drops them), a nested unit joins, after-commit callbacks that run only after a successful commit
- [ ] The overlay: staged events per stream, staged saga records, and staged processed-event and processed-command markers, readable by the adapters until the unit ends (design table)
- [ ] A constraint violation at commit is `OPTIMISTIC_CONCURRENCY` (with the stream and both versions) when another writer appended to a stream the unit appended to
- [ ] Schema: copy `crates/tarodomain-cloudflare/migrations/0001_init.sql` from the Rust repo (nine `tarodomain_` tables, following §27.2) and compare the two files before ticking this step. The Rust repo isn't mounted in the dev container, so the copy and the comparison are done on the host with plain file tools. `schema_migrations` is created by the migrator itself
- [ ] `migrate.go` embeds the SQL, applies each missing migration in one batch with its record, and is a no-op the second time. When two isolates migrate at once, the loser checks again instead of failing. Also `Tables` and `ClearAll` for the contract harness
- [ ] Tests on SQLite: commit, rollback, reads through the overlay, a conflict at commit, callbacks, migrating twice and concurrently

#### Step 11.4: D1 event store adapter
Files: `cloudflare/event_store.go`, `cloudflare/rows.go`, `cloudflare/event_store_test.go`
- [ ] Append in one batch, or staged in the active unit: a guard statement, the events with their stream versions, and the stream row. A conflict is `OPTIMISTIC_CONCURRENCY` with the stream, the expected version and the actual one (read again)
- [ ] Stream, category and `$all` reads ordered by position. An `afterEventID` cursor resolves to its position, or to an ID comparison if its event was deleted
- [ ] Inside a unit, stream reads, cross-stream reads, correlation reads and stream versions include the overlay's staged events
- [ ] Subscriptions: idempotent subscribe, ack only moves forward (one `UPDATE`, in append order), `RecordFailedAttempt`
- [ ] `rows.go`: typed column readers shared by every adapter
- [ ] Tests on SQLite

#### Step 11.5: Other D1 adapters
Files: `cloudflare/snapshot_store.go`, `cloudflare/saga_store.go`, `cloudflare/idempotency.go`, `cloudflare/dead_letter.go`
- [ ] Idempotent writes with `ON CONFLICT DO NOTHING` / `DO UPDATE` (§14.3)
- [ ] Inside a unit, saga loads and idempotency checks read the overlay first
- [ ] Deletes return their counts, and `DeadLetterStore.Write` its ID. A dead letter written inside a unit is staged and returns `0`, because its ID isn't known before the commit
- [ ] The same semantics as the in-memory adapters, checked by the contract suites in Step 11.6

#### Step 11.6: Queue event bus and provider
Files: `cloudflare/queue_bus.go`, `cloudflare/queue_js.go`, `cloudflare/provider.go`, `cloudflare/provider_js.go`, `cloudflare/contract_test.go`
- [ ] `QueueEventBus`: `PublishAll` sends the serialized events as JSON text in batches of 100 through an `EventQueue` interface (`workers-go`'s producer in `queue_js.go`, a recording queue in tests). Inside a unit of work it sends from an after-commit callback, so a rolled-back batch never reaches the queue. An event over the queue's message size limit is logged and left to the cron catch-up. A failed send is logged, not returned (§11.2)
- [ ] `cloudflare.NewProvider(Bindings{D1, Queue})` (`provider_js.go`; the binding names, the queue optional) and `NewProviderWithBackend(backend, queue)` for native tests. `ProviderType()` is `"cloudflare"`; the eventual bus is the queue bus and the strong bus an `InMemoryEventBus`; eventual without a queue binding is `INVALID_CONFIGURATION`
- [ ] `Initialize` applies missing migrations once per provider (a Worker keeps its provider for the isolate)
- [ ] `provider.DB()` gives applications the same `DB`, so read-model writes join the unit's batch in strong mode
- [ ] `contract_test.go`: every contract suite passes on SQLite (`tarotest.RunAll`)

#### Step 11.7: Worker entry points
Files: `cloudflare/worker.go`, `cloudflare/worker_js.go`, `cloudflare/lease.go`, `cloudflare/worker_test.go`
- [ ] `cloudflare.Serve(framework, provider, options)` (`worker_js.go`): registers the queue consumer and the cron task without blocking, then serves the `tarohttp` handler through `workers.Serve` (design table)
- [ ] Queue: runs `HandleEvents` on the batch, then per message `Ack`, or `Retry` with the delay from `RetryPolicy` and the message's attempt count (whole seconds, at least one). A non-retryable error was already dead-lettered by `HandleEvents`, so its message is acknowledged; a body that isn't an event is logged and acknowledged (§10.5)
- [ ] Scheduled: the cron expression picks `RunScheduled` (catch-up, then timeouts) or `RunCleanup` (`CleanupCron`, daily at 03:00 UTC by default). Each job holds a lease in `tarodomain_locks` (5 minutes at most), so an overlapping run skips
- [ ] `worker.go` holds the pure parts (outcome → ack or retry, cron selection), tested natively with the leases: lease expiry and takeover, a held lease skipping a run, a garbage message
- [ ] The backpressure time budget stays the core's 25 s default; check it against the Workers CPU limits for Cron Triggers when implementing

#### Step 11.8: Contract harness in workerd
Files: `cloudflare/harness/main.go`, `cloudflare/harness/wrangler.toml`, `cloudflare/harness/package.json`, `scripts/cloudflare-contract.sh`, `Dockerfile.dev`
- [ ] A Worker (`main` package, `js/wasm` only) with `POST /suites/{name}`: it empties the local D1 (`ClearAll`, before each case), runs one `tarotest` suite against the Cloudflare provider with a `tarotest.NewRecorder()`, and answers 200, or 500 with the failures as JSON
- [ ] `wrangler.toml` and `package.json` follow `workers-go`'s standard Go template for the build (compile to wasm, copy the runtime assets); copy them from the template at the pinned version
- [ ] `scripts/cloudflare-contract.sh` builds the harness, starts `wrangler dev` (local mode, no Cloudflare account needed), calls every suite, and exits non-zero on any failure
- [ ] `Dockerfile.dev` gains a pinned Node.js LTS and wrangler (installed in the image, with npm's cache in a named volume). `node_modules` must not land in the mounted repo from the host's point of view as root-owned files; check after the first install
- [ ] Every suite passes on D1 itself, run as `./dev scripts/cloudflare-contract.sh`, confirming the SQLite runs of Step 11.6. workerd and the local D1 run inside the container; no Cloudflare account is needed

#### Step 11.9: Example read model on D1
Files: `examples/go.mod`, `examples/d1balances/store.go`, `examples/d1balances/store_test.go`
- [ ] The examples module adds the `cloudflare` module (a second `replace` directive)
- [ ] A D1 `BalanceStore` that writes through `provider.DB()` with upserts and increments (`balance = balance + ?`, §14.3), never read-then-write
- [ ] Tests natively on SQLite, including a strong-mode run in which the projection commits with the events and rolls back with them

#### Step 11.10: Example worker
Files: `examples/cmd/bank-worker/main.go`, `examples/cmd/bank-worker/wrangler.toml`, `examples/cmd/bank-worker/package.json`, `examples/cmd/bank-worker/README.md`, `compose.yaml`
- [ ] The bank domain behind `cloudflare.Serve`, with the D1 balance store. The framework is built once per isolate (`sync.Once`); `GET /balances` shows the read model; a `CONSISTENCY` variable picks strong or eventual
- [ ] `wrangler.toml`: the D1 binding, the queue as producer and consumer (with a Cloudflare dead-letter queue as a last resort), and two cron triggers (catch-up every minute, cleanup daily). It is the template to copy
- [ ] README: `npx wrangler dev`, a curl walkthrough in which an eventual deposit reaches the projection through the queue, and deploying (`wrangler d1 create`, `wrangler queues create`, `wrangler deploy`)
- [ ] `compose.yaml` (this step's fifth file) publishes the dev service's port 8787 on the host's loopback only, and the walkthrough starts `wrangler dev --ip 0.0.0.0` in the container, so curl on the host reaches it
- [ ] Deploying needs Cloudflare credentials: `CLOUDFLARE_API_TOKEN` is passed into the container from the host's environment and never written to the repo or the image
- [ ] Run the walkthrough under `wrangler dev`
- [ ] Record in the README: the wasm module's size, its startup time and the CPU time of one dispatch, against the Workers limits (Risks: Go on Workers). If any is too close, try TinyGo and record the result

#### Step 11.11: Workers CI
Files: `.github/workflows/ci.yml`, `README.md`
- [ ] A `workers` job: the dev image, then `scripts/cloudflare-contract.sh` in it
- [ ] README: package table status, and a Cloudflare development section
- [ ] CI green

### Phase 12: Documentation

#### Step 12.1: Guides
Files: `README.md`, `docs/GETTING_STARTED.md`, `docs/CLOUDFLARE.md`, `example_test.go`, `doc.go`
- [ ] Getting started: define events, an aggregate, commands, a projection and a saga; build; dispatch
- [ ] Cloudflare: bindings, migrations, strong and eventual mode on D1 and Queues, read models through `provider.DB()`, cron schedules, D1 and Workers limits, building and deploying a Go Worker
- [ ] Writing another adapter: implement `Provider` in a nested module and pass `tarotest.RunAll`
- [ ] Using the module from another repository: `go get` at a tag, and `GOPRIVATE` while the repo is private
- [ ] `example_test.go`: a package example that `go test` compiles and runs, so the guide's core snippet can't rot
- [ ] README: package table, verification status, what is not verified
- [ ] `go doc` output reads well for the root package: the package comment maps the files to the spec's layers

## Risks

### Everything depends on the dev container

No tool is installed on the host, so until Step 0.3 is done nothing can be compiled, and the skeleton of Step 0.1 never has been. Three things to watch. Kiri is pre-1.0; the repo depends only on the Docker Engine API, so another engine runs the same files after a `docker context use`. Bind-mounted source can be slow on macOS, and the Go caches must stay in named volumes, not in the mount. And `wrangler dev` inside a container has to listen on all interfaces and have its port published, or the host can't reach it (Step 11.10); workerd also needs a glibc image, which is why the base is Debian. The engine reported `linux/amd64` on 2026-11-17; if the laptop is Apple Silicon, check that the Go and workerd binaries in the image run natively and not under emulation.

### Go on Workers is not a first-party target

Cloudflare supports JavaScript, TypeScript, Python and Rust directly; Go runs as WebAssembly behind community glue (`workers-go`), which describes itself as experimental and its D1 package as alpha. Three things follow. The glue's API may change, so the adapter pins a version and keeps its use of it in the `*_js.go` files. A Go wasm module carries the Go runtime, so its size, its start-up (Workers allow one second) and its CPU time per request (10 ms on the Free plan, far more on Paid) have to be measured, not assumed: Step 11.10 records them, and TinyGo is the fallback. And the Go garbage collector lives inside the isolate's 128 MB. Limits as read from Cloudflare's documentation on 2026-11-17; check them again when implementing.

### D1 has no interactive transactions (Steps 11.2–11.6)

D1 can't hold a transaction open across awaits; only one `batch` is atomic. So the unit of work stages every write and sends them in one batch at commit, and anything read inside the unit sees the database as it was before the unit began. **Resolution (from the Rust port):** the overlay covers what the framework reads back inside a unit: stream events and versions, saga records, and idempotency markers. Read models are the application's: on D1 they must write through `provider.DB()`, so their writes join the batch, and use upserts and increments instead of read-then-write. The unit of work contract and the strong-mode transfer saga on D1 both cover it.

### D1 limits

D1 limits bound parameters per statement, statements per batch, queries per Worker invocation and response size, and those limits differ by plan and change over time. A strong-mode chain (a saga dispatching several commands) produces one large batch. **Resolution:** split multi-row inserts (Step 11.2), add a contract test with a large append, and check the current limits when implementing Phase 11. Strong mode on D1 suits short chains; long sagas should run eventual.

### The contract suites must run without `go test`

Inside workerd there is no test binary, so the suites can't depend on `*testing.T`. **Resolution:** the `tarotest.T` interface (Step 4.5). It has to be settled in Phase 4, long before the harness uses it in Step 11.8; changing it later means touching every suite.

### A `ctx` that outlives its unit of work

The unit of work travels in the `ctx`. A goroutine started inside a dispatch that keeps the `ctx` will stage writes on a unit after it committed, and those writes are lost. **Rule:** the framework never starts a goroutine inside a dispatch. A provider returns a clear `INVALID_CONFIGURATION` error when a write arrives on a finished unit, and the doc comment on `ExecuteInUnit` says to detach (`context.WithoutCancel` plus a fresh unit) for background work.

### A write that forgets the unit of work

An adapter that runs a write on the backend directly, instead of through `DB`, writes at once instead of joining the batch, so a rollback leaves it behind. **Resolution (from the Rust port):** one way in (`cloudflare/d1.go`, Step 11.2), a source-scan test that enforces it, and the unit of work contract's rollback test, which covers the writes of every store.

### No exhaustiveness check on `Apply`

A Rust `match` over the event enum fails to compile when an event is missing; a Go type switch doesn't. A forgotten case shows up as `MISSING_EVENT_HANDLER` when the event is raised or replayed. **Mitigation:** `AssertAppliesAll` (Step 2.5), which a domain's tests call with every event of the aggregate. A build-time check that applies each event listed in `WithEvents` to a fresh aggregate would catch more, but it breaks aggregates whose `Apply` assumes earlier events, so it is not planned.

### Events are values, but slices and maps are shared

Passing an event by value copies its fields, not what its slices and maps point to. A handler that appends to an event's slice can change what the next handler sees. **Rule:** event fields are read-only after `Raise`; the doc comment on `Event` says so, and the bank example uses only scalar fields.

### Numbers through `encoding/json`

Decoding into `any` turns every number into `float64`, which loses precision above 2^53. Metadata and error context decode with `json.Number`; event data is decoded into the event's own struct, so its integer fields are exact. Money in the examples is an integer of minor units.

### Import cycles

`local` and `tarotest` import the core, so a test in `package tarodomain` can't import them. **Rule:** tests that need a provider are in `package tarodomain_test`. The shared bank fixture is therefore also in the external test package.

### One large package

The core will be around 55 source files in one package. Unexported helpers are visible across layers, so nothing but review stops the domain files from calling persistence code. If that becomes a problem, the layers can move under `internal/` with the root package re-exporting them; doing it later doesn't change the public API.

### UUIDv7 order within one millisecond

Two UUIDv7s from the same millisecond can sort either way, and other servers' clocks disagree. On Workers the clock only advances after I/O, so every ID from one stretch of computation shares a millisecond. Nothing relies on that order: per-stream order comes from the stream version and global order from the append position. Step 4.5 has a contract test with two same-millisecond IDs that sort in reverse.

## Resolved questions

Decided on 2026-11-17:

1. **Adapters.** TaroDomain has platform adapters, each in its own module, and the one to implement now is Cloudflare (Phase 11). A SQL provider and AWS stay out of scope.
2. **Package layout.** One flat `tarodomain` package for the core. If the layering needs the compiler's help later, the layers can move under `internal/` without changing the public API (Risks: one large package).
3. **Schema.** The D1 schema is the Rust port's, table for table, so one database can be served by either port.
4. **Compiler.** Standard Go for the Worker, with TinyGo as the measured fallback (Step 11.10).
5. **Tooling.** All tools run in the dev container on Kiri; the host has only a container engine, git and an editor.

No open questions right now.
