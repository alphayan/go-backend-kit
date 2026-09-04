package generate

import (
	"fmt"
	"path"
	"strings"
)

type scaffoldFile struct {
	Source string
	Output string
}

func selectedScaffoldFiles(opts ProjectOptions) ([]scaffoldFile, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	files := commonScaffoldFiles(opts)
	files = append(files, httpScaffoldFiles(opts)...)
	files = append(files, databaseScaffoldFiles(opts)...)
	files = append(files, loggingScaffoldFiles(opts)...)
	if opts.HasRedis() {
		files = append(files, redisScaffoldFiles()...)
	}
	if opts.HasNATS() {
		files = append(files, natsScaffoldFiles()...)
	}
	if opts.HasJWT() {
		files = append(files, jwtScaffoldFiles()...)
	}
	if opts.HasSession() {
		files = append(files, sessionScaffoldFiles(opts)...)
	}
	if opts.IsProduction() {
		files = append(files, productionScaffoldFiles()...)
	}
	if err := detectScaffoldCollisions(files); err != nil {
		return nil, err
	}
	return files, nil
}

func commonScaffoldFiles(opts ProjectOptions) []scaffoldFile {
	files := mapScaffold("common/",
		"common/LICENSE",
		"common/.gitignore.tmpl",
		"common/Makefile.tmpl",
		"common/atlas.hcl.tmpl",
		"common/README.md.tmpl",
		"common/README.zh-CN.md.tmpl",
		"common/go.mod.tmpl",
		"common/cmd/api/main.go.tmpl",
		"common/.env.example.tmpl",
		"common/.github/workflows/ci.yml.tmpl",
		"common/migrations/.gitkeep",
		"common/internal/platform/apperror/error.go.tmpl",
		"common/internal/platform/optional/field.go.tmpl",
		"common/internal/platform/validation/validation.go.tmpl",
		"common/internal/platform/validation/validation_test.go.tmpl",
		"common/internal/platform/database/logger.go.tmpl",
		"common/internal/platform/config/config.go.tmpl",
		"common/internal/platform/httpx/httpx.go.tmpl",
	)
	if opts.IsProduction() || opts.IsPostgres() || opts.HasRedis() || opts.HasNATS() {
		files = append(files, mapScaffold("common/", "common/docker-compose.yml.tmpl")...)
	}
	return files
}

func productionScaffoldFiles() []scaffoldFile {
	return mapScaffold("common/",
		"common/Dockerfile.tmpl",
		"common/.dockerignore.tmpl",
		"common/observability/prometheus.yml.tmpl",
		"common/observability/alerts.yml.tmpl",
		"common/observability/grafana/provisioning/datasources/datasources.yml.tmpl",
		"common/observability/grafana/provisioning/dashboards/dashboards.yml.tmpl",
		"common/observability/grafana/dashboards/backend.json",
		"common/internal/platform/observability/observability.go.tmpl",
		"common/internal/platform/observability/observability_test.go.tmpl",
	)
}

func httpScaffoldFiles(opts ProjectOptions) []scaffoldFile {
	switch opts.HTTP {
	case HTTPEcho:
		return mapScaffold("http/echo/",
			"http/echo/internal/app/app.go.tmpl",
			"http/echo/internal/app/app_test.go.tmpl",
			"http/echo/internal/platform/httpx/write.go.tmpl",
		)
	case HTTPFiber:
		return mapScaffold("http/fiber/",
			"http/fiber/internal/app/app.go.tmpl",
			"http/fiber/internal/app/app_test.go.tmpl",
			"http/fiber/internal/platform/httpx/write.go.tmpl",
		)
	default:
		return nil
	}
}

