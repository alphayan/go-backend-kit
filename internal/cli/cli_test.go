package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphayan/go-backend-kit/internal/cli"
	"github.com/alphayan/go-backend-kit/internal/generate"
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

func TestUpgradeCommandDefaultsToPreview(t *testing.T) {
	kit, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOBACKEND_DEVELOPMENT_REPLACE", kit)
	target := filepath.Join(t.TempDir(), "api")
	if err := (generate.Generator{Version: "v0.1.0", DevelopmentReplace: kit}).New(t.Context(), target, "example.com/cli-upgrade", generate.DefaultProjectOptions()); err != nil {
		t.Fatal(err)
	}
	t.Chdir(target)
	before, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	info := cli.BuildInfo{Version: "v0.2.0"}
	if err := cli.Execute(t.Context(), info, &output, &bytes.Buffer{}, []string{"upgrade"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Preview only") || !strings.Contains(output.String(), "Review/recovery directory:") {
		t.Fatalf("preview output = %s", &output)
	}
	if after, _ := os.ReadFile("go.mod"); !bytes.Equal(before, after) {
		t.Fatal("default upgrade wrote source")
	}
	output.Reset()
	if err := cli.Execute(t.Context(), info, &output, &bytes.Buffer{}, []string{"upgrade", "--apply"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Upgrade applied") {
		t.Fatalf("apply output = %s", &output)
	}
	if err := cli.Execute(t.Context(), info, &bytes.Buffer{}, &bytes.Buffer{}, []string{"upgrade", "unexpected"}); err == nil {
		t.Fatal("upgrade accepted positional arguments")
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
		"--profile", "personal",
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

func TestNewCommandRejectsInvalidProfileFlag(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "api")
	err := cli.Execute(t.Context(), cli.BuildInfo{Version: "v0.2.0"}, &bytes.Buffer{}, &bytes.Buffer{}, []string{
		"new", target, "--module", "example.com/api", "--profile", "enterprise",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid profile value") ||
		!strings.Contains(err.Error(), "personal, production") {
		t.Fatalf("error = %v, want invalid profile with allowed values", err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("invalid profile created target directory: %v", statErr)
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
		{"--auth", "oauth", `invalid auth value "oauth"`, "none, jwt, session"},
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
		{"--auth", []string{"none", "jwt", "session"}},
		{"--profile", []string{"personal", "production"}},
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

func TestAuthFlagCompletionMatchesGeneratorChoices(t *testing.T) {
	var stdout bytes.Buffer
	root := cli.New(cli.BuildInfo{Version: "v0.2.0"}, &stdout, &bytes.Buffer{})
	root.SetArgs([]string{"__complete", "new", "--auth", ""})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, choice := range generate.AuthChoices() {
		if !strings.Contains(stdout.String(), choice+"\n") {
			t.Errorf("auth completion missing %q:\n%s", choice, stdout.String())
		}
	}
}

func TestNewCommandRejectsFiberSessionBeforeCreatingTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "api")
	err := cli.Execute(t.Context(), cli.BuildInfo{Version: "v0.2.0"}, &bytes.Buffer{}, &bytes.Buffer{}, []string{
		"new", target, "--module", "example.com/api", "--http", "fiber", "--auth", "session",
	})
	want := `auth "session" requires --http echo; Fiber session authentication is not supported in v1`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("invalid combination created target directory: %v", statErr)
	}
}

func TestProductionDatabaseDefaultAndExplicitSQLite(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOBACKEND_DEVELOPMENT_REPLACE", root)
	for _, explicit := range []bool{false, true} {
		target := filepath.Join(t.TempDir(), "api")
		args := []string{"new", target, "--module", "example.com/api", "--profile", "production"}
		want := `"database": "postgres"`
		if explicit {
			args = append(args, "--database", "sqlite")
			want = `"database": "sqlite"`
		}
		if err := cli.Execute(t.Context(), cli.BuildInfo{}, &bytes.Buffer{}, &bytes.Buffer{}, args); err != nil {
			t.Fatal(err)
		}
		body, err := os.ReadFile(filepath.Join(target, ".gobackend-project.json"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), want) {
			t.Fatalf("database selection missing %s", want)
		}
	}
}
