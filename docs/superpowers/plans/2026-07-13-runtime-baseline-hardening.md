# Runtime Baseline Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Align generated projects with Atlas Community's supported guarantees, separate the Go compatibility floor from the preferred patched toolchain, and add a validated, probed, explicitly closed database connection pool.

**Architecture:** Keep the existing generator and runtime layers. Template-level assertions protect generated baselines; generated config parses pool settings through a testable environment lookup; the database package owns a `Connection` containing GORM and `database/sql` handles; generated `main` bounds startup probing and owns shutdown.

**Tech Stack:** Go 1.26.4 language baseline required by Atlas v1.2.3, Go 1.26.5 toolchain, Echo v5, GORM v1.31.2, PostgreSQL 17, Atlas v1.2.3 Community, pure-Go SQLite tests.

## Global Constraints

- Keep Echo v5, GORM, PostgreSQL 17, Atlas Community, and OpenAPI 3.1.
- Do not change API routes, request/response schemas, search, pagination, authentication, authorization, or observability.
- Do not add standard Atlas, Atlas Cloud, goose, golang-migrate, or a second Go module.
- Keep generated files deterministic and generated-project tests cross-platform.
- Use red-green-refactor for every Go behavior change.
- Modify only files listed by this plan plus mechanically updated `go.sum` if `go mod tidy` requires it.

---

### Task 1: Align Go and Atlas Community Baselines

**Files:**
- Modify: `internal/generate/project_test.go`
- Modify: `go.mod`
- Modify: `internal/generate/scaffold/go.mod.tmpl`
- Modify: `internal/generate/scaffold/Makefile.tmpl`
- Modify: `internal/generate/scaffold/.github/workflows/ci.yml.tmpl`
- Modify: `scripts/postgres-e2e.sh`
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `internal/generate/scaffold/README.md.tmpl`
- Modify: `internal/generate/scaffold/README.zh-CN.md.tmpl`

**Interfaces:**
- Consumes: existing `Generator.New` scaffold rendering and PostgreSQL E2E workflow.
- Produces: generated modules with `go 1.26.4` and `toolchain go1.26.5`; Community-compatible migration checks with no `migrate lint` invocation.

- [ ] **Step 1: Write failing generated-baseline assertions**

Add these checks to `TestNewAddGenerateAndCheck` after reading the generated CI file:

```go
moduleFile, err := os.ReadFile(filepath.Join(root, "go.mod"))
if err != nil {
	t.Fatal(err)
}
if !strings.Contains(string(moduleFile), "go 1.26.4\n\ntoolchain go1.26.5\n") {
	t.Fatal("generated go.mod does not separate the Go minimum from the preferred toolchain")
}
if strings.Contains(string(ci), "migrate lint") {
	t.Fatal("generated CI promises Atlas linting that Community edition does not provide")
}
if !strings.Contains(string(ci), "migrate apply --env ci") ||
	!strings.Contains(string(ci), "schema diff --env ci --from env://url --to env://src") {
	t.Fatal("generated CI does not apply and compare versioned migrations")
}
makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
if err != nil {
	t.Fatal(err)
}
if strings.Contains(string(makefile), "migrate-lint") {
	t.Fatal("generated Makefile exposes unsupported Atlas Community linting")
}
```

- [ ] **Step 2: Run the assertion and verify RED**

Run:

```bash
go test ./internal/generate -run TestNewAddGenerateAndCheck -count=1
```

Expected: FAIL because the generated module still says `go 1.26.5` and generated CI/Makefile still contain `migrate lint`.

- [ ] **Step 3: Separate minimum and preferred Go versions**

Change both `go.mod` and `internal/generate/scaffold/go.mod.tmpl` to:

```go
go 1.26.4

toolchain go1.26.5
```

Keep every GitHub Actions `go-version` at `1.26.5`.

- [ ] **Step 4: Remove unsupported Community lint claims**

In `internal/generate/scaffold/Makefile.tmpl`, remove `migrate-lint` from `.PHONY` and delete:

```make
migrate-lint:
	./scripts/atlas.sh migrate lint --env local --latest 1
```

In generated CI, rename the step to `Apply and compare migrations`, remove the `migrate lint` command, and retain `migrate apply` followed by the existing schema comparison.

In `scripts/postgres-e2e.sh`, remove only:

