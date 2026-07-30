# Generator Safety Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix generated-file ownership, exact numeric constraints, OpenAPI component collisions, and PostgreSQL test-container volume cleanup while retaining the official GORM CLI field-helper workflow.

**Architecture:** Keep `go tool gorm gen` as the unmodified producer of field-helper Go code. Add a digest manifest for precise ownership, represent YAML bounds as canonical decimal numbers, validate the OpenAPI component namespace before rendering, and exercise Docker cleanup through fake-command script tests.

**Tech Stack:** Go 1.26.5, `gorm.io/cli/gorm` v0.2.4, `shopspring/decimal`, YAML v3, OpenAPI 3.1, POSIX shell, Docker.

## Global Constraints

- Preserve YAML Schema v1 syntax and supported field types.
- Preserve the pinned official `go tool gorm gen -i ... -o ...` invocation.
- Do not modify the bytes emitted by the official GORM CLI.
- Never recursively infer ownership from the generic GORM marker.
- Never prune Docker volumes broadly; clean only the exact temporary container.
- Every production behavior starts with a failing regression test.

---

### Task 1: Manifest-based generated-file ownership

**Files:**
- Create: `internal/generate/manifest.go`
- Modify: `internal/generate/generator.go`
- Modify: `internal/generate/project_test.go`

**Interfaces:**
- Produces: `addGeneratedManifest(map[string][]byte) error`
- Produces: `staleGenerated(root string, desired map[string][]byte) ([]string, error)`
- Produces: `.gobackend-generated.json` with format version and SHA-256 digests
- Consumes: unchanged official GORM CLI output from `renderDesired`

- [ ] **Step 1: Write a failing preservation test**

Add `TestGeneratePreservesUnownedGeneratedFiles` to `project_test.go`. Create a
fresh project, then write the official GORM marker into these three files:

```go
paths := []string{
	"internal/custom/query_gen.go",
	"internal/resources/custom/gormgen/query_gen.go",
	".worktrees/other/internal/resources/task/gormgen/query_gen.go",
}
```

Run `Generator.Generate` and assert every file still exists with identical
contents. The current recursive scan must fail this test by deleting them.

- [ ] **Step 2: Run the preservation test and verify RED**

Run:

```bash
go test ./internal/generate -run TestGeneratePreservesUnownedGeneratedFiles -count=1
```

Expected: FAIL because at least one unrelated official GORM output is missing.

- [ ] **Step 3: Write failing stale-file integrity tests**

Add two cases:

```go
func TestGenerateRemovesManifestOwnedUnchangedStaleFiles(t *testing.T)
func TestGeneratePreservesModifiedManifestOwnedStaleFiles(t *testing.T)
```

Both create a project, add a resource, and remove its YAML. The first asserts
unchanged generated resource files disappear. The second changes
`model_gen.go`, expects generation to fail with a modified-stale-file error, and
asserts the edited file remains.

- [ ] **Step 4: Run stale-file tests and verify RED**

Run:

```bash
go test ./internal/generate -run 'TestGenerate(RemovesManifestOwnedUnchangedStaleFiles|PreservesModifiedManifestOwnedStaleFiles)' -count=1
```

Expected: FAIL because no manifest exists and modified stale files are deleted.

- [ ] **Step 5: Implement the manifest**

Create `manifest.go` with:

```go
const generatedManifestName = ".gobackend-generated.json"

type generatedManifest struct {
	GeneratedBy string            `json:"generated_by"`
	Version     int               `json:"version"`
	Files       map[string]string `json:"files"`
}
```

Use SHA-256 hex digests. Validate every loaded path with slash-based cleaning,
`filepath.IsAbs`, and `..` rejection before reading or deleting it. Build the
desired manifest after all GORM helpers exist, exclude the manifest from its own
file map, and marshal it deterministically with the existing `marshalJSON`.

When a previous manifest exists, stale files are previous entries absent from
`desired`. Delete only exact digest matches. Return an error before installation
when a stale file was modified.

For projects without a manifest, inspect only known gobackend-owned resource
filenames directly below `internal/resources/<package>` and accept only the
gobackend marker. Do not recognize the generic GORM marker during legacy stale
discovery.

- [ ] **Step 6: Restrict replacement ownership**

Update `isManagedGenerated` so the official GORM marker is accepted only for
the exact path shape:

```text
internal/resources/<package>/gormgen/query_gen.go
```

Recognize `.gobackend-generated.json` by successfully parsing its
`generated_by` and version fields. Remove the repository-wide `WalkDir`.

- [ ] **Step 7: Verify GREEN**

Run:

```bash
go test ./internal/generate -run 'TestGenerate|TestCheck|TestNewAddGenerateAndCheck' -count=1
go test ./internal/generate -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/generate/manifest.go internal/generate/generator.go internal/generate/project_test.go
git commit -m "fix: scope generated file ownership"
```

