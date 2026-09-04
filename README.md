# go-backend-kit

`go-backend-kit` is a deterministic Go backend scaffold for database-backed CRUD APIs. Define a resource in strict YAML and generate a project with five endpoints, versioned migrations, OpenAPI 3.1, embedded Swagger UI, and contract tests. Session projects also include a generated Vue admin console served by the Go binary.

Default `gobackend new` selection is Echo + SQLite + slog, with no Redis, NATS, or authentication. PostgreSQL, Fiber, Redis, NATS, zap/zerolog logging, JWT verification, and Echo database sessions are opt-in at project creation. Providers are compiled in statically; the generated binary has no plugin registry or DI container.

[中文文档](README.zh-CN.md)

The default profile is intentionally personal: Echo + SQLite + slog, with no Redis, NATS, authentication, or operations stack. PostgreSQL, Fiber, Redis, NATS, alternate logging, JWT verification, Echo database sessions, and the production profile are opt-in at project creation.

## Requirements

- Go 1.27.1 or newer; generated projects pin `go 1.27.1`
- Session frontend development requires Node.js 24.20.0 LTS and pnpm 11.25.0; both versions are pinned in generated projects. Neither is a production runtime dependency.
- Docker is not required to run a default SQLite project. Use Docker for PostgreSQL, Redis, NATS, and Atlas migrations when those are selected.
- The production profile also uses Docker for the local application, Prometheus, and Grafana stack.

## Install and create a project

This checkout targets the unreleased v0.2.0. The published v0.1.0 does not contain the Session/production/upgrade features below. Until the release gates pass and a tag is explicitly published, use a local source checkout:

```bash
git clone https://github.com/alphayan/go-backend-kit.git
cd go-backend-kit
kit_root="$(pwd)"
GOBACKEND_DEVELOPMENT_REPLACE="$kit_root" go run ./cmd/gobackend new ../product-api --module github.com/yourname/product-api
cd ../product-api
```

Keep the source checkout available: the generated development project explicitly replaces the generator module with that local path. A binary installed with `go install ...@<tag or pushed commit>` pins that module version instead. Any other build, including `go build` from a modified checkout (Go stamps it `+dirty`), is a source build: it requires the development replacement and fails early instead of resolving a nonexistent or unpublished revision. Source availability, a published tag, passing release CI and deployment are separate states; see the release checklist in CONTRIBUTING.md.

Optional flags on `new` (defaults shown):

```text
--http echo|fiber           (default echo)
--database sqlite|postgres  (personal: sqlite; production: postgres)
--cache none|redis          (default none)
--messaging none|nats       (default none)
--logging slog|zap|zerolog  (default slog)
--auth none|jwt|session     (default none; session requires Echo)
--profile personal|production (default personal)
```

Create `product.yaml`:

```yaml
schema_version: 1
name: Product
table: products
route: /products
fields:
  - name: name
    type: string
    required: true
    max_length: 120
    searchable: true
    unique: true
  - name: status
    type: string
    required: true
    enum: [enabled, disabled]
    filterable: true
  - name: owner_id
    type: int64
    filterable: true
    sortable: true
```

Then generate, create a migration, and run:

```bash
go tool gobackend add product.yaml
make migration name=create_products
make migrate-apply
make run
```

A default SQLite project stores data in `data/app.db` and does not need Compose. PostgreSQL projects still use `docker compose up -d postgres` and `DATABASE_URL`.

Open `http://localhost:8080/docs`.

## Optional production profile

Select this profile only when the project needs a more complete local operations loop:

    gobackend new product-api --module github.com/yourname/product-api --profile production
    cd product-api
    cp .env.example .env
    cp .env.postgres.example .env.postgres
    chmod 600 .env .env.postgres
    # Follow the generated docs/postgres-operations.md to configure, initialize and migrate
    make up

It adds a pinned application Dockerfile, one-command Compose startup, container health checks, configurable JSON log levels and service metadata, Prometheus metrics/alerts, and a basic Grafana dashboard. Use make logs, make down, and make compose-config for the local lifecycle. It deliberately does not generate Kubernetes manifests or CI image-publish/deployment jobs.