```sh
./scripts/atlas.sh migrate lint --env ci --latest 1
```

Retain migration generation, application, schema comparison, PostgreSQL contracts, vet, and vulnerability scanning.

- [ ] **Step 5: Correct documentation**

Change root requirements to `Go 1.26.4 or newer` / `Go 1.26.4 或更高版本`, note that Atlas v1.2.3 sets this floor, and recommend Go 1.26.5 or a newer supported patch.

Change generated README requirements the same way. In all four READMEs, state that the pinned Community image supports the project's migration generation/application path, while advanced linting, rollback, migration testing, approval policies, and advanced database-object governance are outside this open-source profile. Preserve the requirement to review generated SQL before applying it.

- [ ] **Step 6: Verify GREEN**

Run:

```bash
go mod tidy
go test ./internal/generate -run TestNewAddGenerateAndCheck -count=1
git diff --check
```

Expected: all commands exit 0; the targeted test reports `ok`.

- [ ] **Step 7: Commit the baseline change**

```bash
git add go.mod go.sum README.md README.zh-CN.md scripts/postgres-e2e.sh internal/generate/project_test.go internal/generate/scaffold
git commit -m "fix: align generated runtime baselines"
```

---

### Task 2: Validate Generated Database Pool Configuration

**Files:**
- Create: `internal/generate/scaffold/internal/platform/config/config_test.go.tmpl`
- Modify: `internal/generate/scaffold/internal/platform/config/config.go.tmpl`
- Modify: `internal/generate/scaffold/.env.example.tmpl`
- Modify: `internal/generate/project_test.go`

**Interfaces:**
- Consumes: `config.Load() (Config, error)` and environment variables.
- Produces: `load(envLookup) (Config, error)` plus five validated pool fields on `Config`.

- [ ] **Step 1: Add failing generated config tests**

Create `config_test.go.tmpl` with tests that call an unexported lookup-driven loader:

```go
package config

import (
	"strings"
	"testing"
	"time"
)

func lookup(values map[string]string) envLookup {
	return func(name string) string { return values[name] }
}

func TestLoadDatabaseDefaults(t *testing.T) {
	cfg, err := load(lookup(map[string]string{"DATABASE_URL": "postgres://example"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseMaxOpenConnections != 25 || cfg.DatabaseMaxIdleConnections != 25 {
		t.Fatalf("pool counts = %d/%d", cfg.DatabaseMaxOpenConnections, cfg.DatabaseMaxIdleConnections)
	}
	if cfg.DatabaseConnectionMaxLifetime != 30*time.Minute || cfg.DatabaseConnectionMaxIdleTime != 5*time.Minute {
		t.Fatalf("pool durations = %s/%s", cfg.DatabaseConnectionMaxLifetime, cfg.DatabaseConnectionMaxIdleTime)
	}
	if cfg.DatabaseConnectTimeout != 5*time.Second {
		t.Fatalf("connect timeout = %s", cfg.DatabaseConnectTimeout)
	}
}

func TestLoadDatabaseOverrides(t *testing.T) {
	cfg, err := load(lookup(map[string]string{
		"DATABASE_URL":             "postgres://example",
		"DB_MAX_OPEN_CONNS":        "12",
		"DB_MAX_IDLE_CONNS":        "4",
		"DB_CONN_MAX_LIFETIME":     "45m",
		"DB_CONN_MAX_IDLE_TIME":    "90s",
		"DB_CONNECT_TIMEOUT":       "3s",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseMaxOpenConnections != 12 || cfg.DatabaseMaxIdleConnections != 4 ||
		cfg.DatabaseConnectionMaxLifetime != 45*time.Minute ||
		cfg.DatabaseConnectionMaxIdleTime != 90*time.Second ||
		cfg.DatabaseConnectTimeout != 3*time.Second {
		t.Fatalf("unexpected overrides: %+v", cfg)
	}
}

func TestLoadRejectsInvalidDatabaseSettings(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{"open syntax", "DB_MAX_OPEN_CONNS", "many", "DB_MAX_OPEN_CONNS must be an integer"},
		{"open zero", "DB_MAX_OPEN_CONNS", "0", "DB_MAX_OPEN_CONNS must be greater than zero"},
		{"idle negative", "DB_MAX_IDLE_CONNS", "-1", "DB_MAX_IDLE_CONNS cannot be negative"},
		{"lifetime negative", "DB_CONN_MAX_LIFETIME", "-1s", "DB_CONN_MAX_LIFETIME cannot be negative"},
		{"idle duration syntax", "DB_CONN_MAX_IDLE_TIME", "later", "DB_CONN_MAX_IDLE_TIME must be a Go duration"},
		{"connect zero", "DB_CONNECT_TIMEOUT", "0", "DB_CONNECT_TIMEOUT must be greater than zero"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := map[string]string{"DATABASE_URL": "postgres://example"}
			values[test.key] = test.value
			_, err := load(lookup(values))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadRejectsIdleConnectionsAboveOpenLimit(t *testing.T) {
	_, err := load(lookup(map[string]string{
		"DATABASE_URL":      "postgres://example",
		"DB_MAX_OPEN_CONNS": "5",
		"DB_MAX_IDLE_CONNS": "6",
	}))
	if err == nil || !strings.Contains(err.Error(), "DB_MAX_IDLE_CONNS cannot exceed DB_MAX_OPEN_CONNS") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	_, err := load(lookup(nil))
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("error = %v", err)
	}
}
```

