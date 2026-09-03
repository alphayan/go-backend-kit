package generate_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphayan/go-backend-kit/internal/generate"
	"golang.org/x/mod/modfile"
)

func TestPairwiseScaffoldMatrix(t *testing.T) {
	productionEcho := generate.DefaultProjectOptions()
	productionEcho.Profile = generate.ProfileProduction
	productionFiber := productionEcho
	productionFiber.HTTP = generate.HTTPFiber
	productionSession := productionEcho
	productionSession.Auth = generate.AuthSession
	productionPostgres := productionSession
	productionPostgres.Database = generate.DatabasePostgres
	cases := []struct {
		name string
		opts generate.ProjectOptions
	}{
		{"echo-sqlite-slog", generate.DefaultProjectOptions()},
		{"echo-postgres-redis-nats-zap-jwt", generate.ProjectOptions{
			HTTP: generate.HTTPEcho, Database: generate.DatabasePostgres, Cache: generate.CacheRedis,
			Messaging: generate.MessagingNATS, Logging: generate.LoggingZap, Auth: generate.AuthJWT,
		}},
		{"fiber-sqlite-redis-zerolog-jwt", generate.ProjectOptions{
			HTTP: generate.HTTPFiber, Database: generate.DatabaseSQLite, Cache: generate.CacheRedis,
			Messaging: generate.MessagingNone, Logging: generate.LoggingZerolog, Auth: generate.AuthJWT,
		}},
		{"fiber-postgres-nats-slog", generate.ProjectOptions{
			HTTP: generate.HTTPFiber, Database: generate.DatabasePostgres, Cache: generate.CacheNone,
			Messaging: generate.MessagingNATS, Logging: generate.LoggingSlog, Auth: generate.AuthNone,
		}},
		{"echo-sqlite-session", generate.ProjectOptions{
			HTTP: generate.HTTPEcho, Database: generate.DatabaseSQLite, Cache: generate.CacheNone,
			Messaging: generate.MessagingNone, Logging: generate.LoggingSlog, Auth: generate.AuthSession,
		}},
		{"echo-postgres-session", generate.ProjectOptions{
			HTTP: generate.HTTPEcho, Database: generate.DatabasePostgres, Cache: generate.CacheNone,
			Messaging: generate.MessagingNone, Logging: generate.LoggingSlog, Auth: generate.AuthSession,
		}},
		{"echo-production", productionEcho},
		{"echo-sqlite-session-production", productionSession},
		{"echo-postgres-session-production", productionPostgres},
		{"fiber-production", productionFiber},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "api")
			g := testGenerator(t)
			if err := g.New(t.Context(), root, "example.com/"+test.name, test.opts); err != nil {
				t.Fatalf("New() error = %v", err)
			}
			source := filepath.Join(t.TempDir(), "task.yaml")
			if err := os.WriteFile(source, []byte(taskYAML), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := g.Add(t.Context(), root, source); err != nil {
				t.Fatalf("Add() error = %v", err)
			}
			if err := g.Check(t.Context(), root); err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			assertSelectedDependencies(t, root, test.opts)
			assertNoForbiddenAbstractions(t, root)
			command := exec.Command("go", "test", "./...")
			command.Dir = root
			if test.opts.IsSQLite() {
				command.Env = append(os.Environ(), "CGO_ENABLED=0")
			}
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("go test ./...: %v\n%s", err, output)
			}
			if test.name == "echo-sqlite-session" {
				// Smoke only: catch broken or missing benchmarks, never assert timing in CI.
				command := exec.Command("go", "test", "./internal/platform/auth", "-run=^$", "-bench=^BenchmarkPassword$", "-benchtime=1x", "-cpu=1,2", "-benchmem")
				command.Dir = root
				output, err := command.CombinedOutput()
				if err != nil {
					t.Fatalf("password benchmark smoke: %v\n%s", err, output)
				}
				for _, operation := range []string{"Argon2idHash", "Argon2idVerify", "PBKDF2Verify"} {
					for _, mode := range []string{"serial", "parallel"} {
						if !strings.Contains(string(output), "BenchmarkPassword/"+operation+"/"+mode) {
							t.Errorf("password benchmark smoke missing %s/%s:\n%s", operation, mode, output)
						}
					}
				}
			}
		})
	}
}

