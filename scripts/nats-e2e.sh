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

if [ -z "${TEST_NATS_URL:-}" ]; then
  container_name="go-backend-kit-nats-e2e-$$"
  created_container=$(docker run -d --name "$container_name" \
    -p 127.0.0.1::4222 \
    nats:2.14.5-alpine3.22)
  container=$created_container
  port=$(docker port "$container" 4222/tcp | head -n 1 | awk -F: '{print $NF}')
  TEST_NATS_URL="nats://127.0.0.1:${port}"
  export TEST_NATS_URL
  attempts=0
  until docker exec "$container" wget -q -O- http://127.0.0.1:8222/healthz >/dev/null 2>&1; do
    attempts=$((attempts + 1))
    if [ "$attempts" -ge 30 ]; then
      echo "NATS did not become ready" >&2
      exit 1
    fi
    sleep 1
  done
fi

binary="$tmp/gobackend"
project="$tmp/product-api"
go build -o "$binary" ./cmd/gobackend
GOBACKEND_DEVELOPMENT_REPLACE="$root" "$binary" new "$project" --module example.com/product-api --messaging nats

cd "$project"
go tool gobackend add "$root/examples/product.yaml"
go tool gobackend generate
go tool gobackend check
NATS_URL="$TEST_NATS_URL" TEST_NATS_URL="$TEST_NATS_URL" CGO_ENABLED=0 go test -race ./...
go vet ./...
go tool govulncheck ./...
