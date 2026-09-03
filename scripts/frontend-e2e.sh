#!/bin/sh
set -eu

profile=${1:-personal}
case "$profile" in personal|production) ;; *) echo "usage: $0 [personal|production]" >&2; exit 2 ;; esac

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/go-backend-kit-frontend-e2e.XXXXXX")

cleanup() {
  result=$?
  if [ "$result" -eq 0 ]; then
    rm -rf -- "$work"
  else
    echo "Frontend failure artifacts retained at $work" >&2
  fi
}
trap cleanup EXIT HUP INT TERM

project="$work/api"
GOBACKEND_DEVELOPMENT_REPLACE="$root" go run "$root/cmd/gobackend" new "$project" \
  --module example.com/frontend-e2e \
  --http echo \
  --database sqlite \
  --auth session \
  --profile "$profile"

cp "$root/examples/product.yaml" "$work/product.yaml"
cp "$root/examples/defaults.yaml" "$work/defaults.yaml"
(
  cd "$project"
  go tool gobackend add "$work/product.yaml"
  go tool gobackend add "$work/defaults.yaml"
  go tool gobackend check
  cp "$root/scripts/frontend-resource.spec.ts" web/e2e/resource.spec.ts
  if [ "$profile" = production ]; then
    mkdir -p .gobackend/tls-proxy
    cp "$root/scripts/frontend-tls-proxy/main.go" .gobackend/tls-proxy/main.go
    cp "$root/scripts/frontend-tls.config.ts" web/playwright.config.ts
    cp "$root/scripts/frontend-tls.spec.ts" web/e2e/tls.spec.ts
  fi
  pnpm install --frozen-lockfile
  pnpm frontend:typecheck
  pnpm frontend:test
  pnpm frontend:build
  go test ./web
  ./scripts/atlas.sh migrate diff browser_test --env local
  ./scripts/atlas.sh migrate apply --env ci
  PLAYWRIGHT_SKIP_BROWSER_GC=1 pnpm exec playwright install chromium
  E2E_DISPOSABLE_DATABASE=1 pnpm frontend:e2e
)
