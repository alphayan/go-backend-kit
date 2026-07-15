#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d)
container=""

cleanup() {
  if [ -n "$container" ]; then
    docker rm -f "$container" >/dev/null 2>&1 || true
  fi
  rm -rf "$tmp"
}
trap cleanup EXIT HUP INT TERM

if [ -z "${DATABASE_URL:-}" ]; then
  container="go-backend-kit-e2e-$$"
  docker run -d --name "$container" \
    -e POSTGRES_DB=kit_test \
    -e POSTGRES_USER=kit \
    -e POSTGRES_PASSWORD=kit \
    -P postgres:17-alpine >/dev/null
  port=$(docker port "$container" 5432/tcp | head -n 1 | awk -F: '{print $NF}')
  DATABASE_URL="postgres://kit:kit@localhost:${port}/kit_test?sslmode=disable"
  export DATABASE_URL
  attempts=0
  until docker exec "$container" pg_isready -U kit -d kit_test >/dev/null 2>&1; do
    attempts=$((attempts + 1))
    if [ "$attempts" -ge 30 ]; then
      echo "PostgreSQL did not become ready" >&2
      exit 1
    fi
    sleep 1
  done
fi

binary="$tmp/gobackend"
project="$tmp/product-api"
go build -o "$binary" ./cmd/gobackend
GOBACKEND_DEVELOPMENT_REPLACE="$root" "$binary" new "$project" --module example.com/product-api

cd "$project"
go tool gobackend add "$root/examples/product.yaml"
go tool gobackend add "$root/examples/defaults.yaml"
go tool gobackend generate
go tool gobackend check

./scripts/atlas.sh migrate diff initial --env local
./scripts/atlas.sh migrate apply --env ci
drift=$(./scripts/atlas.sh schema diff --env ci --from env://url --to env://src --exclude atlas_schema_revisions --format '{{ sql . "  " }}')
if [ -n "$drift" ]; then
  echo "applied migrations do not match the generated GORM schema" >&2
  echo "$drift" >&2
  exit 1
fi
TEST_DATABASE_URL="$DATABASE_URL" go test -race ./...
go vet ./...
go tool govulncheck ./...