Production defaults to PostgreSQL unless a database is explicitly selected; intentionally single-process projects may still use `--database sqlite`. Production PostgreSQL separates bootstrap, migration and runtime roles, keeps database administration secrets outside the API environment, and includes native backup/new-database restore scripts. Existing volumes are not automatically upgraded. Offsite backup destinations, cutover and TLS deployment still require separate configuration and verification.

## CLI

```text
gobackend new <dir> --module <module-path> [--http echo|fiber] [--database sqlite|postgres] [--cache none|redis] [--messaging none|nats] [--logging slog|zap|zerolog] [--auth none|jwt|session] [--profile personal|production]
go tool gobackend add <resource.yaml>
go tool gobackend generate
go tool gobackend check
go tool gobackend version
```

Generated projects pin `gobackend` and `gorm` as Go tool dependencies. `go tool gobackend generate` invokes the pinned official `go tool gorm gen` workflow; the official GORM CLI remains the sole producer of `gormgen/query_gen.go`, and gobackend does not rewrite its output. Atlas no longer maintains its current CLI as a Go-installable package, so the open-source Atlas CLI is pinned through `arigaio/atlas:1.3.0-community` locally and in CI. The Atlas Go engine and GORM provider are also pinned in `go.mod`.

The Community profile is used to generate and apply versioned migrations and to compare the applied schema with the generated GORM schema. It does not provide this project's advanced migration linting, rollback, migration testing, approval policies, or governance for advanced database objects. Review every generated SQL migration before applying it.

## Generated API

Every resource receives:

```text
GET    /api/v1/products
GET    /api/v1/products/:id
POST   /api/v1/products
PATCH  /api/v1/products/:id
DELETE /api/v1/products/:id
```

The list endpoint supports `page`, `page_size`, `sort`, `q`, and declared exact filters. PATCH fields preserve three states: omitted, explicit `null`, and a supplied value, including `false`, `0`, and an empty string.

