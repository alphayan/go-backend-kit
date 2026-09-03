package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootPostgresGatesUsePinnedImageAndRunSessionE2E(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, name := range []string{"ci.yml", "release.yml"} {
		path := filepath.Join(root, ".github", "workflows", name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		body := string(data)
		if got := strings.Count(body, "image: "+pinPostgresImage); got != 2 {
			t.Errorf("%s pinned PostgreSQL service count = %d, want 2", name, got)
		}
		if !strings.Contains(body, "run: ./scripts/session-e2e.sh") {
			t.Errorf("%s does not run scripts/session-e2e.sh", name)
		}
		if !strings.Contains(body, "GOBACKEND_POSTGRES_SECURITY_E2E") || !strings.Contains(body, "^TestProductionPostgresSecurity$") {
			t.Errorf("%s does not enforce the production PostgreSQL isolation/restore gate", name)
		}
	}

	for _, name := range []string{"postgres-e2e.sh", "session-e2e.sh"} {
		path := filepath.Join(root, "scripts", name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "-p 127.0.0.1::5432 "+pinPostgresImage) {
			t.Errorf("%s does not use pinned PostgreSQL image %s", name, pinPostgresImage)
		}
	}
}

func TestReadmesInstallCurrentVersion(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, name := range []string{"README.md", "README.zh-CN.md"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		want := "go install github.com/alphayan/go-backend-kit/cmd/gobackend@" + CurrentVersion
		if !strings.Contains(string(data), want) {
			t.Errorf("%s does not install current version %s", name, CurrentVersion)
		}
	}
}

func TestDockerBuildDoesNotDownloadUnpublishedGenerator(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("scaffold", "common", "Dockerfile.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{
		"-droptool=github.com/alphayan/go-backend-kit/cmd/gobackend",
		"-droprequire=github.com/alphayan/go-backend-kit",
		"-dropreplace=github.com/alphayan/go-backend-kit",
		"go build -mod=readonly",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Docker build missing %q", want)
		}
	}
	if strings.Contains(body, "go mod download") {
		t.Fatal("Docker build downloads the development tool graph before excluding the generator")
	}
}

func TestRootFrontendGatesUsePinnedToolchain(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, name := range []string{"ci.yml", "release.yml"} {
		data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", name))
		if err != nil {
			t.Fatal(err)
		}
		body := string(data)
		for _, want := range []string{
			"version: " + pinPNPM,
			"node-version: " + pinNode,
			"run: ./scripts/frontend-e2e.sh",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s does not contain %q", name, want)
			}
		}
	}

	lock, err := os.ReadFile(filepath.Join("scaffold", "auth", "session", "pnpm-lock.yaml.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"specifier: " + pinVue,
		"specifier: " + pinVite,
		"specifier: " + pinTypeScript,
		"specifier: " + pinVitest,
	} {
		if !strings.Contains(string(lock), want) {
			t.Errorf("frontend lockfile does not contain %q", want)
		}
	}
}
