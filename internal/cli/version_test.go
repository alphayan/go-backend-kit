package cli

import (
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
