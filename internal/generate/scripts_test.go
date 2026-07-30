package generate_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphayan/go-backend-kit/internal/generate"
)

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

func fakeScriptTools(t *testing.T, goScript string) (string, string) {
	t.Helper()
	root := t.TempDir()
	log := filepath.Join(root, "docker.log")
	dockerScript := `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_DOCKER_LOG"
case "$1" in
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
	container := ""
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] != "run" || fields[1] != "-d" {
			continue
		}
		for i := 2; i+1 < len(fields); i++ {
			if fields[i] == "--name" {
				container = fields[i+1]
				break
			}
		}
		if container != "" {
			break
		}
	}
	if container == "" {
		t.Fatalf("docker log has no named detached container:\n%s", data)
	}
	want := "rm -fv " + container
	for _, line := range lines {
		if line == want {
			return
		}
	}
	t.Fatalf("docker cleanup did not run %q:\n%s", want, data)
}