func databaseScaffoldFiles(opts ProjectOptions) []scaffoldFile {
	switch opts.Database {
	case DatabaseSQLite:
		return mapScaffold("database/sqlite/",
			"database/sqlite/internal/platform/database/database.go.tmpl",
			"database/sqlite/internal/platform/database/database_test.go.tmpl",
			"database/sqlite/internal/platform/config/config_test.go.tmpl",
			"database/sqlite/scripts/atlas.sh.tmpl",
		)
	case DatabasePostgres:
		files := mapScaffold("database/postgres/",
			"database/postgres/internal/platform/database/database.go.tmpl",
			"database/postgres/internal/platform/database/database_test.go.tmpl",
			"database/postgres/internal/platform/config/config_test.go.tmpl",
			"database/postgres/scripts/atlas.sh.tmpl",
		)
		if opts.IsProduction() {
			files = append(files, mapScaffold("database/postgres/",
				"database/postgres/.env.postgres.example.tmpl",
				"database/postgres/scripts/postgres-init.sh.tmpl",
				"database/postgres/scripts/postgres-backup.sh.tmpl",
				"database/postgres/scripts/postgres-restore.sh.tmpl",
				"database/postgres/docs/postgres-operations.md.tmpl",
			)...)
		}
		return files
	default:
		return nil
	}
}

func loggingScaffoldFiles(opts ProjectOptions) []scaffoldFile {
	switch opts.Logging {
	case LoggingSlog:
		return mapScaffold("logging/slog/",
			"logging/slog/internal/platform/logging/logging.go.tmpl",
			"logging/slog/internal/platform/logging/logging_test.go.tmpl",
		)
	case LoggingZap:
		return mapScaffold("logging/zap/",
			"logging/zap/internal/platform/logging/logging.go.tmpl",
			"logging/zap/internal/platform/logging/logging_test.go.tmpl",
		)
	case LoggingZerolog:
		return mapScaffold("logging/zerolog/",
			"logging/zerolog/internal/platform/logging/logging.go.tmpl",
			"logging/zerolog/internal/platform/logging/logging_test.go.tmpl",
		)
	default:
		return nil
	}
}

func redisScaffoldFiles() []scaffoldFile {
	return mapScaffold("cache/redis/",
		"cache/redis/internal/platform/cache/cache.go.tmpl",
		"cache/redis/internal/platform/cache/cache_test.go.tmpl",
	)
}

func natsScaffoldFiles() []scaffoldFile {
	return mapScaffold("messaging/nats/",
		"messaging/nats/internal/platform/messaging/messaging.go.tmpl",
		"messaging/nats/internal/platform/messaging/messaging_test.go.tmpl",
	)
}

func jwtScaffoldFiles() []scaffoldFile {
	return mapScaffold("auth/jwt/",
		"auth/jwt/internal/platform/auth/auth.go.tmpl",
		"auth/jwt/internal/platform/auth/auth_test.go.tmpl",
	)
}

