# Contributing

Use Go 1.27.1 or newer. Changes to generator behavior should start with a failing test and include generated-project coverage when applicable.

Before opening a pull request, run:

```bash
gofmt -w .
go test -race -timeout 30m ./...
go vet ./...
go tool govulncheck ./...
```

These commands remain the authoritative local quality and release gates. The repository's automatic branch and pull-request `ci` workflow is disabled; run the checks locally before pushing. The tag-triggered `release` workflow remains enabled. Generated output is part of the public contract. Preserve deterministic ordering, never overwrite handwritten files, and keep database or filesystem errors out of API responses.

## Release checklist

Source availability, a pushed tag, a passing `release` workflow and deployment are separate states. Publish a version only in this order:

1. Run the local checks above and the PostgreSQL, production and Session frontend integration gates before landing the code on `main`. Record any platform coverage that could not be verified locally; the `release` workflow must pass its full platform matrix before publishing binaries.
2. In the release commit, replace the source-checkout block at the top of `README.md` and `README.zh-CN.md` with `go install github.com/alphayan/go-backend-kit/cmd/gobackend@<version>`, where `<version>` equals `generate.CurrentVersion`. `TestReadmesInstallInstructionsAreConsistent` rejects any other version; the source-checkout instructions must stay documented further down.
3. Tag that commit `<version>` (already including the `v` prefix) and push the tag. The `release` workflow reruns every gate and publishes binaries only if all of them pass.
4. Only then bump `generate.CurrentVersion` for the next cycle and restore the source-checkout instructions as the primary path.

Binaries built with `go build` from a modified checkout are stamped `+dirty` and are treated as source builds: they require `GOBACKEND_DEVELOPMENT_REPLACE`. `go install ...@<tag or pushed commit>` binaries pin that module version and need no replacement.

## Local quality toolkit

This repository vendors [make-toolkit](https://github.com/alphayan/make-toolkit) at commit `a535269` under `tools/make-toolkit` for local development checks. It applies only to this kit repo. Generated-project Makefiles (`internal/generate/scaffold/common/Makefile.tmpl`) are out of scope.

```bash
make tk-help         # list toolkit targets
make test            # unit tests in short mode; this repo has no testing.Short skips and covers all packages
make race-check      # go test -race on all packages
make quality-check   # go vet + golangci-lint v2.12.2
make scan            # govulncheck + Trivy
make lint            # quality-check + scan
make format          # gofumpt + goimports + modernize
```

`make test` passes `-short`, but no test in this repository calls `testing.Short()`, so the suite still runs in full and includes `cmd` and `internal`. Default toolkit excludes (`/cmd`, `docs`, and similar) are overridden so those packages are not skipped.
