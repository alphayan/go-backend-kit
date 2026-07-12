package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphayan/go-backend-kit/internal/cli"
)

func TestVersion(t *testing.T) {
	var stdout bytes.Buffer
	err := cli.Execute(context.Background(), cli.BuildInfo{Version: "v0.1.0", Commit: "abc123", Date: "2026-07-13"}, &stdout, &bytes.Buffer{}, []string{"version"})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"v0.1.0", "abc123", "2026-07-13"} {
		if !strings.Contains(stdout.String(), value) {
			t.Fatalf("version output %q does not contain %q", stdout.String(), value)
		}
	}
}

func TestNewCommand(t *testing.T) {
	target := filepath.Join(t.TempDir(), "api")
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOBACKEND_DEVELOPMENT_REPLACE", root)
	err = cli.Execute(context.Background(), cli.BuildInfo{Version: "v0.1.0"}, &bytes.Buffer{}, &bytes.Buffer{}, []string{"new", target, "--module", "example.com/api"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "go.mod")); err != nil {
		t.Fatal(err)
	}
}
