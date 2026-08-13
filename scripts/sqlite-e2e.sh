#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d)

cleanup() {
  rm -rf "$tmp"
}
trap cleanup EXIT HUP INT TERM

binary="$tmp/gobackend"
project="$tmp/product-api"
go build -o "$binary" ./cmd/gobackend
GOBACKEND_DEVELOPMENT_REPLACE="$root" "$binary" new "$project" --module example.com/product-api

cd "$project"
go tool gobackend add "$root/examples/product.yaml"
go tool gobackend add "$root/examples/defaults.yaml"
go tool gobackend generate
go tool gobackend check
CGO_ENABLED=0 go test -race ./...
go vet ./...
go tool govulncheck ./...
