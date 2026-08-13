# Pluggable Scaffold Providers and JWT Authentication Design

## Status

This document is the implementation contract for the next scaffold iteration.
It changes generated-project behavior but does not change this repository by
itself. The implementation is intentionally scoped to generation-time choices;
it does not introduce runtime plugins or a dependency-injection framework.

## Context

`go-backend-kit` currently has two distinct layers:

- the root module is a Cobra/YAML code generator; and
- generated projects are fixed to Echo v5, GORM, PostgreSQL, Atlas, `log/slog`,
  and a pure-Go SQLite driver used by contract tests.

The fixed stack is a good baseline, but it cannot currently create a
zero-external-service SQLite application, a Fiber application, or a project
with optional Redis, NATS, logging backends, or JWT verification.

The generator already protects resource output with a digest manifest, while
base scaffold files such as `go.mod`, `cmd/api/main.go`, Compose, and database
wiring are rendered only by `gobackend new`. That ownership boundary determines
the supported meaning of "pluggable" in this iteration: providers are selected
when a project is created and are then statically compiled into that project.

## Decisions at a glance

- Provider selection happens in `gobackend new`, not on each request.
- The generated binary contains only the selected implementations.
- There is no use of Go's `plugin` package, reflection, string-keyed runtime
  registries, service locators, or dependency-injection containers.
- Echo and Fiber get separate generated HTTP code. There is no universal web
  framework interface.
- GORM remains the only ORM in this iteration. The generator does not invent a
  generic `ORM`, `Repository[T]`, or CRUD interface before a second persistence
  implementation has concrete requirements.
- SQLite and PostgreSQL are real runtime choices with dialect-specific setup,
  migrations, and contract tests.
- Redis and NATS are optional concrete clients. The scaffold does not invent
  cache or message-bus interfaces until generated business code consumes a
  specific behavior.
- Application logging remains the standard `*slog.Logger` API. The selected
  logging provider changes its handler/backend, not every call site.
- JWT support verifies externally issued bearer tokens. It does not create a
  user table, password login, refresh tokens, token issuance, or RBAC.
- Provider changes in an existing generated project are outside this scope.
  They require a separate migration/reconfiguration design because database
  migrations and base scaffold files may already contain user changes.

## Goals

- Make the following choices available when creating a generated project:
  - HTTP: Echo v5 or Fiber v3;
  - database: pure-Go SQLite or PostgreSQL;
  - cache: none or Redis;
  - messaging: none or NATS;
  - logging: standard slog, zap-backed slog, or zerolog-backed slog; and
  - authentication: none or JWT verification.
- Keep GORM runtime access and the official GORM CLI field-helper workflow.
- Generate simple concrete Go code with explicit construction and shutdown.
- Preserve `context.Context` through HTTP, authentication, GORM, Redis, and
  NATS I/O.
- Preserve current CRUD/OpenAPI/error behavior for equivalent selections.
- Pin one tested dependency bill of materials per generator release rather than
  resolving `@latest` during project creation.
- Keep handwritten files outside generator ownership.

## Non-goals

- Runtime or hot switching of providers.
- Editing provider choices in an already-created project.
- A general plugin SDK or third-party template loading.
- A second ORM, SQL builder, or generated repository abstraction.
- Automatic Redis caching of generated CRUD endpoints.
- Automatic NATS publication, subscriptions, JetStream streams, or outbox
  behavior.
- Local user registration, password hashing, login, refresh-token storage,
  OAuth/OIDC discovery, SSO, RBAC, or permission-schema changes.
- Additional external services such as object storage, search, email, SMS,
  payment, workflow, or feature flags.
- Identical query-planner or data-type behavior between SQLite and PostgreSQL.

## Command-line surface

`gobackend new` adds local flags to its existing factory-built Cobra command:

```text
gobackend new <dir> --module <module-path> \
  --http echo|fiber \
  --database sqlite|postgres \
  --cache none|redis \
  --messaging none|nats \
  --logging slog|zap|zerolog \
  --auth none|jwt
```

Defaults are:

```text
http=echo
database=sqlite
cache=none
messaging=none
logging=slog
auth=none
```

