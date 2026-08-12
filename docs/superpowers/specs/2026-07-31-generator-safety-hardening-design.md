# Generator Safety Hardening Design

## Goal

Fix the four confirmed generator defects without changing the supported YAML
Schema v1 surface or replacing the official GORM CLI field-helper workflow.

## GORM CLI contract

Generated projects continue to pin `gorm.io/cli/gorm` as a Go tool dependency
and invoke:

```text
go tool gorm gen -i <generated-model> -o <generated-output>
```

The CLI remains the sole producer of `gormgen/query_gen.go`. Gobackend does not
rewrite the generated Go source or replace the typed field helpers with a
home-grown implementation. This follows the official GORM CLI workflow: models
are the input, generated field helpers are plain Go output, and those helpers
are used for typed predicates and ordering.

References:

- https://gorm.io/cli/index.html
- https://gorm.io/cli/workflow.html
- https://gorm.io/cli/field_helpers.html

## Generated-file ownership

Gobackend will add a tracked `.gobackend-generated.json` manifest. The manifest
contains an owner, a format version, and the SHA-256 digest of every generated
file except the manifest itself. The manifest is untrusted repository input:

- only a genuinely missing manifest enables legacy discovery;
- malformed JSON, an unknown owner or version, an invalid digest, or an unsafe
  path fails closed without installing or deleting files;
- file names use canonical `/`-separated project-relative paths and must match
  the exact supported generated-output shapes; empty paths, absolute paths,
  `.`/`..`, backslashes, Windows volume paths, and invalid resource package
  segments are rejected; and
- every manifest-driven read, replacement, and deletion is confined to the
  opened project root and refuses symlink traversal outside it. On the supported
  Go toolchain this uses `os.OpenRoot`/`os.Root`, not unchecked `root + path`
  joins.

Generation follows these rules:

1. Desired files are rendered and validated completely in a staging directory,
   including unmodified GORM CLI output, before the project is mutated.
2. Existing desired targets may be replaced only when they contain the
   format-specific gobackend ownership marker or, for the exact
   `internal/resources/<package>/gormgen/query_gen.go` shape, the official GORM
   CLI marker. An existing manifest is replaceable only after its owner,
   version, digests, and paths have been validated.
3. Stale candidates come from the previous manifest, never from a recursive
   repository-wide marker scan.
4. A stale file is deleted only when its current digest still matches the
   previous manifest. Modified files are preserved and cause a clear error.
5. Projects without a manifest use a one-time legacy scan limited to generated
   Go files directly inside `internal/resources/<package>`. The gobackend
   `DO NOT EDIT` marker is the explicit legacy ownership proof; marked stale
   files may be removed, while generic GORM output, nested directories, and all
   other paths are ignored. This compatibility rule does not claim to detect
   edits made while retaining the generated marker.

Installation preserves a retryable ownership state. After all overwrite and
stale-digest checks pass, non-manifest desired files are atomically replaced in
deterministic order, stale files are removed, and the new manifest is atomically
replaced last. If a replacement or removal fails, the previous manifest remains
in place; ordinary files need not be rolled back, but the new manifest must
never claim an incomplete installation. A retry must safely converge.

This preserves official GORM output, prevents deletion in `.worktrees`,
`vendor`, or unrelated packages, and retains deterministic stale cleanup after
the manifest has been established.

## Exact numeric constraints

`min` and `max` remain YAML numbers, but the parser stores them as a canonical
decimal number rather than `float64`. Numeric YAML scalars are parsed from their
original lexical value. Both the input token and its expanded canonical decimal
form are limited to 1 MiB so exponent expansion cannot cause unbounded memory
allocation.

- `int32` and `int64` values compare through exact decimal conversion.
- `decimal.Decimal` values and defaults compare directly without a float
  round-trip. Quoted and unquoted Decimal defaults normalize to the same
  canonical decimal representation.
- `float64` values retain normal float64 semantics and are converted using their
  round-trip decimal representation. Float64 defaults are normalized to the
  actual decoded float64 value before comparison.
- Integer constraints must contain at least one value in the target integer
  range after ceiling/flooring. Float64 constraints must contain at least one
  finite representable value; the check accounts for subnormal values and
  adjacent floats rather than treating every decimal underflow as zero.
- Integer-range validation, default validation, generated literals, OpenAPI
  bounds, and resource fingerprints all use the same canonical value.
- Existing ordinary constraints keep numeric JSON/OpenAPI output; constraints
  are not converted to quoted JSON strings.

The generated validation API accepts canonical bound strings and converts both
the value and bound to `decimal.Decimal` before comparison.

## OpenAPI schema namespace

Before building the document, gobackend reserves the fixed `Error` schema and
then claims the model, `Create<Name>Input`, and `Update<Name>Input` names for
every resource. Any duplicate claim returns a deterministic error naming both
owners. The generator does not silently overwrite schemas or rename existing
public components.

## Docker lifecycle

Both locally created PostgreSQL containers use `docker rm -fv` in their cleanup
handlers. A cleanup target remains empty until `docker run -d` succeeds, after
which it holds the returned container ID. Cleanup operates only on that ID. A
failure before container creation, a name collision, or a failed `docker run`
must not remove any container. Successful cleanup removes the created container
and its anonymous volumes without broad pruning or touching named application
volumes.

## Error handling

- Unsafe stale-file deletion fails before desired files are installed.
- Invalid manifests and root-escape attempts fail closed before any mutation.
- The previous manifest remains installed until every ordinary replacement and
  stale deletion has succeeded.
- Invalid or colliding schema definitions fail during rendering, before file
  replacement.
- Invalid numeric constraints retain field-index and field-name context.
- Temporary-directory cleanup remains best effort but is explicitly handled so
  static analysis does not report ignored cleanup errors.

## Testing and verification

Each behavior is implemented through a red-green regression:

- unrelated official GORM output and nested worktree files remain untouched;
- manifest traversal, platform-specific separators, and symlink-parent escapes
  fail without changing files outside the project root;
- manifest-owned unchanged stale files are removed, while modified stale files
  are preserved with an error;
- an injected stale-removal failure leaves the previous manifest intact and a
  retry completes cleanup;
- `2^53 + 1` is rejected against a `2^53` maximum for both `int64` and Decimal;
- quoted and unquoted Decimal defaults retain exact default-versus-bound
  behavior;
- oversized numeric tokens or canonical expansions are rejected, while valid
  float64 subnormal bounds are accepted and empty adjacent-float intervals are
  rejected;
- `Error` and derived component-name collisions fail generation;
- both Docker cleanup scripts remove the successfully created container by ID
  and its anonymous volume; pre-creation and `docker run` failures perform no
  container removal.

Final verification includes formatting, root and generated-project tests,
`go test -race ./...`, `go vet ./...`, `golangci-lint`, `govulncheck`, module
tidiness, and the PostgreSQL migration/contract path when Docker is available.