Successful responses use `{"data": ...}`. Errors use:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "request validation failed",
    "details": {"name": "is required"},
    "request_id": "..."
  }
}
```

Models always include `id int64`, `created_at`, and `updated_at`. Client DTOs cannot write these fields. Timestamps are generated in UTC and serialized as RFC3339. Deletes are hard deletes in v0.1.0.

## Resource schema

Supported scalar types are `string`, `text`, `bool`, `int32`, `int64`, `float64`, `decimal`, `time`, `uuid`, and `json`.

`decimal` values use JSON strings to preserve precision.

Supported field options are `required`, `nullable`, `default`, `unique`, `index`, `enum`, `min`, `max`, `max_length`, `searchable`, `filterable`, and `sortable`. Unknown keys, unsafe names or routes, duplicate base fields, type-invalid defaults, and contradictory constraints fail before any generated file is replaced.

Relations are intentionally not generated in v0.1.0. Use scalar fields such as `user_id` and add domain behavior in ordinary handwritten `.go` files, which the generator never overwrites.

## Generated project shape

```text
.gobackend-project.json         provider selection and canonical fingerprint
.gobackend-generated.json       generated-file ownership and SHA-256 digests
cmd/api/                       server entrypoint and graceful shutdown
internal/app/                  selected HTTP framework setup and middleware
internal/platform/             config, database, errors, response, optional fields
internal/resources/<resource>/ model, DTO, concrete store, HTTP handlers and routes
internal/resources/<resource>/gormgen/ official GORM CLI field helpers
openapi/                       generated OpenAPI 3.1 and embedded spec
resources/                     strict YAML source of truth
tools/gormschema/              Atlas GORM provider program
migrations/                    reviewed SQL migrations
web/                           generated Vue admin console for session projects
```

`.gobackend-generated.json` records the exact generator-owned paths and their SHA-256 digests. Handwritten `.go` files and unrelated official GORM output are preserved. A stale manifest-owned file is removed only when its bytes still match the recorded digest; if it was modified, generation stops with an error instead of deleting it. Generation is staged, formatted, validated, and installed with per-file atomic replacement. Running generation twice produces no changes; `check` fails on missing, stale, or modified generated files.

## Runtime defaults

The generated server includes request IDs, JSON `*slog.Logger` logging, panic recovery, a 1 MiB body limit, a 15-second request timeout, security headers, configurable CORS, and a 10-second graceful shutdown. It exposes `/health/live`, `/health/ready`, `/openapi.json`, and `/docs`. JWT, when selected, protects `/api/v1` only.

PostgreSQL projects default to 25 open connections, 25 idle connections, a 30-minute connection lifetime, and a 5-minute idle lifetime. `DB_MAX_OPEN_CONNS`, `DB_MAX_IDLE_CONNS`, `DB_CONN_MAX_LIFETIME`, and `DB_CONN_MAX_IDLE_TIME` override these values. Startup performs a fail-closed database probe bounded by `DB_CONNECT_TIMEOUT`, which defaults to 5 seconds, and shutdown explicitly closes selected clients in reverse construction order.

Production startup never calls `AutoMigrate`. SQLite `AutoMigrate` is used only inside generated contract tests. PostgreSQL CI applies reviewed Atlas migrations before rerunning the same contracts.

The personal profile keeps the generated runtime small. The production profile additionally exposes /metrics, records HTTP request rate/latency/in-flight and database-pool metrics, and provisions Prometheus and Grafana locally. The profile is immutable after project creation.

## Upgrading generated projects

Run the newer `gobackend upgrade` inside the project to preview; only `gobackend upgrade --apply` writes source files. New projects record upstream scaffold digests in `.gobackend-scaffold.json`; commit it. Ordinary `generate` never adopts scaffold edits. Module tidy after `add` only advances `go.mod`/`go.sum` digests that still matched their upstream baseline before tidy.

Older projects without that baseline require `--baseline /absolute/pristine-old-project`: a verified reconstruction using the original generator, matching module/providers/resource definitions, or a trustworthy original snapshot. Never use the modified project or the new candidate as the old baseline. An existing generated-file manifest is required; ownership is not guessed. Provider/profile changes are not supported in place.

Preview resolves Go dependencies and retains `candidate/` and `plan.json` under gitignored `.gobackend/upgrade-*`, without changing application sources. Apply first backs up every replaced/deleted original with its permissions into `before/` in that private directory. Concurrent upstream/user edits block the entire batch. After manually merging against the candidate, explicitly use `--keep internal/app/app.go` (repeat for each resolved scaffold path) to retain your merged file and advance its upstream baseline. Generated outputs, `.env`, migrations and resource definitions cannot be kept/overridden this way. Merge custom dependency changes in `go.mod`/`go.sum` too; keeping stale dependencies is not a completed upgrade. Scaffold files that a newer generator no longer ships (for example `.npmrc`) are reported as `remove` and backed up when they still match their upstream digest; a modified copy is a conflict that `--keep` retains as your own file.

Apply uses the project lock, change detection and per-file replacement, rolling back observed write failures without overwriting newer concurrent edits. It is not an atomic directory transaction: after power loss/forced exit, inspect `plan.json` and `before/` to recover or retry. New files have no original backup; compare against the candidate before removing them during manual rollback. Do not edit, run other generators or deploy the directory during apply. Retain recovery artifacts until validation, then remove only the exact upgrade directory. Upgrade never reads real `.env`, migrates databases, commits or deploys.

After upgrading, run `go mod tidy`, `go tool gobackend generate`, `go tool gobackend check`, `go test -race ./...`, `go vet ./...` and `go tool govulncheck ./...`. Session projects additionally require frozen pnpm installation, frontend build/browser tests. Review database migrations and verify recovery separately before production cutover.

## Session authentication option

`--auth session` is an Echo-only, database-backed login kit with revocable HttpOnly cookies, Argon2id passwords, fixed `admin`/`viewer` RBAC, global standard-library cross-origin protection, bounded login/KDF limits, and best-effort audit logs. It adds `POST /auth/login`, `POST /auth/logout`, `GET /auth/me`, and `POST /auth/password`, plus generated route-permission tables and an embedded Vue admin console for resources.

Session projects model `auth_users`, `auth_sessions`, and `auth_audit_logs` in the Atlas desired schema; PostgreSQL also includes the shared limiter's `auth_rate_limits`. They do not ship handwritten authentication-table SQL or call `AutoMigrate` at runtime: create, review, and apply an explicit migration before starting. Empty user tables require the paired bootstrap email/password environment variables. Production requires the `__Host-session` Secure cookie, TLS, and HSTS. PostgreSQL shares IP/email budgets across instances using short database transactions; SQLite limiting is single-process only. Configure `HTTP_TRUSTED_PROXY_CIDRS` for actual proxy hops; empty means direct-IP mode. Audit writes happen after business commit, so a crash in that narrow window can lose the audit event.

The console is available at `/admin/`. It generates typed Zod forms, searchable/filterable/sortable tables, pagination, admin-only create/update/delete controls, viewer read-only behavior, login/logout, password change, light/dark/system themes, responsive layouts, and accessible dialogs. Use `make frontend-install`, `make frontend-typecheck`, `make frontend-test`, and `make frontend-build`; `make frontend-dev` runs Vite with a same-origin development proxy. The production Docker build compiles the frontend before the Go binary embeds `web/dist`, so Node.js and pnpm are not present in the runtime image.

## Development

Session consoles also include admin-only Users and Audit logs pages: account creation, role assignment, disable/re-enable, session listing/revocation, and paginated audit filters. Administrators cannot change their own role/status. Access changes revoke the target user's sessions. Audit retention is opt-in through `AUTH_AUDIT_RETENTION_DAYS` (default `0`, disabled).

```bash
go test -race -timeout 30m ./...
go vet ./...
go tool govulncheck ./...
./scripts/frontend-e2e.sh
```

The generator end-to-end test creates a project, adds multiple resources, regenerates, checks drift, compiles all supported types, and runs the generated API contracts.

Session projects include serial/parallel `BenchmarkPassword` workloads. See the [password benchmark record](docs/password-benchmark-2026-09-04.md) for reproducible commands, ten repeated samples, allocations, and local RSS limits. Microbenchmarks do not establish production login latency or algorithm strength.

`scripts/frontend-e2e.sh` requires Docker for explicit SQLite migrations and installs the pinned Chromium test browser. It checks the frozen pnpm lockfile, types, helper tests, build/embed, then browser login, user/session administration, audit filters, password rotation, resource CRUD/search/filter/pagination, and viewer restrictions. Test data is isolated in a temporary generated project; failures retain that directory for inspection.

`scripts/frontend-e2e.sh production` generates the production profile with an explicit disposable SQLite fixture and a test-only Go standard-library TLS proxy. Browser-local `.test` hostname mappings verify HTTP rejection of Secure cookies, HTTPS `__Host-session`/HttpOnly/Path/SameSite behavior, same-origin resource writes, cross-site form POST rejection, Lax top-level GET navigation, and rejection of replayed cookies after rotation/logout. Proxy unit tests check forwarding-header sanitization while preserving Host/Origin/Cookie. The harness owns and closes ports 4187–4189 (`E2E_PORT` selects three consecutive ports) and never reuses existing servers.

The proxy is copied only into the temporary test project, not shipped as a deployment. Test browser contexts ignore httptest's self-signed certificate; the system trust store is unchanged. The HSTS assertion proves the proxy header only, not real certificate issuance/renewal or persistent browser upgrades. Real domains, proxy IP trust configuration, production capacity and offsite backups still need separate acceptance. PostgreSQL migrations and shared limits have independent integration gates; this SQLite TLS fixture does not replace them.

## Current boundaries

Provider changes in an already-created project, a second ORM, refresh tokens, runtime-editable role definitions, password reset/MFA/SSO, soft delete, relations, MySQL, automatic CRUD caching, NATS/JetStream topologies, and uploads are intentionally out of scope.

Kubernetes manifests and CI image publishing/deployment are also intentionally out of scope for this release.

## License

MIT