The defaults deliberately produce a small application that has no required
external service. PostgreSQL, Redis, NATS, alternate logging backends, and JWT
are opt-in.

Flags are local to `new`; `add`, `generate`, and `check` read the selection
recorded in project metadata. Cobra enum validation returns an error through
`RunE`. It must name the invalid value and the allowed values. No package-level
flag variables or global command instances are introduced. The CLI passes one
typed `ProjectOptions` value to `Generator.New`; the generator package remains
unaware of Cobra. Each enum flag supplies shell completion with
`cobra.ShellCompDirectiveNoFileComp`, and tests execute a fresh root command in
memory through `SetArgs`/`ExecuteContext`.

There is no `--orm` flag in this iteration. GORM is a declared fixed part of
the generated data path. Adding a flag that only accepts `gorm` would advertise
an abstraction that does not yet exist.

## Project metadata and compatibility

New projects contain a machine-owned `.gobackend-project.json`:

```json
{
  "generated_by": "gobackend",
  "schema_version": 1,
  "generator_version": "v0.2.0",
  "selection": {
    "http": "echo",
    "database": "sqlite",
    "cache": "none",
    "messaging": "none",
    "logging": "slog",
    "auth": "none"
  },
  "fingerprint": "<sha256-of-canonical-selection>"
}
```

The version shown above is illustrative of the next minor release; the actual
release version comes from build information. The fingerprint is a consistency
check, not a secret or security signature.

Rules:

- `new` writes metadata before the first resource generation.
- `add`, `generate`, and `check` strictly decode it with unknown fields
  rejected, validate every enum, and verify the fingerprint.
- The metadata path becomes an explicitly allowed generated-output path and is
  tracked by `.gobackend-generated.json`.
- An existing metadata target is considered owned only after strict parsing,
  enum validation, and fingerprint verification; a marker substring is not
  sufficient ownership evidence.
- Users do not edit this file to reconfigure a project. A modified or invalid
  selection fails with a message directing the user to create a new project.
- A project with neither metadata nor a metadata entry in its generated
  manifest is treated as a legacy project with the existing fixed selection:
  Echo, GORM, PostgreSQL, no Redis, no NATS, slog, and no authentication.
- A project with a generated manifest that claims metadata but whose metadata
  is missing or invalid fails closed.
- Legacy projects retain the current render path and are not silently converted
  to the new default selection.

The generated manifest remains the authority for file ownership. Provider
metadata expands its exact path allowlist but does not broaden recursive file
ownership or allow arbitrary scaffold files to be deleted.

## Version policy

"Latest" means the latest version tested and pinned by a particular
`go-backend-kit` release. `gobackend new` must not resolve the network's current
latest versions because doing so would make the same generator binary produce
different projects on different days.

The implementation begins from this dependency snapshot, verified on
2026-08-13, and refreshes it once before merging:

| Component | Initial pin |
| --- | --- |
| Go toolchain and generated `go` directive | 1.26.5 |
| Echo | `github.com/labstack/echo/v5 v5.3.1` |
| Fiber | `github.com/gofiber/fiber/v3 v3.4.0` |
| GORM | `gorm.io/gorm v1.31.2` |
| GORM CLI | `gorm.io/cli/gorm v0.2.4` |
| GORM PostgreSQL driver | `gorm.io/driver/postgres v1.6.2` |
| Pure-Go SQLite driver | `github.com/libtnb/sqlite v1.2.2` |
| Atlas Go engine | `ariga.io/atlas v1.3.0` |
| Atlas GORM provider | `ariga.io/atlas-provider-gorm v0.6.1` |
| Atlas Community image | `arigaio/atlas:1.3.0-community` |
| PostgreSQL image | `postgres:18.4-alpine3.24` |
| Redis client | `github.com/redis/go-redis/v9 v9.22.0` |
| Redis image | `redis:8.10.0-alpine3.23` |
| NATS client | `github.com/nats-io/nats.go v1.53.1` |
| NATS image | `nats:2.14.5-alpine3.22` |
| JWT implementation | `github.com/go-jose/go-jose/v4 v4.1.4` |
| zap | `go.uber.org/zap v1.28.0` |
| zap slog bridge | `github.com/samber/slog-zap/v2 v2.7.0` |
| zerolog | `github.com/rs/zerolog v1.35.1` |
| zerolog slog bridge | `github.com/samber/slog-zerolog/v2 v2.9.2` |

