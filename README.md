# go-backend-kit

`go-backend-kit` is a deterministic Go backend scaffold for database-backed CRUD APIs. Define a resource in strict YAML and generate an Echo v5 + GORM project with five endpoints, versioned PostgreSQL migrations, OpenAPI 3.1, embedded Swagger UI, and contract tests.

[中文文档](README.zh-CN.md)

## Requirements

- Go 1.26.4 or newer, as required by Atlas v1.2.3; Go 1.26.5 or a newer supported patch is recommended
- Docker for PostgreSQL development and Atlas migrations

## Install and create a project

```bash
go install github.com/alphayan/go-backend-kit/cmd/gobackend@v0.1.0
gobackend new product-api --module github.com/yourname/product-api
cd product-api
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
docker compose up -d postgres
make migration name=create_products
DATABASE_URL='postgres://app:app@localhost:5432/app?sslmode=disable' make migrate-apply
DATABASE_URL='postgres://app:app@localhost:5432/app?sslmode=disable' make run
```

Open `http://localhost:8080/docs`.

## CLI

```text
gobackend new <dir> --module <module-path>
go tool gobackend add <resource.yaml>
go tool gobackend generate
go tool gobackend check
go tool gobackend version
```

Generated projects pin `gobackend` and `gorm` as Go tool dependencies. `go tool gobackend generate` invokes the pinned official `go tool gorm gen` workflow; the official GORM CLI remains the sole producer of `gormgen/query_gen.go`, and gobackend does not rewrite its output. Atlas no longer maintains its current CLI as a Go-installable package, so the open-source Atlas CLI v1.2.3 is pinned through `arigaio/atlas:1.2.3-community` locally and in CI. The Atlas Go engine and GORM provider are also pinned in `go.mod`.

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
.gobackend-generated.json       generated-file ownership and SHA-256 digests
cmd/api/                       server entrypoint and graceful shutdown
internal/app/                  Echo setup and middleware
internal/platform/             config, database, errors, response, optional fields
internal/resources/<resource>/ model, DTO, concrete store, HTTP handlers and routes
internal/resources/<resource>/gormgen/ official GORM CLI field helpers
openapi/                       generated OpenAPI 3.1 and embedded spec
resources/                     strict YAML source of truth
tools/gormschema/              Atlas GORM provider program
migrations/                    reviewed PostgreSQL SQL migrations
```

`.gobackend-generated.json` records the exact generator-owned paths and their SHA-256 digests. Handwritten `.go` files and unrelated official GORM output are preserved. A stale manifest-owned file is removed only when its bytes still match the recorded digest; if it was modified, generation stops with an error instead of deleting it. Generation is staged, formatted, validated, and installed with per-file atomic replacement. Running generation twice produces no changes; `check` fails on missing, stale, or modified generated files.

## Runtime defaults

The generated server includes request IDs, JSON `slog`, panic recovery, a 1 MiB body limit, a 15-second request timeout, security headers, configurable CORS, standard `http.Server` timeouts, and a 10-second graceful shutdown. It exposes `/health/live`, `/health/ready`, `/openapi.json`, and `/docs`.

The PostgreSQL pool defaults to 25 open connections, 25 idle connections, a 30-minute connection lifetime, and a 5-minute idle lifetime. `DB_MAX_OPEN_CONNS`, `DB_MAX_IDLE_CONNS`, `DB_CONN_MAX_LIFETIME`, and `DB_CONN_MAX_IDLE_TIME` override these values. Startup performs a fail-closed database probe bounded by `DB_CONNECT_TIMEOUT`, which defaults to 5 seconds, and shutdown explicitly closes the pool.

Production startup never calls `AutoMigrate`. SQLite `AutoMigrate` is used only inside generated contract tests. CI applies reviewed Atlas migrations to PostgreSQL before rerunning the same contracts.

## Development

```bash
go test -race ./...
go vet ./...
go tool govulncheck ./...
```

The generator end-to-end test creates a project, adds multiple resources, regenerates, checks drift, compiles all supported types, and runs the generated API contracts.

## v0.1.0 boundaries

Authentication, JWT, RBAC, soft delete, relations, MySQL, Redis, queues, uploads, and an admin frontend are intentionally out of scope.

## License

MIT
