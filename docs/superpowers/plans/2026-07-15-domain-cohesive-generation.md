# Domain-Cohesive Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the generated repository/service/handler stack with a concrete domain-local store and HTTP handler while preserving the complete HTTP and database contract.

**Architecture:** Each resource remains one package. `dto_gen.go` validates and builds database values, `store_gen.go` contains concrete GORM operations, and `http_gen.go` parses HTTP input, calls the store, maps errors, and registers routes. The generated server keeps its focused shared runtime packages but removes the generic repository package.

**Tech Stack:** Go 1.26.4 with toolchain Go 1.26.5, Echo v5, GORM, SQLite contract tests, PostgreSQL 17, Atlas v1.2.3 Community, OpenAPI 3.1.

## Global Constraints

- Preserve the five CRUD routes, response envelopes, status codes, pagination, filtering, sorting, search, PATCH three-state semantics, validation, unique-conflict handling, UTC normalization, and safe internal errors.
- Preserve deterministic generation, drift detection, generated GORM field helpers, Atlas migrations, PostgreSQL schema comparison, pool configuration, startup probing, and graceful shutdown.
- Do not add interfaces, dependency-injection machinery, generic repositories, compatibility aliases, hooks, or new API features.
- Keep the generator and generated server under `internal`; remove only the generated shared repository package.
- Generated stores return database-layer errors and do not import HTTP response packages.
- Handwritten non-generated files must survive regeneration unchanged.

---

### Task 1: Generate concrete domain resources

**Files:**
- Modify: `internal/generate/project_test.go:59-172`
- Modify: `internal/generate/render.go:26-52`
- Modify: `internal/generate/render.go:560-844`
- Modify: `internal/generate/generator.go:132-153`
- Modify: `internal/generate/generator.go:484-501`
- Delete: `internal/generate/scaffold/internal/platform/repository/repository.go.tmpl`
- Test: `internal/generate/project_test.go`

**Interfaces:**
- Consumes: parsed `spec.Resource`, existing template helpers, generated `gormgen` columns, `apperror`, `httpx`, Echo v5, and GORM.
- Produces: unexported `store`, unexported `handler`, exported `Register(*echo.Group, *gorm.DB)`, `createValues(CreateXInput)`, and `updateValues(UpdateXInput)` inside each resource package.

- [ ] **Step 1: Change the generated-layout contract first**

In `TestNewAddGenerateAndCheck`, replace the resource entries in `wantFiles` with:

```go
"internal/resources/task/model_gen.go",
"internal/resources/task/dto_gen.go",
"internal/resources/task/gormgen/query_gen.go",
"internal/resources/task/store_gen.go",
"internal/resources/task/http_gen.go",
"internal/resources/task/contract_gen_test.go",
```

After the `wantFiles` loop, require all removed outputs to be absent:

```go
obsoleteFiles := []string{
	"internal/resources/task/repository_gen.go",
	"internal/resources/task/service_gen.go",
	"internal/resources/task/handler_gen.go",
	"internal/resources/task/routes_gen.go",
	"internal/resources/task/custom.go",
	"internal/platform/repository/repository.go",
}
for _, name := range obsoleteFiles {
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("obsolete generated file %s still exists: %v", name, err)
	}
}
```

Add `errors` to the test imports. Replace the repository-content assertion with concrete store and HTTP assertions:

```go
storeData, err := os.ReadFile(filepath.Join(root, "internal/resources/task/store_gen.go"))
if err != nil {
	t.Fatal(err)
}
storeSource := string(storeData)
for _, required := range []string{
	"type store struct",
	"clause.Eq{",
	"Column: gormgen.Task.Done.Column()",
	"Value:",
	"gormgen.Task.Done.Asc()",
	"func (s store) create(",
	"func (s store) update(",
	"func translateDatabaseError(",
} {
	if !strings.Contains(storeSource, required) {
		t.Errorf("generated store missing %q", required)
	}
}

dtoData, err := os.ReadFile(filepath.Join(root, "internal/resources/task/dto_gen.go"))
if err != nil {
	t.Fatal(err)
}
for _, required := range []string{
	"func createValues(input CreateTaskInput)",
	"func updateValues(input UpdateTaskInput)",
} {
	if !strings.Contains(string(dtoData), required) {
		t.Errorf("generated DTO helpers missing %q", required)
	}
}

httpData, err := os.ReadFile(filepath.Join(root, "internal/resources/task/http_gen.go"))
if err != nil {
	t.Fatal(err)
}
httpSource := string(httpData)
for _, required := range []string{
	"type handler struct",
	"h := handler{store: store{db: db}}",
	"func Register(group *echo.Group, db *gorm.DB)",
} {
	if !strings.Contains(httpSource, required) {
		t.Errorf("generated HTTP resource missing %q", required)
	}
}

combined := storeSource + "\n" + httpSource
for _, forbidden := range []string{
	"type Repository interface",
	"type Service interface",
	"Base[",
	"NewHandler(",
	"NewService(",
	"NewRepository(",
} {
	if strings.Contains(combined, forbidden) {
		t.Errorf("generated resource retains speculative abstraction %q", forbidden)
	}
}
```