The implementation verifies these exact module versions and image tags again
before merging. Floating Docker tags such as `latest`, a bare major tag, or an
unreviewed host CLI are not emitted. Generated documentation calls out the
selected server image's license rather than implying all choices share the Go
client's license.

Provider-specific dependencies appear only when selected. `go mod tidy` must
leave no direct Echo dependency in a Fiber project, no PostgreSQL driver in a
SQLite-only project, and no Redis, NATS, zap, zerolog, or JWT dependency when
the corresponding provider is disabled.

## Generator design

### Typed options, not a plugin framework

The generator adds small named string types and one concrete options value:

```go
type HTTPChoice string
type DatabaseChoice string
type CacheChoice string
type MessagingChoice string
type LoggingChoice string
type AuthChoice string

type ProjectOptions struct {
    HTTP      HTTPChoice
    Database  DatabaseChoice
    Cache     CacheChoice
    Messaging MessagingChoice
    Logging   LoggingChoice
    Auth      AuthChoice
}
```

`ProjectOptions.Validate` is direct switch-based validation. It returns early
and wraps parse/read failures with context. It does not use reflection, struct
tag validation, a map of `any`, or a universal `Provider` interface.

Templates are first separated by lifecycle, then by what they generate.
One-time scaffold files rendered only by `new` live under:

```text
internal/generate/scaffold/common/
internal/generate/scaffold/http/echo/
internal/generate/scaffold/http/fiber/
internal/generate/scaffold/database/sqlite/
internal/generate/scaffold/database/postgres/
internal/generate/scaffold/cache/redis/
internal/generate/scaffold/messaging/nats/
internal/generate/scaffold/logging/slog/
internal/generate/scaffold/logging/zap/
internal/generate/scaffold/logging/zerolog/
internal/generate/scaffold/auth/jwt/
```

Files that must change on every `add`, `generate`, or `check` are not scaffold
assets. Resource HTTP handlers and contracts, the global registrar, models and
stores with dialect-sensitive content, GORM schema loader, OpenAPI, and project
metadata remain in the repeatable `renderDesired` path. Their logical groups
are:

```text
generated/common/
generated/http/echo/
generated/http/fiber/
generated/database/sqlite/
generated/database/postgres/
generated/auth/none/
generated/auth/jwt/
```

These logical groups may be stored as embedded template files or as small Go
source files containing template constants, but must not be routed through
`renderScaffold`. This distinction is covered by a regression that creates a
project, adds a resource, and proves the selected framework/database/auth
content appears in the regenerated files.

Each selection maps a known source to a known output path. A collision between
two selected templates is an error before any project file is written. Common
files are rendered once. Small provider switches are preferred to an interface
with one implementation or a registration API.

The current GORM CLI contract remains unchanged:

```text
go tool gorm gen -i <generated-model> -o <generated-output>
```

Official GORM CLI output remains the sole source of
`gormgen/query_gen.go`.

### Generated package shape

This feature preserves the existing `internal/platform` boundary instead of
combining provider work with an unrelated layout migration. Packages under it
remain named for one concrete purpose rather than `provider`, `adapter`,
`service`, `repository`, or `common` layers:

```text
cmd/api/main.go
internal/app/
internal/platform/auth/          # only for auth=jwt
internal/platform/cache/         # only for cache=redis
internal/platform/config/
internal/platform/database/
internal/platform/httpx/         # selected Echo or Fiber boundary helpers
internal/platform/logging/
internal/platform/messaging/     # only for messaging=nats
internal/platform/apperror/
internal/platform/optional/
internal/platform/validation/
internal/resources/<resource>/
openapi/
```

The existing resource-cohesive shape remains: model, DTO, concrete GORM store,
HTTP code, and contract tests live together for each resource. The generator
does not recreate repository/service/controller layers.

`cmd/api/main.go` constructs concrete dependencies explicitly and passes them
to the selected application template. It may have conditional arguments, but
it does not pass a `Container`, `ServiceProvider`, or `map[string]any`.

