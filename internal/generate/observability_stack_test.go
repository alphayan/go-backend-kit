package generate

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Text assertions cannot prove the observability stack boots. Start only the
// generated Grafana service on a fresh named volume, then check its health,
// the provisioned Prometheus datasource, and the provisioned dashboard.
func TestProductionGrafanaBootsWithGeneratedProvisioning(t *testing.T) {
	if os.Getenv("GOBACKEND_DOCKER_E2E") != "1" {
		t.Skip("set GOBACKEND_DOCKER_E2E=1 for isolated Docker checks")
	}
	kit, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "api")
	opts := DefaultProjectOptions()
	opts.Profile = ProfileProduction
	if err := (Generator{DevelopmentReplace: kit}).New(t.Context(), root, "example.com/grafana-check", opts); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "test-compose.yml"), []byte("services:\n  grafana:\n    ports: !override [\"127.0.0.1::3000\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Compose validates every service, so the api env_file must exist even for --no-deps.
	if err := os.WriteFile(filepath.Join(root, ".env"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	password := "disposable-grafana-" + strings.ReplaceAll(time.Now().Format("150405.000000000"), ".", "")
	project := "kit-grafana-" + strings.ReplaceAll(time.Now().Format("20060102150405.000000000"), ".", "")
	env := []string{}
	for _, entry := range os.Environ() {
		if key, _, _ := strings.Cut(entry, "="); !strings.HasPrefix(key, "COMPOSE_") && key != "GRAFANA_ADMIN_PASSWORD" {
			env = append(env, entry)
		}
	}
	env = append(env, "COMPOSE_PROJECT_NAME="+project, "COMPOSE_FILE="+filepath.Join(root, "docker-compose.yml")+":"+filepath.Join(root, "test-compose.yml"), "GRAFANA_ADMIN_PASSWORD="+password)
	compose := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "docker", append([]string{"compose"}, args...)...)
		cmd.Dir, cmd.Env = root, env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("docker compose %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	t.Cleanup(func() {
		cmd := exec.Command("docker", "compose", "down", "--volumes", "--remove-orphans")
		cmd.Dir, cmd.Env = root, env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("remove grafana test stack: %v: %s", err, out)
		}
	})
	compose("up", "-d", "--no-deps", "grafana")
	port := strings.TrimPrefix(compose("port", "grafana", "3000"), "127.0.0.1:")
	client := &http.Client{Timeout: 5 * time.Second}
	get := func(path string, auth bool) (int, []byte) {
		t.Helper()
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://127.0.0.1:"+port+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if auth {
			request.SetBasicAuth("admin", password)
		}
		response, err := client.Do(request)
		if err != nil {
			return 0, nil
		}
		defer func() { _ = response.Body.Close() }()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return 0, nil
		}
		return response.StatusCode, body
	}
	deadline := time.Now().Add(90 * time.Second)
	for {
		var health struct {
			Database string `json:"database"`
		}
		var datasources []struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		}
		var dashboards []struct {
			Title string `json:"title"`
		}
		healthStatus, healthBody := get("/api/health", false)
		sourceStatus, sourceBody := get("/api/datasources", true)
		dashboardStatus, dashboardBody := get("/api/search?type=dash-db", true)
		// Database health can precede asynchronous datasource/dashboard provisioning.
		if healthStatus == http.StatusOK && json.Unmarshal(healthBody, &health) == nil && health.Database == "ok" &&
			sourceStatus == http.StatusOK && json.Unmarshal(sourceBody, &datasources) == nil && len(datasources) == 1 && datasources[0].Type == "prometheus" && datasources[0].URL == "http://prometheus:9090" &&
			dashboardStatus == http.StatusOK && json.Unmarshal(dashboardBody, &dashboards) == nil && len(dashboards) > 0 {
			break
		}
		if time.Now().After(deadline) {
			cmd := exec.CommandContext(t.Context(), "docker", "compose", "logs", "--no-log-prefix", "grafana")
			cmd.Dir, cmd.Env = root, env
			logs, err := cmd.CombinedOutput()
			t.Fatalf("grafana not ready: health=%d %s, datasources=%d %s, dashboards=%d %s; logs error=%v\n%s", healthStatus, healthBody, sourceStatus, sourceBody, dashboardStatus, dashboardBody, err, logs)
		}
		time.Sleep(time.Second)
	}
	if status, _ := get("/api/datasources", false); status != http.StatusUnauthorized {
		t.Fatalf("anonymous datasource access = %d", status)
	}
}
