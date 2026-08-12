package generate_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alphayan/go-backend-kit/internal/generate"
)

const fakeContainerID = "fake-container-id"

func TestTemporaryPostgresCleanupRemovesAnonymousVolumes(t *testing.T) {
	t.Run("root PostgreSQL E2E failure", func(t *testing.T) {
		kitRoot, err := filepath.Abs(filepath.Join("..", ".."))
		if err != nil {
			t.Fatal(err)
		}
		tools, log := fakeScriptTools(t, "#!/bin/sh\nexit 23\n")
		command := exec.Command("sh", filepath.Join(kitRoot, "scripts", "postgres-e2e.sh"))
		command.Dir = kitRoot
		command.Env = scriptTestEnv(tools, log)
		if output, err := command.CombinedOutput(); err == nil {
			t.Fatalf("postgres-e2e.sh unexpectedly succeeded:\n%s", output)
		}
		assertExactContainerCleanup(t, log)
	})

	t.Run("generated Atlas success", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "api")
		kitRoot, err := filepath.Abs(filepath.Join("..", ".."))
		if err != nil {
			t.Fatal(err)
		}
		generator := generate.Generator{Version: "v0.1.0", DevelopmentReplace: kitRoot}
		if err := generator.New(t.Context(), root, "example.com/api"); err != nil {
			t.Fatal(err)
		}
		tools, log := fakeScriptTools(t, "#!/bin/sh\nprintf '%s\\n' 'CREATE TABLE example (id bigint);'\n")
		command := exec.Command("sh", filepath.Join(root, "scripts", "atlas.sh"), "migrate", "status")
		command.Dir = root
		command.Env = scriptTestEnv(tools, log)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("atlas.sh failed: %v\n%s", err, output)
		}
		assertExactContainerCleanup(t, log)
	})
}

func TestTemporaryPostgresCleanupRequiresSuccessfulCreation(t *testing.T) {
	t.Run("root Docker creation failure", func(t *testing.T) {
		kitRoot, err := filepath.Abs(filepath.Join("..", ".."))
		if err != nil {
			t.Fatal(err)
		}
		tools, log := fakeScriptTools(t, "#!/bin/sh\nexit 0\n")
		command := exec.Command("sh", filepath.Join(kitRoot, "scripts", "postgres-e2e.sh"))
		command.Dir = kitRoot
		command.Env = append(scriptTestEnv(tools, log), "FAKE_DOCKER_RUN_FAIL=1")
		if output, err := command.CombinedOutput(); err == nil {
			t.Fatalf("postgres-e2e.sh unexpectedly succeeded:\n%s", output)
		}
		assertNoContainerCleanup(t, log)
	})

	t.Run("generated Atlas pre-creation failure", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "api")
		kitRoot, err := filepath.Abs(filepath.Join("..", ".."))
		if err != nil {
			t.Fatal(err)
		}
		generator := generate.Generator{Version: "v0.1.0", DevelopmentReplace: kitRoot}
		if err := generator.New(t.Context(), root, "example.com/api"); err != nil {
			t.Fatal(err)
		}
		tools, log := fakeScriptTools(t, "#!/bin/sh\nexit 23\n")
		command := exec.Command("sh", filepath.Join(root, "scripts", "atlas.sh"), "migrate", "status")
		command.Dir = root
		command.Env = scriptTestEnv(tools, log)
		if output, err := command.CombinedOutput(); err == nil {
			t.Fatalf("atlas.sh unexpectedly succeeded:\n%s", output)
		}
		assertNoContainerCleanup(t, log)
	})

	t.Run("generated Atlas Docker creation failure", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "api")
		kitRoot, err := filepath.Abs(filepath.Join("..", ".."))
		if err != nil {
			t.Fatal(err)
		}
		generator := generate.Generator{Version: "v0.1.0", DevelopmentReplace: kitRoot}
		if err := generator.New(t.Context(), root, "example.com/api"); err != nil {
			t.Fatal(err)
		}
		tools, log := fakeScriptTools(t, "#!/bin/sh\nexit 0\n")
		command := exec.Command("sh", filepath.Join(root, "scripts", "atlas.sh"), "migrate", "status")
		command.Dir = root
		command.Env = append(scriptTestEnv(tools, log), "FAKE_DOCKER_RUN_FAIL=1")
		if output, err := command.CombinedOutput(); err == nil {
			t.Fatalf("atlas.sh unexpectedly succeeded:\n%s", output)
		}
		assertNoContainerCleanup(t, log)
	})
}

func fakeScriptTools(t *testing.T, goScript string) (string, string) {
	t.Helper()
	root := t.TempDir()
	log := filepath.Join(root, "docker.log")
	dockerScript := `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_DOCKER_LOG"
case "$1" in
  run)
    printf '%s\n' '` + fakeContainerID + `'
    if [ "${FAKE_DOCKER_RUN_FAIL:-}" = 1 ]; then
      exit 23
    fi
    ;;
  port)
    printf '%s\n' '0.0.0.0:55432'
    ;;
esac
`
	for name, content := range map[string]string{
		"docker": dockerScript,
		"go":     goScript,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root, log
}

func scriptTestEnv(tools, log string) []string {
	env := make([]string, 0, len(os.Environ())+4)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "PATH=") ||
			strings.HasPrefix(value, "DATABASE_URL=") ||
			strings.HasPrefix(value, "ATLAS_DATABASE_URL=") ||
			strings.HasPrefix(value, "FAKE_DOCKER_RUN_FAIL=") ||
			strings.HasPrefix(value, "FAKE_DOCKER_LOG=") {
			continue
		}
		env = append(env, value)
	}
	return append(env,
		"PATH="+tools+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DATABASE_URL=",
		"ATLAS_DATABASE_URL=",
		"FAKE_DOCKER_LOG="+log,
	)
}

func assertExactContainerCleanup(t *testing.T, log string) {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := "rm -fv " + fakeContainerID
	if slices.Contains(lines, want) {
		return
	}
	t.Fatalf("docker cleanup did not run %q:\n%s", want, data)
}

func assertNoContainerCleanup(t *testing.T, log string) {
	t.Helper()
	data, err := os.ReadFile(log)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		if strings.HasPrefix(line, "rm ") {
			t.Fatalf("container cleanup ran without successful creation:\n%s", data)
		}
	}
}