## HTTP choices

Echo and Fiber have separate templates for:

- application construction and middleware;
- resource registration and handler methods;
- JSON decoding and response serialization;
- centralized error handling;
- request ID and structured request logging;
- authentication middleware; and
- HTTP contract tests.

They share generated models, DTO validation, concrete GORM stores, public error
codes, and OpenAPI output. Store methods continue to accept
`context.Context`; no store imports Echo or Fiber.

The generated request path is deliberately concrete:

```text
selected framework handler -> DTO helper -> concrete GORM store -> database
```

There is no `WebContext`, `RequestContext`, or generic handler abstraction.
Fiber request objects are never retained beyond the handler. Any standard
context passed to database/auth/cache/message I/O must remain valid only for
that synchronous request path and preserve cancellation where Fiber supports
it.

Both server templates keep explicit header/read/write/idle timeouts where the
underlying server supports them and implement bounded graceful shutdown.

## Database choices

GORM is common; only the dialector and dialect-sensitive generated behavior
change.

### SQLite

- Use `github.com/libtnb/sqlite`; generated builds must pass with
  `CGO_ENABLED=0`.
- The default path is `data/app.db`, configurable with `DATABASE_PATH`.
- Create the parent directory explicitly and report a wrapped path-safe error.
- Enable foreign keys, a bounded busy timeout, and WAL using driver-supported
  configuration.
- Default to one open connection unless the implementation proves a safer
  tested pool configuration for the chosen driver.
- Generate SQLite-specific GORM tags and Atlas schema dialect.
- Production startup does not call `AutoMigrate`.
- Use Atlas Community with `gormschema.New("sqlite")`, a disposable
  `.gobackend` SQLite development database, and the mounted application file as
  the migration target. The temporary development database is removed on
  success and failure; the application database is never used as Atlas's dev
  database.

### PostgreSQL

- Require `DATABASE_URL` and keep bounded startup ping and explicit pool
  settings.
- Use the pinned GORM PostgreSQL driver and pinned PostgreSQL server image.
- Keep the current Atlas versioned-migration and schema-drift workflow, updated
  to the pinned Atlas Community release.
- Keep transaction/error behavior proven by the real PostgreSQL contract path.

### Dialect behavior

The generator makes these functions database-aware rather than hiding the
difference behind an ORM interface:

- GORM field tags for UUID, JSON, Decimal, and defaults;
- Atlas `gormschema` dialect and development database;
- database open/configuration code;
- transaction options unsupported by SQLite;
- migration and CI templates; and
- generated contract setup.

SQLite and PostgreSQL must preserve the public CRUD status codes, JSON shapes,
validation, exact-filter behavior, unique conflicts, PATCH semantics, and UTC
normalization. They are not promised to have identical type affinity, decimal
sort precision, isolation, locking, or query plans. The SQLite README explicitly
directs workloads requiring PostgreSQL concurrency or exact database-native
numeric semantics to choose PostgreSQL.

Changing database choice after migrations exist is unsupported. PostgreSQL and
SQLite migration directories are not interchangeable.

## Redis choice

`cache=none` emits no Redis dependency, configuration, process, health probe,
or package.

`cache=redis` emits:

- `internal/platform/cache` with a concrete constructor returning the selected
  `*redis.Client`;
- `REDIS_URL` and a bounded connect/ping timeout;
- startup `PING`, readiness `PING`, and explicit `Close` handling;
- a pinned Redis Compose service and integration test; and
- documentation showing how a consuming domain can define the small interface
  it needs.

Generated CRUD handlers are not automatically cached. There is no broad
`Cache` interface in the scaffold because no generated domain consumes it yet.
If handwritten code needs caching, that consuming package defines the minimal
`Get`/`Set`/`Delete`-style interface it actually uses, enabling a simple fake in
tests.

## NATS choice

`messaging=none` emits no NATS dependency, configuration, process, health
probe, or package.

`messaging=nats` emits:

- `internal/platform/messaging` with a concrete constructor returning
  `*nats.Conn`;
- `NATS_URL` and bounded connect/flush/drain timeouts;
- startup and readiness checks using a context-bounded flush;
- an explicit NATS drain timeout, shutdown through `Drain`, and `Close` where
  required;