---

### Task 2: Exact numeric bounds from YAML through runtime validation

**Files:**
- Create: `internal/spec/number.go`
- Create: `internal/spec/number_test.go`
- Modify: `internal/spec/spec.go`
- Modify: `internal/spec/spec_test.go`
- Modify: `internal/generate/render.go`
- Modify: `internal/generate/openapi.go`
- Create: `internal/generate/scaffold/internal/platform/validation/validation_test.go.tmpl`
- Modify: `internal/generate/scaffold/internal/platform/validation/validation.go.tmpl`

**Interfaces:**
- Produces: `spec.Number` with canonical `String`, exact Decimal conversion,
  YAML decoding, and numeric JSON encoding
- Changes: `spec.Field.Min` and `spec.Field.Max` from `*float64` to `*Number`
- Changes generated validation calls to `Min(..., bound string)` and
  `Max(..., bound string)`

- [ ] **Step 1: Write failing parser boundary tests**

Add invalid-spec cases for:

```yaml
- name: sequence
  type: int64
  default: 9007199254740993
  max: 9007199254740992
```

and:

```yaml
- name: amount
  type: decimal
  default: "9007199254740993"
  max: 9007199254740992
```

- [ ] **Step 2: Verify parser tests RED**

Run:

```bash
go test ./internal/spec -run TestParseRejectsInvalidSpecs -count=1
```

Expected: FAIL because both invalid defaults are accepted.

- [ ] **Step 3: Write `Number` behavior tests**

Test that YAML `1.00`, `1e2`, and `+0` normalize to `1`, `100`, and `0`; quoted
strings and non-numeric scalars fail; JSON output remains an unquoted number;
and `9007199254740993` remains exact.

- [ ] **Step 4: Implement canonical `spec.Number`**

Back `Number` with a canonical decimal string. `UnmarshalYAML` accepts only
`!!int` and `!!float` scalar nodes and validates with
`decimal.NewFromString`. `MarshalJSON` emits the already validated canonical
number bytes. Provide:

```go
func (n Number) String() string
func (n Number) Decimal() decimal.Decimal
```

- [ ] **Step 5: Replace float comparisons in spec validation**

Use `decimal.Decimal` for:

- integer representable-range intersection;
- default-versus-min/max checks;
- exact decimal defaults;
- canonical field fingerprint JSON.

Continue rejecting bounds that cannot be represented by a `float64` field.

- [ ] **Step 6: Verify spec GREEN**

Run:

```bash
go test ./internal/spec -count=1
```

Expected: PASS.

- [ ] **Step 7: Write a generated validation regression**

Add `validation_test.go.tmpl` with a table that calls generated `Max` using:

```go
int64(9007199254740993)
decimal.RequireFromString("9007199254740993")
```

against bound `"9007199254740992"` and requires both to populate validation
details.

- [ ] **Step 8: Verify generated validation RED**

Run:

```bash
go test ./internal/generate -run TestGeneratedProjectCompilesAndRunsContract -count=1
```

Expected: FAIL because the old generated validation API accepts `float64` and
loses precision.

- [ ] **Step 9: Implement exact generated comparisons**

Change generated `Min` and `Max` to parse the bound string with
`decimal.RequireFromString`. Convert `int32`, `int64`, finite `float64`, and
`decimal.Decimal` values to Decimal without routing integer or Decimal values
through float64.

Update render helpers to use canonical bound strings and exact Decimal
clamping. Emit explicitly typed `int32(...)` and `int64(...)` contract values.
Pass `spec.Number` directly into OpenAPI maps so JSON bounds remain numeric.

- [ ] **Step 10: Verify GREEN**

Run:

```bash
go test ./internal/generate -run 'TestGeneratedProjectCompilesAndRunsContract|TestOpenAPI' -count=1
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add internal/spec internal/generate
git commit -m "fix: preserve numeric constraint precision"
```

---

### Task 3: Reject OpenAPI component-name collisions

**Files:**
- Modify: `internal/generate/openapi.go`
- Modify: `internal/generate/openapi_test.go`
- Modify: `internal/generate/render.go`

**Interfaces:**
- Changes: `buildOpenAPI([]spec.Resource) (map[string]any, error)`
- Produces deterministic ownership errors for fixed and derived component names

- [ ] **Step 1: Write failing collision tests**

Add table cases using `renderGenerated`:

```text
single Error resource
Foo plus CreateFooInput
Foo plus UpdateFooInput
```

Each must return an error containing the conflicting component name and both
owners.

- [ ] **Step 2: Verify RED**

Run:

```bash
go test ./internal/generate -run TestOpenAPIRejectsComponentNameCollisions -count=1
```

Expected: FAIL because map assignments silently overwrite schemas.

