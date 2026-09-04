package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphayan/go-backend-kit/internal/generate"
)

func TestReleaseVersionUsesCurrentDevelopmentBaseline(t *testing.T) {
	for _, input := range []string{"", "devel", "(devel)"} {
		if got := releaseVersion(input); got != generate.CurrentVersion {
			t.Errorf("releaseVersion(%q) = %q, want %s", input, got, generate.CurrentVersion)
		}
	}
	if got := releaseVersion("v1.2.3"); got != "v1.2.3" {
		t.Errorf("releaseVersion(v1.2.3) = %q, want v1.2.3", got)
	}
}

func TestInstalledModuleVersionAndSourceBuildBoundary(t *testing.T) {
	t.Setenv("GOBACKEND_DEVELOPMENT_REPLACE", "")
	const pseudo = "v0.1.1-0.20260903231737-edfcd6f70bb0"
	for _, version := range []string{"", "devel", "(devel)"} {
		if got := buildVersion(version, pseudo, false); got != pseudo {
			t.Fatalf("installed version = %q", got)
		}
	}
	if got := buildVersion("v1.2.3", pseudo, false); got != "v1.2.3" {
		t.Fatalf("release override = %q", got)
	}
	// go build in a modified checkout stamps +dirty and vcs.modified; that
	// version names the last pushed commit, not the code that was compiled.
	for _, source := range []struct {
		module   string
		modified bool
	}{{pseudo + "+dirty", false}, {pseudo, true}, {"(devel)", false}} {
		if got := buildVersion("devel", source.module, source.modified); got != "devel" {
			t.Fatalf("modified checkout %q adopted as %q", source.module, got)
		}
	}
	if _, err := scaffoldGenerator(BuildInfo{Version: "devel"}); err == nil || !strings.Contains(err.Error(), "GOBACKEND_DEVELOPMENT_REPLACE") {
		t.Fatalf("modified checkout generator = %v", err)
	}
	root := filepath.Join(t.TempDir(), "api")
	err := Execute(t.Context(), BuildInfo{Version: "devel"}, &bytes.Buffer{}, &bytes.Buffer{}, []string{"new", root, "--module", "example.com/api"})
	if err == nil || !strings.Contains(err.Error(), "GOBACKEND_DEVELOPMENT_REPLACE") {
		t.Fatalf("source build error = %v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("failed source build created target: %v", err)
	}
	g, err := scaffoldGenerator(BuildInfo{Version: pseudo})
	if err != nil || g.Version != pseudo {
		t.Fatalf("installed generator pin = %q, %v", g.Version, err)
	}
}
