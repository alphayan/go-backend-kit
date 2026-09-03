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
						for _, auth := range []AuthChoice{AuthNone, AuthJWT, AuthSession} {
							opts.HTTP, opts.Database, opts.Cache = httpChoice, database, cache
							opts.Messaging, opts.Logging, opts.Auth = messaging, logging, auth
							if auth == AuthSession && httpChoice == HTTPFiber {
								if _, err := selectedScaffoldFiles(opts); err == nil {
									t.Fatalf("selectedScaffoldFiles(%+v) accepted Fiber session auth", opts)
								}
								continue
							}
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
							if _, exists := outputs["internal/platform/auth/password.go"]; exists != opts.HasSession() {
								t.Fatalf("session package presence = %v, want %v for %+v", exists, opts.HasSession(), opts)
							}
						}
					}
				}
			}
		}
	}
}

func TestProductionSelectionAddsOptionalPlatformFiles(t *testing.T) {
	opts := DefaultProjectOptions()
	opts.Profile = ProfileProduction
	files, err := selectedScaffoldFiles(opts)
	if err != nil {
		t.Fatal(err)
	}
	outputs := make(map[string]bool, len(files))
	for _, file := range files {
		outputs[file.Output] = true
	}
	for _, name := range []string{
		"Dockerfile",
		".dockerignore",
		"docker-compose.yml",
		"observability/prometheus.yml",
		"observability/alerts.yml",
		"observability/grafana/dashboards/backend.json",
		"internal/platform/observability/observability.go",
		"internal/platform/observability/observability_test.go",
	} {
		if !outputs[name] {
			t.Errorf("production selection missing %s", name)
		}
	}
	personalFiles, err := selectedScaffoldFiles(DefaultProjectOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range personalFiles {
		if file.Output == "Dockerfile" || strings.HasPrefix(file.Output, "observability/") ||
			strings.HasPrefix(file.Output, "internal/platform/observability/") {
			t.Errorf("personal selection emitted production file %s", file.Output)
		}
	}
}
