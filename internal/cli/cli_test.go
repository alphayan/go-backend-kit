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

func TestNewCommandDefaultsAndValidFlags(t *testing.T) {
	target := filepath.Join(t.TempDir(), "api")
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOBACKEND_DEVELOPMENT_REPLACE", root)
	err = cli.Execute(t.Context(), cli.BuildInfo{Version: "v0.2.0"}, &bytes.Buffer{}, &bytes.Buffer{}, []string{
		"new", target, "--module", "example.com/api",
		"--http", "echo",
		"--database", "sqlite",
		"--cache", "none",
		"--messaging", "none",
		"--logging", "slog",
		"--auth", "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, ".gobackend-project.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"http": "echo"`, `"database": "sqlite"`, `"cache": "none"`, `"auth": "none"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("project metadata missing %s:\n%s", want, data)
		}
	}
}

func TestNewCommandRejectsInvalidProviderFlags(t *testing.T) {
	tests := []struct {
		flag    string
		value   string
		want    string
		allowed string
	}{
		{"--http", "chi", `invalid http value "chi"`, "echo, fiber"},
		{"--database", "mysql", `invalid database value "mysql"`, "sqlite, postgres"},
		{"--cache", "memcached", `invalid cache value "memcached"`, "none, redis"},
		{"--messaging", "kafka", `invalid messaging value "kafka"`, "none, nats"},
		{"--logging", "logrus", `invalid logging value "logrus"`, "slog, zap, zerolog"},
		{"--auth", "oauth", `invalid auth value "oauth"`, "none, jwt"},
	}
	for _, test := range tests {
		t.Run(test.flag+" "+test.value, func(t *testing.T) {
			parent := t.TempDir()
			target := filepath.Join(parent, "api")
			err := cli.Execute(t.Context(), cli.BuildInfo{Version: "v0.2.0"}, &bytes.Buffer{}, &bytes.Buffer{}, []string{
				"new", target, "--module", "example.com/api", test.flag, test.value,
			})
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), test.allowed) {
				t.Fatalf("error = %v, want %q and allowed %q", err, test.want, test.allowed)
			}
			if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
				t.Fatalf("invalid flags created target directory: %v", statErr)
			}
		})
	}
}

func TestNewCommandFlagCompletion(t *testing.T) {
	tests := []struct {
		flag string
		want []string
	}{
		{"--http", []string{"echo", "fiber"}},
		{"--database", []string{"sqlite", "postgres"}},
		{"--cache", []string{"none", "redis"}},
		{"--messaging", []string{"none", "nats"}},
		{"--logging", []string{"slog", "zap", "zerolog"}},
		{"--auth", []string{"none", "jwt"}},
	}
	for _, test := range tests {
		t.Run(test.flag, func(t *testing.T) {
			var stdout bytes.Buffer
			root := cli.New(cli.BuildInfo{Version: "v0.2.0"}, &stdout, &bytes.Buffer{})
			root.SetArgs([]string{"__complete", "new", test.flag, ""})
			if err := root.ExecuteContext(t.Context()); err != nil {
				t.Fatal(err)
			}
			output := stdout.String()
			for _, value := range test.want {
				if !strings.Contains(output, value) {
					t.Errorf("completion output missing %q:\n%s", value, output)
				}
			}
			if !strings.Contains(output, "ShellCompDirectiveNoFileComp") && !strings.Contains(output, ":4") {
				t.Errorf("completion did not disable file completion:\n%s", output)
			}
		})
	}
}