Add `internal/platform/config/config_test.go` to the expected scaffold files in `TestNewAddGenerateAndCheck`.

- [ ] **Step 2: Run the generated project test and verify RED**

Run:

```bash
go test ./internal/generate -run TestGeneratedProjectCompilesAndRunsContract -count=1
```

Expected: FAIL compiling the generated config tests because `envLookup`, `load`, and the pool fields do not exist.

- [ ] **Step 3: Implement lookup-driven parsing and validation**

In `config.go.tmpl`, add:

```go
type envLookup func(string) string

type Config struct {
	Address                       string
	DatabaseURL                   string
	DatabaseMaxOpenConnections    int
	DatabaseMaxIdleConnections    int
	DatabaseConnectionMaxLifetime time.Duration
	DatabaseConnectionMaxIdleTime time.Duration
	DatabaseConnectTimeout        time.Duration
	CORSOrigins                   []string
	RequestTimeout                time.Duration
}

func Load() (Config, error) { return load(os.Getenv) }
```

Implement `load` using defaults `25`, `25`, `30*time.Minute`, `5*time.Minute`, and `5*time.Second`. Parse integers with `strconv.Atoi`, durations with `time.ParseDuration`, and return the exact validation messages asserted above. Allow zero for max idle, max lifetime, and max idle time; require a positive max-open value and connect timeout; reject max idle above max open. Preserve address, CORS, request timeout, and required `DATABASE_URL` behavior.

- [ ] **Step 4: Document generated environment controls**

Append to `.env.example.tmpl`:

```dotenv
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=25
DB_CONN_MAX_LIFETIME=30m
DB_CONN_MAX_IDLE_TIME=5m
DB_CONNECT_TIMEOUT=5s
```

- [ ] **Step 5: Verify GREEN**

Run:

```bash
gofmt -w internal/generate/scaffold/internal/platform/config/config.go.tmpl internal/generate/scaffold/internal/platform/config/config_test.go.tmpl
go test ./internal/generate -run 'TestNewAddGenerateAndCheck|TestGeneratedProjectCompilesAndRunsContract' -count=1
```

Expected: both tests pass.

- [ ] **Step 6: Commit config validation**

```bash
git add internal/generate/project_test.go internal/generate/scaffold/.env.example.tmpl internal/generate/scaffold/internal/platform/config
git commit -m "feat: validate generated database pool settings"
```

---

### Task 3: Own, Probe, and Close the Generated Database Pool

**Files:**
- Create: `internal/generate/scaffold/internal/platform/database/database_test.go.tmpl`
- Modify: `internal/generate/scaffold/internal/platform/database/database.go.tmpl`
- Modify: `internal/generate/scaffold/cmd/api/main.go.tmpl`
- Modify: `internal/generate/project_test.go`
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `internal/generate/scaffold/README.md.tmpl`
- Modify: `internal/generate/scaffold/README.zh-CN.md.tmpl`

**Interfaces:**
- Consumes: the five validated database settings from `config.Config`.
- Produces: `database.PoolOptions`, `database.Connection`, `database.Open(context.Context, string, PoolOptions) (*Connection, error)`, and `(*Connection).Close() error`.

