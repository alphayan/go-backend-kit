# Contributing

Use Go 1.26.5 or newer. Changes to generator behavior should start with a failing test and include generated-project coverage when applicable.

Before opening a pull request, run:

```bash
gofmt -w .
go test -race ./...
go vet ./...
go tool govulncheck ./...
```

Generated output is part of the public contract. Preserve deterministic ordering, never overwrite handwritten files, and keep database or filesystem errors out of API responses.