Replace the `custom.go` preservation block with an arbitrary handwritten file:

```go
handwritten := filepath.Join(root, "internal/resources/task/rules.go")
const handwrittenSource = "package task\n\nconst HandwrittenRule = true\n"
if err := os.WriteFile(handwritten, []byte(handwrittenSource), 0o644); err != nil {
	t.Fatal(err)
}
before := snapshot(t, root)
if err := g.Generate(ctx, root); err != nil {
	t.Fatalf("second Generate() error = %v", err)
}
after := snapshot(t, root)
if before != after {
	t.Fatal("second generation changed the project")
}
data, err := os.ReadFile(handwritten)
if err != nil {
	t.Fatal(err)
}
if string(data) != handwrittenSource {
	t.Fatalf("generation changed handwritten source: %q", data)
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
go test ./internal/generate -run TestNewAddGenerateAndCheck -count=1
```

Expected: FAIL because `store_gen.go` and `http_gen.go` do not exist and the old layered files still do.

- [ ] **Step 3: Change the generated file set**

In `renderGenerated`, use exactly these resource templates:

```go
resourceTemplates := []struct {
	name string
	body string
}{
	{"model_gen.go", modelTemplate},
	{"dto_gen.go", dtoTemplate},
	{"store_gen.go", storeTemplate},
	{"http_gen.go", httpTemplate},
	{"contract_gen_test.go", contractTemplate},
}
```

Delete `repositoryTemplate`, `serviceTemplate`, `handlerTemplate`, and `routesTemplate`. Replace them with `storeTemplate` and `httpTemplate` as described below.

- [ ] **Step 4: Move value construction beside the DTOs**

Keep `validateCreate` and `validateUpdate`. Add these unexported helpers to `dtoTemplate` immediately after them:

```go
func createValues(input Create{{.Resource.Name}}Input) (map[string]any, validation.Details) {
	details := validateCreate(input)
	if len(details) > 0 {
		return nil, details
	}
	zeroModel := {{.Resource.Name}}{}
	_ = zeroModel
	values := map[string]any{}
{{range .Resource.Fields}}{{if not .HasDefault}}{{if and (eq .Type "json") (not .Nullable)}}	values[{{quote .Column}}] = []byte("{}")
{{else}}	values[{{quote .Column}}] = zeroModel.{{.GoName}}
{{end}}{{end}}{{end}}{{range .Resource.Fields}}	if input.{{.GoName}}.IsNull() {
		values[{{quote .Column}}] = nil
	} else if value, ok := input.{{.GoName}}.Value(); ok {
{{if eq .Type "time"}}		value = value.UTC()
{{end}}		values[{{quote .Column}}] = value
	}
{{end}}	return values, nil
}

func updateValues(input Update{{.Resource.Name}}Input) (map[string]any, validation.Details) {
	details := validateUpdate(input)
	if len(details) > 0 {
		return nil, details
	}
	values := map[string]any{}
{{range .Resource.Fields}}	if input.{{.GoName}}.IsSet() {
		if input.{{.GoName}}.IsNull() {
			values[{{quote .Column}}] = nil
		} else if value, ok := input.{{.GoName}}.Value(); ok {
{{if eq .Type "time"}}			value = value.UTC()
{{end}}			values[{{quote .Column}}] = value
		}
	}
{{end}}	return values, nil
}
```

Do not import `apperror` into `dto_gen.go`. Validation details remain transport-neutral; the HTTP handler decides whether they become a 422 validation error or the existing 400 empty-update error.

- [ ] **Step 5: Generate a concrete store**

Define `storeTemplate` with these imports and types:

```go
import (
	"context"
	"fmt"
	"strings"
	"time"

	"{{.Module}}/internal/resources/{{.Resource.Package}}/gormgen"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type filters struct {
	page     int
	pageSize int
	sort     string
	query    string
	exact    map[string]any
}

type store struct {
	db *gorm.DB
}
```

Move the existing list query into:

```go
func (s store) list(ctx context.Context, filters filters) ([]{{.Resource.Name}}, int64, error)
```

Use the lower-case filter fields and replace shared `repository.Equal` calls with direct GORM clauses:

```go
db = db.Where(clause.Eq{
	Column: gormgen.{{$.Resource.Name}}.{{.GoName}}.Column(),
	Value:  value,
})
```

Keep the generated GORM sort columns and pagination behavior unchanged. Generate these concrete CRUD methods:

```go
func (s store) get(ctx context.Context, id int64) ({{.Resource.Name}}, error) {
	return gorm.G[{{.Resource.Name}}](s.db).Where("id = ?", id).First(ctx)
}

func (s store) create(ctx context.Context, values map[string]any) ({{.Resource.Name}}, error) {
	now := time.Now().UTC()
	columns := []string{"\"created_at\"", "\"updated_at\""}
	arguments := []any{now, now}
{{range .Resource.Fields}}	if value, present := values[{{quote .Column}}]; present {
		columns = append(columns, {{quote (sqlColumn .Column)}})
		arguments = append(arguments, value)
	}
{{end}}	placeholders := make([]string, len(columns))
	for index := range placeholders {
		placeholders[index] = "?"
	}
	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) RETURNING \"id\"",
		{{quote (sqlColumn .Resource.Table)}},
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)
	var created {{.Resource.Name}}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var id int64
		if err := tx.Raw(query, arguments...).Scan(&id).Error; err != nil {
			return translateDatabaseError(tx, err)
		}
		item, err := (store{db: tx}).get(ctx, id)
		if err != nil {
			return err
		}
		created = item
		return nil
	})
	return created, err
}

func (s store) update(ctx context.Context, id int64, values map[string]any) ({{.Resource.Name}}, error) {
	result := s.db.WithContext(ctx).Model(new({{.Resource.Name}})).Where("id = ?", id).Updates(values)
	if result.Error != nil {
		return {{.Resource.Name}}{}, translateDatabaseError(s.db, result.Error)
	}
	if result.RowsAffected == 0 {
		return {{.Resource.Name}}{}, gorm.ErrRecordNotFound
	}
	return s.get(ctx, id)
}

func (s store) delete(ctx context.Context, id int64) error {
	rows, err := gorm.G[{{.Resource.Name}}](s.db).Where("id = ?", id).Delete(ctx)
	if err != nil {
		return translateDatabaseError(s.db, err)
	}
	if rows == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func translateDatabaseError(db *gorm.DB, err error) error {
	if translator, ok := db.Dialector.(gorm.ErrorTranslator); ok {
		return translator.Translate(err)
	}
	return err
}
```

- [ ] **Step 6: Generate one concrete HTTP boundary**

Define `httpTemplate` by combining the current handler and route templates. Use:

```go
type handler struct {
	store store
}

func Register(group *echo.Group, db *gorm.DB) {
	h := handler{store: store{db: db}}
	resource := group.Group({{quote .Resource.Route}})
	resource.GET("", h.list)
	resource.GET("/:id", h.get)
	resource.POST("", h.create)
	resource.PATCH("/:id", h.update)
	resource.DELETE("/:id", h.delete)
}
```

Rename handler methods to lower-case `list`, `get`, `create`, `update`, and `delete` with value receiver `(h handler)`. Keep current path/query/JSON parsing, but construct lower-case `filters` fields.

For every store call, translate database errors at the HTTP boundary:

```go
items, total, err := h.store.list(c.Request().Context(), filters{
	page: page, pageSize: pageSize, sort: sortValue,
	query: c.QueryParam("q"), exact: exact,
})
if err != nil {
	return httpx.WriteError(c, apperror.FromDatabase(err))
}
```

Create must use the DTO helper before calling the store:

```go
values, details := createValues(input)
if len(details) > 0 {
	return httpx.WriteError(c, apperror.Validation(details))
}
item, err := h.store.create(c.Request().Context(), values)
if err != nil {
	return httpx.WriteError(c, apperror.FromDatabase(err))
}
```

Update must preserve the distinct empty-update error:

```go
values, details := updateValues(input)
if len(details) > 0 {
	return httpx.WriteError(c, apperror.Validation(details))
}
if len(values) == 0 {
	return httpx.WriteError(c, apperror.BadRequest("empty_update", "at least one field must be provided"))
}
item, err := h.store.update(c.Request().Context(), id, values)
if err != nil {
	return httpx.WriteError(c, apperror.FromDatabase(err))
}
```

Get and delete map their store errors with `apperror.FromDatabase`. Keep `parseID` in `http_gen.go` unchanged except for formatting.

- [ ] **Step 7: Stop creating extension placeholders and remove the generic repository scaffold**

In `Generator.Generate`, return the generated-file installation result directly after the context check:

```go
if err := ctx.Err(); err != nil {
	return err
}
return installGenerated(root, desired)
```

Delete `ensureExtensions` entirely. Delete
`internal/generate/scaffold/internal/platform/repository/repository.go.tmpl`.
Do not add a replacement shared data-access package.

- [ ] **Step 8: Format and run the focused contract to verify GREEN**

Run:

```bash
gofmt -w internal/generate/project_test.go internal/generate/generator.go internal/generate/render.go
go test ./internal/generate -run TestNewAddGenerateAndCheck -count=1
```

Expected: PASS.

- [ ] **Step 9: Run all generator tests**

Run:

```bash
go test ./internal/generate -count=1
```

Expected: PASS, including compilation and execution of freshly generated projects.

- [ ] **Step 10: Review the generated source for the agreed boundaries**

Generate a disposable resource project and inspect it:

```bash
tmp=$(mktemp -d)
kit_root=$PWD
GOBACKEND_DEVELOPMENT_REPLACE="$kit_root" go run ./cmd/gobackend new "$tmp/api" --module example.com/api
(cd "$tmp/api" && go tool gobackend add "$kit_root/examples/product.yaml")
find "$tmp/api/internal/resources/product" -type f | sort
rg -n 'type (Repository|Service) interface|Base\[|NewHandler|NewService|NewRepository' "$tmp/api/internal" || true
rm -rf "$tmp"
```

Expected: the resource contains `model_gen.go`, `dto_gen.go`, `store_gen.go`, `http_gen.go`, `contract_gen_test.go`, and `gormgen/query_gen.go`; the final `rg` prints nothing.

- [ ] **Step 11: Commit the concrete generator**

```bash
git add internal/generate/project_test.go internal/generate/render.go internal/generate/generator.go internal/generate/scaffold/internal/platform/repository/repository.go.tmpl
git commit -m "refactor: generate domain-cohesive CRUD resources"
```

---

### Task 2: Document the domain-cohesive output

**Files:**
- Modify: `README.md:100-124`
- Modify: `README.zh-CN.md:88-99`
- Modify: `internal/generate/scaffold/README.md.tmpl:13-22`
- Modify: `internal/generate/scaffold/README.zh-CN.md.tmpl:13-22`
- Test: generated README inside a disposable project

**Interfaces:**
- Consumes: the generated layout from Task 1.
- Produces: root and generated documentation that describe concrete domain packages and generic handwritten `.go` extensions.

- [ ] **Step 1: Capture the stale documentation as the RED condition**

Run:

```bash
rg -n 'custom\.go|repository, service, handler|service 和 repository|generic CRUD|泛型 CRUD|repository filters|仓储筛选' README.md README.zh-CN.md internal/generate/scaffold/README.md.tmpl internal/generate/scaffold/README.zh-CN.md.tmpl
```

Expected: matches in all four files.

- [ ] **Step 2: Update the root English README**

Replace the custom extension sentence with:

```text
Relations are intentionally not generated in v0.1.0. Use scalar fields such as `user_id` and add domain behavior in ordinary handwritten `.go` files, which the generator never overwrites.
```

Change the generated shape entries to:

```text
internal/platform/             config, database, errors, response, optional fields
internal/resources/<resource>/ model, DTO, concrete store, HTTP handlers and routes
internal/resources/<resource>/gormgen/ official GORM CLI field helpers
```

Replace the ownership sentence with:

```text
Only files with a recognized generated marker are managed. Handwritten `.go` files are preserved. Generation is staged, formatted, validated, and installed with per-file atomic replacement.
```

- [ ] **Step 3: Update the root Chinese README**

Use these statements:

```text
首版不生成关联。`user_id` 等业务 ID 作为普通标量字段声明；领域扩展直接写在普通手写 `.go` 文件中，生成器永不覆盖。

每个资源是一个领域内聚包：DTO 负责输入验证和值转换，具体 store 负责 GORM 数据访问，HTTP handler 负责协议边界和短链路编排。代码不生成 repository/service 接口、通用泛型仓储或依赖注入链。固定版本的官方 GORM CLI 继续生成筛选与排序使用的字段辅助。

只有带受认可生成标记的文件会被管理；普通手写 `.go` 文件会被保留。
```

Keep the existing atomic replacement, idempotency, drift, runtime, Atlas, and test claims.

- [ ] **Step 4: Update both generated README templates**

Replace references to `custom.go` with generic handwritten `.go` files. Change “repository filters and sorting” to “store filters and sorting” in English and “store 筛选和排序” in Chinese. Do not add a generated directory tree that the short project README does not currently contain.

- [ ] **Step 5: Verify documentation and generated README output**

Run:

```bash
if rg -n 'custom\.go|repository, service, handler|service 和 repository|generic CRUD|泛型 CRUD|repository filters|仓储筛选' README.md README.zh-CN.md internal/generate/scaffold/README.md.tmpl internal/generate/scaffold/README.zh-CN.md.tmpl; then
	exit 1
fi

tmp=$(mktemp -d)
GOBACKEND_DEVELOPMENT_REPLACE="$PWD" go run ./cmd/gobackend new "$tmp/api" --module example.com/api
rg -n 'handwritten.*\.go|手写.*\.go|store filters|store 筛选' "$tmp/api/README.md" "$tmp/api/README.zh-CN.md"
rm -rf "$tmp"
```

Expected: the stale-term scan returns no matches; the generated README scan finds the new extension and store wording.

- [ ] **Step 6: Commit the documentation**

```bash
git add README.md README.zh-CN.md internal/generate/scaffold/README.md.tmpl internal/generate/scaffold/README.zh-CN.md.tmpl
git commit -m "docs: describe domain-cohesive generated resources"
```

---

### Task 3: Verify the full generator and production path

**Files:**
- Verify: all files changed in Tasks 1 and 2
- Verify: freshly generated project and PostgreSQL E2E artifacts

**Interfaces:**
- Consumes: the completed concrete resource generator and updated documentation.
- Produces: fresh evidence that the refactor preserves code quality, behavior, security, migrations, and generated output.

- [ ] **Step 1: Check formatting and whitespace**

Run:

```bash
gofmt -w internal/generate/project_test.go internal/generate/generator.go internal/generate/render.go
git diff --check
```

Expected: no output from `git diff --check`.

- [ ] **Step 2: Run the full race-enabled test suite**

Run:

```bash
go test -race ./...
```

Expected: PASS with zero race reports.

- [ ] **Step 3: Run static analysis**

Run:

```bash
go vet ./...
```

Expected: exit 0 with no findings.

- [ ] **Step 4: Run the vulnerability scan**

Run:

```bash
go tool govulncheck ./...
```

Expected: exit 0 and no vulnerabilities in called code.

- [ ] **Step 5: Run the real PostgreSQL and Atlas path**

Run:

```bash
./scripts/postgres-e2e.sh
```

Expected: Atlas generates and applies the migration, schema comparison is empty, generated contracts pass against PostgreSQL, and the script exits 0.

- [ ] **Step 6: Audit the final diff against the design**

Run:

```bash
git status --short
git diff HEAD~2 --stat
git diff HEAD~2 -- internal/generate/project_test.go internal/generate/generator.go internal/generate/render.go README.md README.zh-CN.md internal/generate/scaffold/README.md.tmpl internal/generate/scaffold/README.zh-CN.md.tmpl
rg -n 'type (Repository|Service) interface|Base\[|NewHandler\(NewService|repository_gen|service_gen|handler_gen|routes_gen|custom\.go' internal/generate/render.go internal/generate/generator.go internal/generate/scaffold README.md README.zh-CN.md
```

Expected: the diff contains only the approved generator, test, scaffold deletion, and documentation changes. The final `rg` prints nothing.

- [ ] **Step 7: Record verification evidence without creating an empty commit**

If verification changed files through formatting, commit only those real changes:

```bash
git add internal/generate/project_test.go internal/generate/generator.go internal/generate/render.go
git diff --cached --quiet || git commit -m "chore: format domain-cohesive generator"
```

Otherwise leave the two implementation commits from Tasks 1 and 2 as the final branch history.
