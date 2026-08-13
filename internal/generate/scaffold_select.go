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
	if opts.IsPostgres() || opts.HasRedis() || opts.HasNATS() {
		files = append(files, mapScaffold("common/", "common/docker-compose.yml.tmpl")...)
	}
	return files
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
		return mapScaffold("database/postgres/",
			"database/postgres/internal/platform/database/database.go.tmpl",
			"database/postgres/internal/platform/database/database_test.go.tmpl",
			"database/postgres/internal/platform/config/config_test.go.tmpl",
			"database/postgres/scripts/atlas.sh.tmpl",
		)
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
