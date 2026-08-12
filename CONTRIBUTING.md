# Contributing

Use Go 1.26.5 or newer. Changes to generator behavior should start with a failing test and include generated-project coverage when applicable.

Before opening a pull request, run:

```bash
gofmt -w .
go test -race ./...
go vet ./...
go tool govulncheck ./...
```

These commands remain the authoritative CI and release gates. Generated output is part of the public contract. Preserve deterministic ordering, never overwrite handwritten files, and keep database or filesystem errors out of API responses.

## Local quality toolkit

This repository vendors [make-toolkit](https://github.com/alphayan/make-toolkit) at commit `a535269` under `tools/make-toolkit` for local development checks. It applies only to this kit repo. Generated-project Makefiles (`internal/generate/scaffold/Makefile.tmpl`) are out of scope.

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
