package generate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/alphayan/go-backend-kit/internal/spec"
)

// Includes the unused-query correction for Fiber resources without filters.
const legacyGeneratedOutputSHA256 = "7ffb40b870109fd6b7988b16315d8db6268148ade6d6680b5698aa2e3efeb136"

func TestLegacyGeneratedOutputRemainsByteIdentical(t *testing.T) {
	resources := []spec.Resource{
		mustParseCompatibilityResource(t, `schema_version: 1
name: Product
table: products
route: /products
fields:
  - {name: name, type: string, required: true, max_length: 120, searchable: true, unique: true}
  - {name: status, type: string, required: true, enum: [enabled, disabled], filterable: true}
  - {name: owner_id, type: int64, filterable: true, sortable: true}
  - {name: price, type: decimal, min: 0}
  - {name: metadata, type: json, nullable: true}
`),
		mustParseCompatibilityResource(t, `schema_version: 1
name: Defaults
table: defaults
route: /defaults
fields:
  - {name: label, type: string, default: fallback}
  - {name: active, type: bool, default: true}
  - {name: count, type: int32, min: -2, max: 9, default: 2}
  - {name: ratio, type: float64, min: -1.5, max: 3.5, default: 1.5}
  - {name: starts_at, type: time, default: "2026-01-02T03:04:05Z"}
  - {name: external_id, type: uuid, default: "550e8400-e29b-41d4-a716-446655440000"}
  - {name: note, type: text, nullable: true, default: null}
`),
	}

	digest := sha256.New()
	for _, httpChoice := range []HTTPChoice{HTTPEcho, HTTPFiber} {
		for _, database := range []DatabaseChoice{DatabaseSQLite, DatabasePostgres} {
			for _, cache := range []CacheChoice{CacheNone, CacheRedis} {
				for _, messaging := range []MessagingChoice{MessagingNone, MessagingNATS} {
					for _, logging := range []LoggingChoice{LoggingSlog, LoggingZap, LoggingZerolog} {
						for _, auth := range []AuthChoice{AuthNone, AuthJWT} {
							for _, profile := range []ProfileChoice{ProfilePersonal, ProfileProduction} {
								opts := ProjectOptions{
									HTTP: httpChoice, Database: database, Cache: cache,
									Messaging: messaging, Logging: logging, Auth: auth, Profile: profile,
								}
								files, err := renderGenerated("example.com/compatibility", resources, opts)
								if err != nil {
									t.Fatalf("renderGenerated(%+v): %v", opts, err)
								}
								if _, err := fmt.Fprintf(digest, "selection=%s/%s/%s/%s/%s/%s/%s\n", httpChoice, database, cache, messaging, logging, auth, profile); err != nil {
									t.Fatal(err)
								}
								for _, name := range slices.Sorted(maps.Keys(files)) {
									if _, err := fmt.Fprintf(digest, "file=%s bytes=%d\n", name, len(files[name])); err != nil {
										t.Fatal(err)
									}
									_, _ = digest.Write(files[name])
								}
							}
						}
					}
				}
			}
		}
	}

	got := hex.EncodeToString(digest.Sum(nil))
	if got != legacyGeneratedOutputSHA256 {
		t.Fatalf("legacy none/JWT generated output digest = %s, want %s", got, legacyGeneratedOutputSHA256)
	}
}

func mustParseCompatibilityResource(t *testing.T, source string) spec.Resource {
	t.Helper()
	resource, err := spec.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	return resource
}