- a pinned NATS Compose service and integration test; and
- documentation showing consumer-defined publisher/subscriber interfaces.

The scaffold does not publish CRUD events or create JetStream resources. Core
NATS, JetStream, delivery guarantees, subjects, retry, and outbox semantics are
business decisions and must not be guessed by the generator.

## Logging choices

All generated application and GORM call sites depend on `*slog.Logger` and
structured `slog.Attr` values. The selection changes only logger construction:

- `slog`: standard JSON handler to stdout;
- `zap`: zap backend exposed through a pinned slog handler bridge; and
- `zerolog`: zerolog backend exposed through a pinned slog handler bridge.

Each template returns a concrete logger and, where required, an explicit flush
function. Flush failures are joined into shutdown status rather than silently
discarded. Request IDs and the existing field names remain consistent across
all logging choices. No provider logs secrets, bearer tokens, database URLs,
Redis URLs, NATS credentials, or JWT claims.

There is no application-specific `Logger` interface, no fluent logging facade,
and no logger stored in a global outside `main` initialization.

## JWT authentication

### Scope and route policy

`auth=none` preserves the current public CRUD behavior and emits no JWT
dependency or auth package.

`auth=jwt` protects the `/api/v1` group. These routes remain public:

- `/health/live`;
- `/health/ready`;
- `/openapi.json`; and
- `/docs` plus its assets.

JWT support only verifies bearer tokens. Token issuance, passwords, sessions,
refresh tokens, user persistence, and authorization policy are outside scope.

For `auth=jwt`, generated OpenAPI declares an HTTP bearer security scheme with
JWT format, applies it to `/api/v1` operations, and documents the 401 response.
For `auth=none`, the security scheme and requirements are absent. Health,
OpenAPI, and documentation routes never acquire an authentication requirement.

### Verification contract

The generated concrete `auth.Verifier` uses `go-jose/v4` and requires:

- `AUTH_JWT_SECRET_B64`: a base64-encoded key of at least 32 decoded bytes;
- `AUTH_JWT_ISSUER`;
- `AUTH_JWT_AUDIENCE`; and
- optional `AUTH_JWT_LEEWAY`, defaulting to 30 seconds and bounded to a
  documented maximum.

HS256 is fixed by generated code for this provider. The verifier rejects a
different or missing `alg`; it never selects an algorithm from untrusted token
input. `iss`, `sub`, `aud`, and `exp` are required and validated. `nbf`, when
present, is validated with the same bounded leeway.

The identity exposed to request-scoped handwritten code is deliberately small:

```go
type Identity struct {
    Subject string
}
```

The auth package provides typed context helpers to attach and read `Identity`.
It does not expose raw claims as `map[string]any`. A later authorization design
may extend the typed identity or define a consuming `Authorizer` interface when
actual permission rules exist.

Missing, malformed, expired, wrong-algorithm, wrong-issuer, wrong-audience, or
invalid-signature tokens return the stable public 401 error and a
`WWW-Authenticate: Bearer` header. Internal verification errors and token
contents are never returned or logged.

## Startup, readiness, and shutdown

Construction remains explicit and linear in `run()`:

1. Build the selected logger.
2. Load and validate typed configuration.
3. Open and ping the database with a bounded startup context.
4. If selected, open and ping Redis.
5. If selected, connect to and flush NATS.
6. If selected, construct the JWT verifier after validating its configuration.
7. Construct the selected HTTP application from the concrete values.
8. Start the server and wait for a server error or process signal.
9. Shut down the server with a fresh bounded context.
10. Drain/close NATS, close Redis, close the SQL pool, and flush logging in
    reverse construction order, joining errors into the exit status.

Every started goroutine has an owner and a stop path. The generated scaffold
does not create background worker pools for Redis or NATS beyond library-owned
connections.

Readiness checks the selected external dependencies with the request context
and an upper bound. With SQLite and no optional services it checks the SQL
handle only. Checks are sequential because the maximum selected set is small;
the design does not add goroutines or an orchestration framework to save a few
milliseconds on a diagnostic endpoint.

## Performance contract