- [ ] **Step 1: Add failing generated database tests**

Create `database_test.go.tmpl`:

```go
package database

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/libtnb/sqlite"
	"gorm.io/gorm"
)

func openTestSQLDB(t *testing.T) (*gorm.DB, *sql.DB) {
	t.Helper()
	orm, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := orm.DB()
	if err != nil {
		t.Fatal(err)
	}
	return orm, sqlDB
}

func testPoolOptions() PoolOptions {
	return PoolOptions{
		MaxOpenConnections:    7,
		MaxIdleConnections:    3,
		ConnectionMaxLifetime: time.Minute,
		ConnectionMaxIdleTime: 30 * time.Second,
	}
}

func TestConfigureAndPing(t *testing.T) {
	_, sqlDB := openTestSQLDB(t)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := configureAndPing(context.Background(), sqlDB, testPoolOptions()); err != nil {
		t.Fatal(err)
	}
	if got := sqlDB.Stats().MaxOpenConnections; got != 7 {
		t.Fatalf("max open connections = %d", got)
	}
}

func TestConfigureAndPingHonorsCanceledContext(t *testing.T) {
	_, sqlDB := openTestSQLDB(t)
	t.Cleanup(func() { _ = sqlDB.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := configureAndPing(ctx, sqlDB, testPoolOptions()); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestConfigureAndPingRejectsInvalidOptions(t *testing.T) {
	_, sqlDB := openTestSQLDB(t)
	t.Cleanup(func() { _ = sqlDB.Close() })
	options := testPoolOptions()
	options.MaxIdleConnections = options.MaxOpenConnections + 1
	if err := configureAndPing(context.Background(), sqlDB, options); err == nil {
		t.Fatal("invalid pool options were accepted")
	}
}

func TestConnectionClose(t *testing.T) {
	orm, sqlDB := openTestSQLDB(t)
	connection := &Connection{ORM: orm, SQL: sqlDB}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.PingContext(context.Background()); err == nil {
		t.Fatal("closed pool still accepts pings")
	}
}
```

Add `internal/platform/database/database_test.go` to the expected scaffold files.

- [ ] **Step 2: Run the generated project test and verify RED**

Run:

```bash
go test ./internal/generate -run TestGeneratedProjectCompilesAndRunsContract -count=1
```

Expected: FAIL compiling the generated database tests because `PoolOptions`, `Connection`, and `configureAndPing` do not exist.

- [ ] **Step 3: Implement connection ownership and startup probing**

Replace `database.go.tmpl` with an implementation containing:

```go
type PoolOptions struct {
	MaxOpenConnections    int
	MaxIdleConnections    int
	ConnectionMaxLifetime time.Duration
	ConnectionMaxIdleTime time.Duration
}

type Connection struct {
	ORM *gorm.DB
	SQL *sql.DB
}

func Open(ctx context.Context, dsn string, options PoolOptions) (*Connection, error) {
	orm, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		TranslateError: true,
		NowFunc:        func() time.Time { return time.Now().UTC() },
		Logger:         newSlogLogger(slog.Default()),
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	sqlDB, err := orm.DB()
	if err != nil {
		return nil, fmt.Errorf("access database pool: %w", err)
	}
	if err := configureAndPing(ctx, sqlDB, options); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return &Connection{ORM: orm, SQL: sqlDB}, nil
}

func configureAndPing(ctx context.Context, db *sql.DB, options PoolOptions) error {
	if options.MaxOpenConnections <= 0 {
		return errors.New("max open connections must be greater than zero")
	}
	if options.MaxIdleConnections < 0 || options.MaxIdleConnections > options.MaxOpenConnections {
		return errors.New("max idle connections must be between zero and max open connections")
	}
	if options.ConnectionMaxLifetime < 0 || options.ConnectionMaxIdleTime < 0 {
		return errors.New("connection lifetimes cannot be negative")
	}
	db.SetMaxOpenConns(options.MaxOpenConnections)
	db.SetMaxIdleConns(options.MaxIdleConnections)
	db.SetConnMaxLifetime(options.ConnectionMaxLifetime)
	db.SetConnMaxIdleTime(options.ConnectionMaxIdleTime)
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	return nil
}

func (connection *Connection) Close() error {
	if connection == nil || connection.SQL == nil {
		return nil
	}
	return connection.SQL.Close()
}
```

