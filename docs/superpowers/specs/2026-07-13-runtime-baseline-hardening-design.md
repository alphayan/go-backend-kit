# Runtime Baseline Hardening Design

## Context

`go-backend-kit` generates production-shaped CRUD services with Echo v5, GORM,
PostgreSQL, Atlas, and OpenAPI 3.1. The v0.1.0 stack is current and its local and
remote verification pass, but three baseline details need tightening:

1. Generated CI invokes Atlas migration linting while the pinned Atlas Community
   edition does not promise the full linting and governance feature set.
2. `go 1.26.5` conflates the minimum compatible Go language/toolchain version
   with the preferred patched toolchain.
3. Generated services do not configure, probe, or explicitly close their SQL
   connection pools.

## Goals

- Keep the existing Echo v5, GORM, PostgreSQL, Atlas Community, and OpenAPI stack.
- Make Atlas Community guarantees accurate in CI, scripts, Make targets, and
  documentation.
- Declare Go 1.26.4 as the compatibility floor and Go 1.26.5 as the preferred
  toolchain.
- Give generated services validated connection-pool defaults, a bounded startup
  database probe, and explicit pool shutdown.
- Preserve deterministic generation and cross-platform generated-project tests.

## Non-goals

- Do not change search behavior, offset pagination, API routes, response shapes,
  authentication, authorization, observability, or PostgreSQL major version.
- Do not adopt standard/proprietary Atlas, Atlas Cloud, goose, or golang-migrate.
- Do not split tool dependencies into a separate Go module in this change.
- Do not retrofit already-created downstream projects automatically; these
  scaffold changes apply when a project is created with `gobackend new`.

## Selected approach

Keep the pinned Atlas Community image and narrow the project's claim to the
features it uses reliably: migration generation, versioned migration application,
and a one-shot comparison between the applied schema and the generated desired
schema. Remove `migrate lint` invocations and the generated `migrate-lint` Make
target. Continue to require human review of generated SQL.

This is preferred over standard Atlas because the project remains self-contained
and open-source without login or license requirements. It is preferred over
replacing Atlas because the existing GORM-to-versioned-SQL path remains intact.

## Atlas and migration workflow

The generated workflow remains:

1. Resource YAML generates GORM models.
2. The generated GORM schema loader emits the desired PostgreSQL schema.
3. Atlas Community generates versioned SQL using a temporary PostgreSQL dev
   database.
4. A developer reviews and commits the SQL migration.
5. CI applies committed migrations to PostgreSQL.
6. CI performs a direct schema comparison and fails if the applied database does
   not equal the generated desired schema.
7. Generated CRUD contracts run against that PostgreSQL database.

The generated Makefile will expose `migration` and `migrate-apply`, but no
`migrate-lint` target. Repository and generated-project documentation will state
that Community edition does not provide advanced linting, rollback, migration
testing, approval policies, or advanced database-object governance.

## Go toolchain declaration

Both the root module and generated module template will declare:

```go
go 1.26.4

toolchain go1.26.5
```

Atlas v1.2.3 declares Go 1.26.4 as its minimum, so `go mod tidy` cannot retain a
lower module baseline. CI remains pinned to Go 1.26.5. Documentation will
describe Go 1.26.4 as the minimum and Go 1.26.5 or a newer supported patch as
the recommended runtime.

## Database configuration and lifecycle

Generated configuration will add these environment variables and defaults:

| Variable | Default | Validation |
| --- | ---: | --- |
| `DB_MAX_OPEN_CONNS` | `25` | greater than zero |
| `DB_MAX_IDLE_CONNS` | `25` | zero through max-open inclusive |
| `DB_CONN_MAX_LIFETIME` | `30m` | zero or a positive Go duration |
| `DB_CONN_MAX_IDLE_TIME` | `5m` | zero or a positive Go duration |
| `DB_CONNECT_TIMEOUT` | `5s` | positive Go duration |

`config.Load` will parse through a testable internal loader that accepts an
environment lookup function. Errors will name the invalid variable and explain
its constraint. A missing `DATABASE_URL` remains an error.

The database package will expose a small `PoolOptions` value and a `Connection`
that owns both the GORM handle and its underlying `*sql.DB`. Opening a connection
will:

1. create the GORM PostgreSQL handle;
2. obtain and configure the underlying SQL pool;
3. apply max-open, max-idle, lifetime, and idle-time settings;
4. call `PingContext` using the caller's bounded startup context;
5. close the pool before returning any post-open error.

The generated main package will create a context bounded by
`DB_CONNECT_TIMEOUT`, open and probe the database, pass the GORM handle to the
application, and defer explicit pool closure. A close error will be logged.

## Error handling

- Invalid pool configuration fails before any database connection attempt.
- A startup timeout or unreachable database fails startup with a wrapped,
  non-secret error; the DSN is never included in the message.
- Readiness behavior remains unchanged and continues to probe using the request
  context.
- Pool close failures are logged during process teardown without masking the
  server's existing exit status.

## Test strategy

Implementation follows red-green-refactor:

1. Extend generator tests to require `go 1.26.4`, `toolchain go1.26.5`, absence
   of `migrate lint`, presence of migration application/schema comparison, and
   the new scaffold test files.
2. Add generated config tests for defaults, overrides, invalid integers,
   invalid durations, max-idle greater than max-open, and missing
   `DATABASE_URL`.
3. Add generated database tests using the existing pure-Go SQLite test dialector
   to prove pool configuration, successful probing, canceled-context failure,
   and explicit close behavior without requiring PostgreSQL for unit tests.
4. Run the generated-project compilation and CRUD contract suite.
5. Run root race tests, vet, vulnerability scanning, formatting, and the real
   PostgreSQL plus Atlas end-to-end workflow.

## Acceptance criteria

- No repository or generated CI path invokes `atlas migrate lint`.
- Atlas still generates, applies, and compares a real PostgreSQL migration in
  end-to-end verification.
- Root and newly generated `go.mod` files separate minimum and preferred Go
  versions exactly as specified.
- Generated services reject invalid pool configuration, cannot report startup
  success before a database ping succeeds, and close the SQL pool on exit.
- Existing API behavior and generated CRUD contracts remain unchanged.
- The working tree contains no unrelated changes.
