#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d)
container=""

cleanup() {
  if [ -n "$container" ]; then
    docker rm -fv "$container" >/dev/null 2>&1 || true
  fi
  rm -rf "$tmp"
}
trap cleanup EXIT HUP INT TERM

if [ -z "${TEST_REDIS_URL:-}" ]; then
  container_name="go-backend-kit-redis-e2e-$$"
  created_container=$(docker run -d --name "$container_name" \
    -p 127.0.0.1::6379 \
    redis:8.10.0-alpine3.23)
  container=$created_container
  port=$(docker port "$container" 6379/tcp | head -n 1 | awk -F: '{print $NF}')
  TEST_REDIS_URL="redis://127.0.0.1:${port}/0"
  export TEST_REDIS_URL
  attempts=0
  until docker exec "$container" redis-cli ping >/dev/null 2>&1; do
    attempts=$((attempts + 1))
    if [ "$attempts" -ge 30 ]; then
      echo "Redis did not become ready" >&2
      exit 1
    fi
    sleep 1
  done
fi

binary="$tmp/gobackend"
project="$tmp/product-api"
go build -o "$binary" ./cmd/gobackend
GOBACKEND_DEVELOPMENT_REPLACE="$root" "$binary" new "$project" --module example.com/product-api --cache redis

cd "$project"
go tool gobackend add "$root/examples/product.yaml"
go tool gobackend generate
go tool gobackend check
REDIS_URL="$TEST_REDIS_URL" TEST_REDIS_URL="$TEST_REDIS_URL" CGO_ENABLED=0 go test -race ./...
go vet ./...
go tool govulncheck ./...
