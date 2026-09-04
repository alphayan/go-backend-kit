package generate

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// A Docker volume uses Linux ownership even on Docker Desktop. The root control
// reproduces the old failure; the caller-UID control must remain writable.
func TestAtlasSQLiteFileOwnership(t *testing.T) {
	if os.Getenv("GOBACKEND_DOCKER_E2E") != "1" {
		t.Skip("set GOBACKEND_DOCKER_E2E=1 for an isolated Linux volume")
	}
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.CommandContext(t.Context(), "docker", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("docker %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	volume := "kit-atlas-owner-" + time.Now().Format("20060102150405.000000000")
	run("volume", "create", volume)
	t.Cleanup(func() {
		if out, err := exec.Command("docker", "volume", "rm", volume).CombinedOutput(); err != nil {
			t.Errorf("remove test volume: %v: %s", err, out)
		}
	})
	run("run", "--rm", "-v", volume+":/workspace", "--entrypoint", "sh", pinPostgresImage, "-c",
		`mkdir -p /workspace/root/migrations /workspace/caller/migrations; chown -R 1000:1000 /workspace; printf '%s\n' 'CREATE TABLE example (id integer NOT NULL PRIMARY KEY);' > /workspace/schema.sql`)
	for _, mode := range []struct{ name, uid string }{{"root", "0:0"}, {"caller", "1000:1000"}} {
		dir := "/workspace/" + mode.name
		atlas := func(args ...string) {
			t.Helper()
			base := []string{"run", "--rm", "--user", mode.uid, "-e", "HOME=/tmp", "-v", volume + ":/workspace", "-w", "/workspace", pinAtlasImage}
			run(append(base, args...)...)
		}
		atlas("migrate", "diff", "bootstrap", "--dir", "file://"+dir+"/migrations", "--to", "file:///workspace/schema.sql", "--dev-url", "sqlite://"+dir+"/dev.db?mode=rwc")
		atlas("migrate", "apply", "--dir", "file://"+dir+"/migrations", "--url", "sqlite://"+dir+"/app.db?mode=rwc")
		owner := run("run", "--rm", "-v", volume+":/workspace", "--entrypoint", "sh", pinPostgresImage, "-c", `stat -c '%u:%g' "$1/app.db" "$1/migrations/atlas.sum" "$1"/migrations/*.sql`, "sh", dir)
		for line := range strings.SplitSeq(owner, "\n") {
			if line != mode.uid {
				t.Fatalf("%s owner = %q", mode.name, owner)
			}
		}
	}
	run("run", "--rm", "--user", "1000:1000", "-v", volume+":/workspace", "--entrypoint", "sh", pinPostgresImage, "-c",
		`test ! -w /workspace/root/app.db && test -w /workspace/caller/app.db && test -w /workspace/caller/migrations/atlas.sum`)
}