Provider selection adds no runtime registry lookup and no per-request
reflection. The generated hot path contains the same direct framework handler,
concrete store, and selected clients that a hand-written project would use.

Interfaces are introduced only by consuming handwritten code when multiple
implementations or a fake are needed. Generics remain limited to existing
algorithmic/data-shape uses such as optional fields and response/page helpers;
they are not used for provider polymorphism or a generic repository.

Performance verification is evidence-based:

- generated dependency scans prove unselected providers are absent;
- compiler escape output or focused benchmarks investigate any new allocation
  observed in HTTP/auth middleware;
- Echo and Fiber benchmarks are reported for the same generated route but do
  not use a brittle absolute CI threshold; and
- real database/Redis/NATS integration timing is kept separate from interface
  or middleware microbenchmarks.

## Generated documentation and Compose

README, `.env.example`, Compose, CI, and migration instructions are selected at
project creation:

- SQLite-only projects do not require Docker to start.
- PostgreSQL projects emit only PostgreSQL by default.
- Redis and NATS services are added only when selected.
- Environment examples contain names and safe placeholders, never generated
  secrets.
- JWT documentation shows how to supply a key and create test tokens without
  committing the key.
- CI contains only jobs relevant to the selected providers, plus portable
  `CGO_ENABLED=0` verification for SQLite projects.
- Local Compose ports bind to `127.0.0.1` unless an explicit documented opt-in
  changes the binding. Redis is not emitted as an unauthenticated public
  listener.

Generated examples use explicit pinned image tags and health checks. No Compose
service is enabled merely because its client library exists in another
provider combination.

## Error handling

- Invalid CLI choices fail before a target directory is created.
- Template source collisions and invalid metadata fail before installation.
- Provider startup errors are wrapped with the provider operation but redact
  credentials and connection strings.
- Partial startup closes already-opened dependencies in reverse order.
- Shutdown attempts every selected close/flush operation and joins errors.
- Auth failures use stable public errors; unexpected details stay at the
  boundary and are logged without secrets.
- Generation never overwrites an unowned file or deletes a modified stale
  generated file.

## Test strategy

Implementation follows red-green-refactor and keeps tests in the standard
`testing` package with table-driven cases and small fakes at real I/O
boundaries.

### Generator tests

- every flag default and valid/invalid enum;
- metadata strict decoding, canonical fingerprinting, tamper detection, and
  legacy inference;
- deterministic generation and `check` for every supported selection;
- provider template collision detection before project mutation;
- exact selected dependency presence and unselected dependency absence;
- no runtime provider registry, reflection, generic repository, DI container,
  or generated service/repository/controller layers;
- official GORM CLI marker and byte-for-byte ownership behavior; and
- preservation of arbitrary handwritten files.

### Generated unit and contract tests

- CRUD contracts for Echo/SQLite, Echo/PostgreSQL, Fiber/SQLite, and
  Fiber/PostgreSQL;
- stable request/response/error/OpenAPI behavior across both HTTP choices;
- correct presence or absence of OpenAPI bearer security and 401 responses;
- SQLite foreign-key, busy-timeout, pool, migration, and `CGO_ENABLED=0`
  behavior;
- PostgreSQL migration, transaction, conflict, and schema-drift behavior;
- Redis open/ping/close with a real temporary Redis service;
- NATS connect/flush/drain with a real temporary NATS service;
- all three logger selections, structured request fields, and flush behavior;
- JWT valid token, missing token, malformed bearer header, wrong algorithm,
  invalid signature, missing required claim, expiry, not-before, issuer,
  audience, public-route bypass, and error redaction; and
- startup rollback when database, Redis, NATS, or auth construction fails.

### Pairwise scaffold matrix

The full Cartesian product is unnecessary. In addition to the four mandatory
HTTP/database contracts, generated-project tests use a pairwise matrix so each
optional choice appears with both HTTP frameworks and both databases across the
suite. At minimum:

```text
echo  + sqlite   + none  + none + slog    + none
echo  + postgres + redis + nats + zap     + jwt
fiber + sqlite   + redis + none + zerolog + jwt
fiber + postgres + none  + nats + slog    + none
```

### Final verification

