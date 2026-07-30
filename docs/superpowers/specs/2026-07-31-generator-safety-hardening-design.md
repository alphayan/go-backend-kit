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
contains a format version and the SHA-256 digest of every generated file except
the manifest itself.

Generation follows these rules:

1. Desired files are rendered completely, including unmodified GORM CLI output.
2. Existing desired targets may be replaced only when they contain either the
   gobackend marker or, for the exact `gormgen/query_gen.go` target shape, the
   official GORM CLI marker.
3. Stale candidates come from the previous manifest, never from a recursive
   repository-wide marker scan.
4. A stale file is deleted only when its current digest still matches the
   previous manifest. Modified files are preserved and cause a clear error.
5. Projects without a manifest use a one-time, narrowly scoped legacy scan that
   recognizes gobackend's own marker only. Generic GORM output is not deleted
   during legacy discovery.

This preserves official GORM output, prevents deletion in `.worktrees`,
`vendor`, or unrelated packages, and retains deterministic stale cleanup after
the manifest has been established.

## Exact numeric constraints

`min` and `max` remain YAML numbers, but the parser stores them as a canonical
decimal number rather than `float64`.

- `int32` and `int64` values compare through exact decimal conversion.
- `decimal.Decimal` values compare directly without a float round-trip.
- `float64` values retain normal float64 semantics and are converted using their
  round-trip decimal representation.
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
handlers. The target is always the exact container name created by the current
script, so anonymous volumes are removed without broad pruning or touching
named application volumes.

## Error handling

- Unsafe stale-file deletion fails before desired files are installed.
- Invalid or colliding schema definitions fail during rendering, before file
  replacement.
- Invalid numeric constraints retain field-index and field-name context.
- Temporary-directory cleanup remains best effort but is explicitly handled so
  static analysis does not report ignored cleanup errors.

## Testing and verification

Each behavior is implemented through a red-green regression:

- unrelated official GORM output and nested worktree files remain untouched;
- manifest-owned unchanged stale files are removed, while modified stale files
  are preserved with an error;
- `2^53 + 1` is rejected against a `2^53` maximum for both `int64` and Decimal;
- exact default-versus-bound validation rejects the same boundary violation;
- `Error` and derived component-name collisions fail generation;
- both Docker cleanup scripts remove the created container and anonymous volume.

Final verification includes formatting, root and generated-project tests,
`go test -race ./...`, `go vet ./...`, `golangci-lint`, `govulncheck`, module
tidiness, and the PostgreSQL migration/contract path when Docker is available.