Add imports for `context`, `database/sql`, `errors`, and `fmt`.

- [ ] **Step 4: Wire generated process lifecycle**

Refactor `cmd/api/main.go.tmpl` to use `main -> os.Exit(run())`, with `run` returning a named `exitCode`. Before constructing the Echo app:

```go
startupContext, cancelStartup := context.WithTimeout(context.Background(), cfg.DatabaseConnectTimeout)
connection, err := database.Open(startupContext, cfg.DatabaseURL, database.PoolOptions{
	MaxOpenConnections:    cfg.DatabaseMaxOpenConnections,
	MaxIdleConnections:    cfg.DatabaseMaxIdleConnections,
	ConnectionMaxLifetime: cfg.DatabaseConnectionMaxLifetime,
	ConnectionMaxIdleTime: cfg.DatabaseConnectionMaxIdleTime,
})
cancelStartup()
if err != nil {
	logger.Error("database connection failed", "error", err)
	return 1
}
defer func() {
	if err := connection.Close(); err != nil {
		logger.Error("database close failed", "error", err)
		exitCode = 1
	}
}()
handler := app.New(cfg, connection.ORM, logger)
```

Replace startup `os.Exit(1)` calls with `return 1`, replace the existing
`exitCode := 0` declaration with assignments to the named return value, retain
the existing server timeouts and shutdown behavior, and return `exitCode` after
shutdown. This structure ensures deferred pool closure runs even when the server
exits with an error.

- [ ] **Step 5: Document runtime pool defaults**

Add the five environment variables and their defaults to both root READMEs and
both generated README templates. State that startup performs a bounded database
probe and fails closed if PostgreSQL is unreachable.

- [ ] **Step 6: Verify GREEN**

Run:

```bash
gofmt -w internal/generate/scaffold/internal/platform/database/database.go.tmpl internal/generate/scaffold/internal/platform/database/database_test.go.tmpl internal/generate/scaffold/cmd/api/main.go.tmpl
go test ./internal/generate -run 'TestNewAddGenerateAndCheck|TestGeneratedProjectCompilesAndRunsContract' -count=1
```

Expected: generated database tests and the full generated project compile/contract test pass.

- [ ] **Step 7: Commit database lifecycle hardening**

```bash
git add README.md README.zh-CN.md internal/generate/project_test.go internal/generate/scaffold/cmd/api/main.go.tmpl internal/generate/scaffold/internal/platform/database internal/generate/scaffold/README.md.tmpl internal/generate/scaffold/README.zh-CN.md.tmpl
git commit -m "feat: harden generated database lifecycle"
```

---

### Task 4: Full Regression and Real PostgreSQL Verification

**Files:**
- Verify: all files changed in Tasks 1-3

**Interfaces:**
- Consumes: the completed generator, scaffold, and E2E workflow.
- Produces: fresh evidence that deterministic generation, runtime contracts, migration application, and security checks still pass.

- [ ] **Step 1: Verify formatting and a clean module graph**

Run:

```bash
test -z "$(gofmt -l .)"
go mod tidy
git diff --check
go mod verify
```

Expected: no formatting paths, no diff errors, and `all modules verified`.

- [ ] **Step 2: Run all local checks**

Run:

```bash
go test -race ./...
go vet ./...
go tool govulncheck ./...
```

Expected: all packages pass, vet emits no diagnostics, and govulncheck reports no called vulnerabilities.

- [ ] **Step 3: Run real PostgreSQL and Atlas E2E**

Run:

```bash
./scripts/postgres-e2e.sh
```

Expected: Atlas generates and applies a migration, the schema comparison is empty, generated contracts pass against PostgreSQL, vet passes, and govulncheck reports zero called vulnerabilities. Output must contain no Atlas `migrate lint` phase.

- [ ] **Step 4: Verify scope and repository state**

Run:

```bash
rg -n "migrate lint|migrate-lint" . -g '!docs/superpowers/**'
git status --short
git diff HEAD~3 --stat
```

Expected: the search has no results; status is clean; the three implementation commits contain only the planned runtime-baseline files.

- [ ] **Step 5: Record final commit state**

No new commit is required if Steps 1-4 leave the tree clean. Record `git log -4 --oneline` and the exact verification outcomes in the final handoff.