```text
gofmt / generated formatting checks
go mod tidy -diff
go test ./...
go test -race ./...
go vet ./...
go tool govulncheck ./...
golangci-lint
Windows cross-compilation
CGO_ENABLED=0 generated SQLite build and tests
real PostgreSQL + Atlas migration/schema-drift workflow
real Redis integration workflow when selected
real NATS integration workflow when selected
```

Tool-version incompatibilities are reported separately from code failures; a
test is not weakened merely to satisfy an analyzer built with an older Go
toolchain.

## Expected implementation surfaces

Cursor should expect to change these existing areas rather than building a
parallel generator:

- `internal/cli/cli.go` and `internal/cli/cli_test.go`: local enum flags,
  completion, typed options, and in-memory command tests;
- `internal/generate/generator.go`: pass validated project options through
  `new`, `add`, `generate`, and `check`, and keep one-time scaffold rendering
  separate from repeatable desired-output rendering;
- new focused files under `internal/generate` for choices/project metadata and
  provider-specific render selection; do not enlarge `generator.go` or
  `render.go` into a provider switch monolith;
- `internal/generate/render.go` and its tests: split current Echo/PostgreSQL
  resource templates into common, HTTP-specific, database-specific, and
  auth-specific repeatable renderers;
- `internal/generate/openapi.go` and tests: optional bearer security scheme,
  protected operation requirements, and 401 responses;
- `internal/generate/manifest.go` and install tests: exact project-metadata
  ownership path and fail-closed validation;
- `internal/generate/scaffold/`: selected one-time runtime, configuration,
  logging, optional-client, auth, Compose, CI, Atlas, and README templates;
- `scripts/postgres-e2e.sh` plus new narrowly named SQLite/Redis/NATS scripts:
  real service gates and exact cleanup by created container ID; and
- root/generated English and Chinese README content and `.env.example`.

The root command registration remains in the existing `New` factory through
`newCommand`; no second command tree, `init` registration, Viper singleton, or
package-level flag state is added.

## Implementation sequence for Cursor

1. Add failing tests for typed choices, CLI flags, metadata, legacy inference,
   and selected dependency output.
2. Implement `ProjectOptions` and metadata without changing current rendering.
3. Split scaffold assets into common and selected groups; keep the legacy path
   passing throughout.
4. Add pure-Go SQLite runtime and SQLite Atlas/contract support while retaining
   the PostgreSQL E2E path.
5. Split Echo HTTP/resource/contract templates, then add Fiber equivalents.
6. Add optional Redis and NATS startup/readiness/shutdown plus Compose tests.
7. Make logger construction selectable while keeping `*slog.Logger` at all
   consuming call sites.
8. Add concrete JWT verification and framework-specific middleware.
9. Add the pairwise generated-project matrix and dependency-absence checks.
10. Update English/Chinese root and generated documentation, run the full
    verification set, and report any Docker-only gate that was not executed.

Each step should be independently testable. Cursor must preserve unrelated
working-tree changes, must not use `git add .`, and must not push or publish
without separate authorization.

## Acceptance criteria

- `gobackend new` supports exactly the documented choices and defaults.
- A default generated project builds and runs with no Docker service and with
  `CGO_ENABLED=0`.
- Echo and Fiber projects expose the same documented CRUD/OpenAPI/error
  contract while using framework-native generated handlers.
- SQLite and PostgreSQL are both real runtime and migration paths, not merely
  unit-test substitutes.
- Redis, NATS, alternate logging backends, and JWT appear only when selected.
- JWT protects `/api/v1` and leaves only the documented health/docs endpoints
  public; secrets and tokens never appear in errors or logs.
- Generated code contains no universal provider/ORM/web context, generic CRUD
  repository, runtime registry, reflection-based DI, service locator, or
  repository/service/controller layer stack.
- Existing projects without project metadata keep the legacy Echo/PostgreSQL
  generation path.
- The official GORM CLI remains the sole producer of GORM field helpers.
- Unselected modules and Compose services are absent.
- Startup, readiness, shutdown, migration, ownership, and generated-project
  verification pass for the documented matrix.
- No claim of a passing real service or platform gate is made unless that gate
  actually ran.