func sessionScaffoldFiles(opts ProjectOptions) []scaffoldFile {
	files := mapScaffold("auth/session/",
		"auth/session/.node-version.tmpl",
		"auth/session/package.json.tmpl",
		"auth/session/pnpm-lock.yaml.tmpl",
		"auth/session/pnpm-workspace.yaml.tmpl",
		"auth/session/web/index.html.tmpl",
		"auth/session/web/tsconfig.json.tmpl",
		"auth/session/web/vite.config.ts.tmpl",
		"auth/session/web/vitest.config.ts.tmpl",
		"auth/session/web/playwright.config.ts.tmpl",
		"auth/session/web/e2e/admin.spec.ts.tmpl",
		"auth/session/web/dist/.gitkeep",
		"auth/session/web/fallback.html.tmpl",
		"auth/session/web/embed.go.tmpl",
		"auth/session/web/embed_test.go.tmpl",
		"auth/session/web/src/App.vue.tmpl",
		"auth/session/web/src/main.ts.tmpl",
		"auth/session/web/src/router.ts.tmpl",
		"auth/session/web/src/styles.css.tmpl",
		"auth/session/web/src/vite-env.d.ts.tmpl",
		"auth/session/web/src/components/AppShell.vue.tmpl",
		"auth/session/web/src/components/ConfirmDialog.vue.tmpl",
		"auth/session/web/src/components/DataTable.vue.tmpl",
		"auth/session/web/src/components/FieldInput.vue.tmpl",
		"auth/session/web/src/components/FormShell.vue.tmpl",
		"auth/session/web/src/lib/api.ts.tmpl",
		"auth/session/web/src/lib/api.test.ts.tmpl",
		"auth/session/web/src/lib/auth.ts.tmpl",
		"auth/session/web/src/lib/auth.test.ts.tmpl",
		"auth/session/web/src/lib/forms.ts.tmpl",
		"auth/session/web/src/lib/forms.test.ts.tmpl",
		"auth/session/web/src/lib/resource.ts.tmpl",
		"auth/session/web/src/lib/theme.ts.tmpl",
		"auth/session/web/src/views/AccountView.vue.tmpl",
		"auth/session/web/src/views/UsersView.vue.tmpl",
		"auth/session/web/src/views/AuditView.vue.tmpl",
		"auth/session/web/src/views/DashboardView.vue.tmpl",
		"auth/session/web/src/views/LoginView.vue.tmpl",
		"auth/session/web/src/views/NotFoundView.vue.tmpl",
		"auth/session/web/src/views/ResourceView.vue.tmpl",
		"auth/session/internal/platform/auth/models.go.tmpl",
		"auth/session/internal/platform/auth/admin.go.tmpl",
		"auth/session/internal/platform/auth/password.go.tmpl",
		"auth/session/internal/platform/auth/password_test.go.tmpl",
		"auth/session/internal/platform/auth/ratelimit.go.tmpl",
		"auth/session/internal/platform/auth/ratelimit_test.go.tmpl",
		"auth/session/internal/platform/auth/session.go.tmpl",
		"auth/session/internal/platform/auth/session_test.go.tmpl",
		"auth/session/internal/platform/auth/store.go.tmpl",
		"auth/session/internal/platform/auth/store_test.go.tmpl",
		"auth/session/internal/platform/config/session_test.go.tmpl",
		"auth/session/internal/platform/audit/audit.go.tmpl",
		"auth/session/internal/platform/audit/audit_test.go.tmpl",
		"auth/session/internal/platform/rbac/rbac.go.tmpl",
		"auth/session/internal/platform/rbac/rbac_test.go.tmpl",
		"auth/session/internal/app/session_middleware.go.tmpl",
		"auth/session/internal/app/auth_handlers.go.tmpl",
		"auth/session/internal/app/admin_handlers.go.tmpl",
		"auth/session/internal/app/admin_handlers_test.go.tmpl",
		"auth/session/internal/app/auth_handlers_test.go.tmpl",
	)
	if opts.IsPostgres() {
		files = append(files, mapScaffold("auth/session/",
			"auth/session/internal/app/session_postgres_test.go.tmpl",
			"auth/session/internal/platform/auth/ratelimit_postgres.go.tmpl",
			"auth/session/internal/platform/auth/ratelimit_postgres_test.go.tmpl",
		)...)
	}
	return files
}

// Scaffold outputs that older generator versions produced for this selection
// and that no longer exist. Upgrade removes an unmodified copy, reports a
// modified copy as a conflict, and still accepts baselines that record them.
func retiredScaffoldPaths(opts ProjectOptions) map[string]bool {
	retired := map[string]bool{}
	if opts.HasSession() {
		// pnpm 11 ignores its settings in .npmrc; they moved to pnpm-workspace.yaml.
		retired[".npmrc"] = true
	}
	return retired
}

func mapScaffold(prefix string, sources ...string) []scaffoldFile {
	files := make([]scaffoldFile, 0, len(sources))
	for _, source := range sources {
		output := strings.TrimPrefix(source, prefix)
		output = strings.TrimSuffix(output, ".tmpl")
		files = append(files, scaffoldFile{Source: source, Output: path.Clean(output)})
	}
	return files
}

func detectScaffoldCollisions(files []scaffoldFile) error {
	seen := make(map[string]string, len(files))
	for _, file := range files {
		if previous, exists := seen[file.Output]; exists {
			return fmt.Errorf("scaffold output %q is claimed by both %s and %s", file.Output, previous, file.Source)
		}
		seen[file.Output] = file.Source
	}
	return nil
}
