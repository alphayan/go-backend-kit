# Domain-Cohesive Generation Design

## Context

`go-backend-kit` currently generates each resource as a repository, service,
handler, and router stack. The generated types include `Repository` and
`Service` interfaces with one implementation each, a shared generic
`repository.Base[T]`, and a constructor chain that wires all three layers.

That structure is more abstract than the generated CRUD behavior requires.
Because the generator can emit concrete resource-specific code, runtime
abstractions do not need to carry the repetition. There are no existing
generated projects to preserve, so this change may replace the v0.1.0 internal
output layout without a compatibility layer.

## Goals

- Generate concrete, domain-cohesive Go code for each resource.
- Remove speculative interfaces, the generic base repository, and constructor
  chaining.
- Keep HTTP parsing, input validation, database access, and error translation
  easy to locate without creating framework-style layers.
- Preserve every public HTTP behavior and production runtime guarantee.
- Continue protecting all handwritten, non-generated files.

## Non-Goals

- Replacing Echo, GORM, Atlas, OpenAPI, PostgreSQL, or Cobra.
- Removing the top-level `internal` boundary from the generator or generated
  server.
- Adding domain hooks, dependency-injection frameworks, mock frameworks, or a
  second persistence implementation.
- Preserving generated internal type names or file paths from v0.1.0.
- Changing the YAML schema or adding relationships, authentication, soft
  deletion, or other new API features.

## Generated Resource Layout

Each resource remains one domain package under
`internal/resources/<resource>`:

```text
internal/resources/product/
├── model_gen.go
├── dto_gen.go
├── store_gen.go
├── http_gen.go
├── contract_gen_test.go
└── gormgen/query_gen.go
```

The generator will no longer create:

```text
repository_gen.go
service_gen.go
handler_gen.go
routes_gen.go
custom.go
```

Users add handwritten behavior in ordinary `.go` files in the resource package
or elsewhere in the generated project. The generator continues to manage only
files carrying a recognized generated marker, so handwritten files remain
untouched. An empty `custom.go` placeholder is unnecessary.

## Resource Components

### DTOs and validation

`dto_gen.go` continues to define create and update inputs with presence-aware
fields. It also owns unexported helpers that validate those inputs and convert
them into database values. This keeps schema-derived validation beside the
schema-derived DTO instead of placing it in an application-wide service type.

The helpers preserve all current behavior:

- required, nullable, enum, length, minimum, and maximum validation;
- create defaults and Go zero values;
- PATCH distinction between absent, explicit `null`, and a supplied value;
- UTC normalization for time values;
- rejection of an update containing no fields.

### Concrete store

`store_gen.go` defines an unexported concrete store:

```go
type store struct {
	db *gorm.DB
}
```

Its methods implement resource-specific list, get, create, update, and delete
operations. They accept `context.Context` and return models plus database-layer
errors. Filtering and sorting continue to use generated GORM field helpers.

The store contains the current PostgreSQL-safe create transaction and
`RETURNING id` behavior. Get, update, and delete are generated directly for the
resource; they do not delegate to a generic `Base[T]`. Update returns the
updated model after the write, preserving the current API response.

No repository interface or exported constructor is generated. Handwritten
files in the same package may reuse the concrete store when needed.

### Concrete HTTP handler and registration

`http_gen.go` defines an unexported handler and the package's exported
registration entry point:

```go
type handler struct {
	store store
}

func Register(group *echo.Group, db *gorm.DB) {
	h := handler{store: store{db: db}}
	resource := group.Group("/products")
	resource.GET("", h.list)
	resource.GET("/:id", h.get)
	resource.POST("", h.create)
	resource.PATCH("/:id", h.update)
	resource.DELETE("/:id", h.delete)
}
```

Handler methods own only the HTTP boundary and the short CRUD orchestration:

1. Parse path, query, or JSON input.
2. Ask the DTO helper to validate and build database values.
3. Call the concrete store with the request context.
4. Translate the result into the stable HTTP response.

The handler does not define business interfaces or accept a generic service.
The global generated registrar continues to call each resource package's
`Register` function.

## Request and Error Flow

The request path is:

```text
Echo route -> handler -> DTO validation/value helper -> store -> GORM
```

Error ownership remains explicit:

- malformed paths, query values, or JSON become stable bad-request errors at
  the handler boundary;
- DTO constraint failures become the existing validation response;
- the store returns GORM/database-layer errors without importing HTTP error
  types;
- the handler maps record-not-found, duplicate-key, and unexpected database
  errors through `apperror.FromDatabase`;
- `httpx.WriteError` remains the only response serializer for public errors.

This preserves the existing 400, 404, 409, 422, and 500 semantics without a
pass-through service layer.

## Shared Packages

The generated server keeps focused shared packages for configuration, database
lifecycle, HTTP response handling, optional PATCH fields, validation, and
public application errors:

```text
internal/platform/apperror
internal/platform/config
internal/platform/database
internal/platform/httpx
internal/platform/optional
internal/platform/validation
```

`internal/platform/repository` is removed. Resource-specific generated stores
replace its CRUD methods. Filter expressions use `clause.Eq` directly. Each
generated store includes the small dialect-error translation needed by its raw
create statement so driver errors still become GORM sentinels before reaching
the handler. No new generic repository or shared data-access package is
created.

The `internal` boundary itself remains. It protects implementation packages in
the generator and generated server and is independent of the layer removal.

## Compatibility

The HTTP contract is unchanged:

- the five CRUD routes and status codes;
- pagination, filtering, search, and sorting;
- success and error response envelopes;
- PATCH three-state behavior;
- unique-conflict and not-found handling;
- UTC timestamps and hard deletion;
- OpenAPI 3.1 output and embedded documentation;
- Atlas migrations and PostgreSQL schema comparison;
- database pool, startup probe, timeout, logging, and shutdown behavior.

The generated internal layout is intentionally incompatible with v0.1.0. No
migration shim, deprecated alias, or legacy file is retained because there are
no existing generated projects.

## Test Strategy

Implementation follows red-green-refactor cycles.

The root generator contract first changes to require:

- `store_gen.go` and `http_gen.go`;
- absence of repository, service, handler, routes, and empty custom files;
- absence of `Repository interface`, `Service interface`, `Base[T]`, and the
  nested constructor chain;
- removal of the shared repository scaffold template;
- preservation of an arbitrary handwritten `.go` file across regeneration;
- deterministic regeneration and drift detection for the new managed files.

The generated SQLite contract continues to exercise all CRUD behavior,
including filtering, sorting, paging, create defaults, three-state PATCH,
empty updates, validation, duplicate keys, not-found responses, and safe 500
responses.

Final verification runs:

```bash
go test -race ./...
go vet ./...
go tool govulncheck ./...
./scripts/postgres-e2e.sh
```

The PostgreSQL E2E must still generate and apply an Atlas migration, report no
schema difference, and pass the same generated CRUD contract against
PostgreSQL.

## Documentation

The root and generated README files will describe resources as domain packages
with concrete stores and HTTP handlers. They will no longer advertise a
service/repository architecture, generic CRUD repository, or generated
`custom.go`. Documentation will state that handwritten non-generated `.go`
files are preserved.