func assertSelectedDependencies(t *testing.T, root string, opts generate.ProjectOptions) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	module, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		t.Fatal(err)
	}
	present := map[string]bool{}
	for _, requirement := range module.Require {
		if requirement.Indirect {
			continue
		}
		present[requirement.Mod.Path] = true
	}
	expect := map[string]bool{
		"github.com/labstack/echo/v5":         opts.IsEcho(),
		"github.com/gofiber/fiber/v3":         opts.IsFiber(),
		"gorm.io/driver/postgres":             opts.IsPostgres(),
		"github.com/redis/go-redis/v9":        opts.HasRedis(),
		"github.com/nats-io/nats.go":          opts.HasNATS(),
		"github.com/go-jose/go-jose/v4":       opts.HasJWT(),
		"golang.org/x/crypto":                 opts.HasSession(),
		"go.uber.org/zap":                     opts.IsZap(),
		"github.com/samber/slog-zap/v2":       opts.IsZap(),
		"github.com/rs/zerolog":               opts.IsZerolog(),
		"github.com/samber/slog-zerolog/v2":   opts.IsZerolog(),
		"github.com/prometheus/client_golang": opts.IsProduction(),
	}
	for path, want := range expect {
		if present[path] != want {
			t.Errorf("go.mod %s present=%v, want %v", path, present[path], want)
		}
	}
	compose := filepath.Join(root, "docker-compose.yml")
	_, composeErr := os.Stat(compose)
	wantCompose := opts.IsProduction() || opts.IsPostgres() || opts.HasRedis() || opts.HasNATS()
	if wantCompose && composeErr != nil {
		t.Errorf("docker-compose.yml missing: %v", composeErr)
	}
	if !wantCompose && !os.IsNotExist(composeErr) {
		t.Errorf("docker-compose.yml present for %+v: %v", opts, composeErr)
	}
	if wantCompose {
		composeData, err := os.ReadFile(compose)
		if err != nil {
			t.Fatal(err)
		}
		text := string(composeData)
		if strings.Contains(text, ":latest") || strings.Contains(text, "postgres:18\n") {
			t.Errorf("compose uses a floating image tag:\n%s", text)
		}
		if opts.IsPostgres() != strings.Contains(text, "postgres:") {
			t.Errorf("postgres service presence mismatch in compose")
		}
		if opts.HasRedis() != strings.Contains(text, "redis:") {
			t.Errorf("redis service presence mismatch in compose")
		}
		if opts.HasNATS() != strings.Contains(text, "nats:") {
			t.Errorf("nats service presence mismatch in compose")
		}
		if opts.IsProduction() {
			for _, required := range []string{"api:", "prometheus:", "grafana:", "127.0.0.1:8080:8080", "127.0.0.1:9090:9090", "127.0.0.1:3000:3000"} {
				if !strings.Contains(text, required) {
					t.Errorf("production compose missing %q", required)
				}
			}
		}
		if opts.IsPostgres() {
			if strings.Contains(text, "/var/lib/postgresql/data") {
				t.Errorf("postgres volume still mounts /var/lib/postgresql/data:\n%s", text)
			}
			if !strings.Contains(text, "postgres-data:/var/lib/postgresql") {
				t.Errorf("postgres volume does not mount /var/lib/postgresql:\n%s", text)
			}
		}
		if opts.HasNATS() {
			if strings.Contains(text, "nats-server --help") {
				t.Errorf("compose uses nats-server --help as a health check:\n%s", text)
			}
			if !strings.Contains(text, "127.0.0.1:8222/healthz") {
				t.Errorf("compose does not check 127.0.0.1:8222/healthz:\n%s", text)
			}
		}
	}
	if opts.IsProduction() {
		for _, name := range []string{
			"Dockerfile",
			".dockerignore",
			"observability/prometheus.yml",
			"observability/alerts.yml",
			"observability/grafana/dashboards/backend.json",
			"internal/platform/observability/observability.go",
		} {
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
				t.Errorf("production file %s missing: %v", name, err)
			}
		}
	}
	if opts.HasNATS() {
		ci, err := os.ReadFile(filepath.Join(root, ".github/workflows/ci.yml"))
		if err != nil {
			t.Fatal(err)
		}
		ciText := string(ci)
		if strings.Contains(ciText, "nats-server --help") {
			t.Errorf("CI uses nats-server --help as a health check:\n%s", ciText)
		}
		if !strings.Contains(ciText, "127.0.0.1:8222/healthz") {
			t.Errorf("CI NATS service is missing a /healthz health check:\n%s", ciText)
		}
	}
	httpData, err := os.ReadFile(filepath.Join(root, "internal/resources/task/http_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	httpSource := string(httpData)
	if opts.IsFiber() && (!strings.Contains(httpSource, "fiber.Ctx") || strings.Contains(httpSource, "echo.Context")) {
		t.Fatal("fiber project did not regenerate Fiber handlers")
	}
	if opts.IsEcho() && !strings.Contains(httpSource, "*echo.Context") {
		t.Fatal("echo project did not regenerate Echo handlers")
	}
}

func assertNoForbiddenAbstractions(t *testing.T, root string) {
	t.Helper()
	forbidden := []string{
		"plugin.Open",
		"plugin.Lookup",
		"Repository[T]",
		"type Repository ",
		"type Service ",
		"ServiceLocator",
		"WebContext",
		"RequestContext",
		"fx.New",
		"dig.New",
		"wire.Build",
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == "repository" || name == "service" || name == "controller" || name == "provider" {
				t.Errorf("forbidden package directory %s", path)
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source := string(data)
		for _, token := range forbidden {
			if strings.Contains(source, token) {
				t.Errorf("%s contains forbidden abstraction %q", path, token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