- [ ] **Step 3: Implement namespace claims**

Reserve `Error`, then claim each resource's model, create input, and update
input names before building paths. Store a human-readable owner for every
claim. Return before constructing the document when a name is already owned.

- [ ] **Step 4: Update callers and verify GREEN**

Run:

```bash
go test ./internal/generate -run TestOpenAPI -count=1
go test ./internal/generate -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/generate/openapi.go internal/generate/openapi_test.go internal/generate/render.go
git commit -m "fix: reject OpenAPI schema collisions"
```

---

### Task 4: Remove anonymous PostgreSQL volumes

**Files:**
- Create: `internal/generate/scripts_test.go`
- Modify: `internal/generate/scaffold/scripts/atlas.sh.tmpl`
- Modify: `scripts/postgres-e2e.sh`

**Interfaces:**
- Preserves existing script CLI
- Changes exact temporary-container cleanup from `docker rm -f` to
  `docker rm -fv`

- [ ] **Step 1: Write behavioral shell-script tests**

Create fake `docker` and `go` executables in `t.TempDir()`. The fake Docker
records arguments, returns a deterministic port, and reports readiness. Execute:

- the root PostgreSQL E2E script until the fake Go build fails and triggers its
  trap;
- a rendered Atlas script through successful completion and its trap.

Assert each log contains an invocation shaped as:

```text
rm -fv <the exact generated container name>
```

Do not assert by reading script source text.

- [ ] **Step 2: Verify RED**

Run:

```bash
go test ./internal/generate -run TestTemporaryPostgresCleanupRemovesAnonymousVolumes -count=1
```

Expected: FAIL because both scripts currently use `rm -f`.

- [ ] **Step 3: Implement exact volume cleanup**

Change only the two cleanup invocations to `docker rm -fv`. Keep container names,
traps, and broad-prune avoidance unchanged.

- [ ] **Step 4: Verify GREEN**

Run:

```bash
go test ./internal/generate -run TestTemporaryPostgresCleanupRemovesAnonymousVolumes -count=1
shellcheck -e SC1007 scripts/postgres-e2e.sh internal/generate/scaffold/scripts/atlas.sh.tmpl
```

Expected: both commands exit 0. SC1007 is excluded because `CDPATH= cd` is the
existing intentional POSIX idiom.

- [ ] **Step 5: Commit**

```bash
git add internal/generate/scripts_test.go internal/generate/scaffold/scripts/atlas.sh.tmpl scripts/postgres-e2e.sh
git commit -m "fix: remove temporary postgres volumes"
```

---

### Task 5: Documentation, cleanup, and full verification

**Files:**
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `internal/generate/scaffold/README.md.tmpl`
- Modify: `internal/generate/scaffold/README.zh-CN.md.tmpl`
- Modify: `internal/generate/generator.go`

**Interfaces:**
- Documents the tracked ownership manifest and unchanged official GORM CLI role
- Leaves public CLI commands unchanged

- [ ] **Step 1: Update documentation**

Explain that the official GORM CLI remains the field-helper producer and that
`.gobackend-generated.json` records exact generated-file ownership. Document
that modified stale generated files fail safely instead of being deleted.

- [ ] **Step 2: Resolve cleanup lint findings**

Replace each bare deferred `os.RemoveAll` with an explicit best-effort closure:

```go
defer func() { _ = os.RemoveAll(stage) }()
```

- [ ] **Step 3: Run focused verification**

```bash
gofmt -w internal/generate internal/spec
go test ./...
go test -race ./...
go vet ./...
golangci-lint run ./...
go tool govulncheck ./...
go mod verify
go mod tidy -diff
git diff --check
```

Expected: every command exits 0.

- [ ] **Step 4: Verify official GORM CLI fidelity**

Generate a fresh project and one resource. Independently run the pinned
`go tool gorm gen` against the same generated model into a temporary directory,
then compare the official `model_gen.go` output byte-for-byte with the project's
`gormgen/query_gen.go`.

Expected: identical bytes.

- [ ] **Step 5: Run PostgreSQL E2E**

If Docker is available:

```bash
./scripts/postgres-e2e.sh
```

Expected: migration generation, apply, schema comparison, PostgreSQL contracts,
vet, and vulnerability scan all pass. Afterwards verify no new dangling volume
was created by the exact temporary container.

- [ ] **Step 6: Commit**

```bash
git add README.md README.zh-CN.md internal/generate/scaffold/README.md.tmpl internal/generate/scaffold/README.zh-CN.md.tmpl internal/generate/generator.go
git commit -m "docs: explain safe generated ownership"
```

- [ ] **Step 7: Audit the completed requirements**

Check the current diff and fresh command outputs against all four defects and
the official GORM CLI constraint. No item is complete without direct evidence.
