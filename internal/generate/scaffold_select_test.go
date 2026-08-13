package generate

import (
	"strings"
	"testing"
)

func TestDetectScaffoldCollisions(t *testing.T) {
	err := detectScaffoldCollisions([]scaffoldFile{
		{Source: "http/echo/internal/app/app.go.tmpl", Output: "internal/app/app.go"},
		{Source: "http/fiber/internal/app/app.go.tmpl", Output: "internal/app/app.go"},
	})
	if err == nil || !strings.Contains(err.Error(), "internal/app/app.go") {
		t.Fatalf("error = %v, want collision on internal/app/app.go", err)
	}
}

func TestSelectedScaffoldFilesCoverCombinationsWithoutCollision(t *testing.T) {
	opts := DefaultProjectOptions()
	for _, httpChoice := range []HTTPChoice{HTTPEcho, HTTPFiber} {
		for _, database := range []DatabaseChoice{DatabaseSQLite, DatabasePostgres} {
			for _, cache := range []CacheChoice{CacheNone, CacheRedis} {
				for _, messaging := range []MessagingChoice{MessagingNone, MessagingNATS} {
					for _, logging := range []LoggingChoice{LoggingSlog, LoggingZap, LoggingZerolog} {
						for _, auth := range []AuthChoice{AuthNone, AuthJWT} {
							opts.HTTP, opts.Database, opts.Cache = httpChoice, database, cache
							opts.Messaging, opts.Logging, opts.Auth = messaging, logging, auth
							files, err := selectedScaffoldFiles(opts)
							if err != nil {
								t.Fatalf("selectedScaffoldFiles(%+v) = %v", opts, err)
							}
							outputs := map[string]string{}
							for _, file := range files {
								if previous, exists := outputs[file.Output]; exists {
									t.Fatalf("output %q claimed by %s and %s", file.Output, previous, file.Source)
								}
								outputs[file.Output] = file.Source
							}
							if opts.IsSQLite() && !opts.HasRedis() && !opts.HasNATS() {
								if _, exists := outputs["docker-compose.yml"]; exists {
									t.Fatalf("sqlite-only selection emitted docker-compose.yml: %+v", opts)
								}
							}
							if _, exists := outputs["internal/platform/cache/cache.go"]; exists != opts.HasRedis() {
								t.Fatalf("redis package presence = %v, want %v for %+v", exists, opts.HasRedis(), opts)
							}
							if _, exists := outputs["internal/platform/messaging/messaging.go"]; exists != opts.HasNATS() {
								t.Fatalf("nats package presence = %v, want %v for %+v", exists, opts.HasNATS(), opts)
							}
							if _, exists := outputs["internal/platform/auth/auth.go"]; exists != opts.HasJWT() {
								t.Fatalf("jwt package presence = %v, want %v for %+v", exists, opts.HasJWT(), opts)
							}
						}
					}
				}
			}
		}
	}
}
