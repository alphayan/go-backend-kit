package generate

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This gate never accepts an existing database URL: it owns one disposable container.
func TestProductionPostgresSecurity(t *testing.T) {
	if os.Getenv("GOBACKEND_POSTGRES_SECURITY_E2E") != "1" {
		t.Skip("set GOBACKEND_POSTGRES_SECURITY_E2E=1 to use an isolated Docker database")
	}
	kit, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "api")
	opts := DefaultProjectOptions()
	opts.Database, opts.Profile, opts.Auth = DatabasePostgres, ProfileProduction, AuthSession
	g := Generator{DevelopmentReplace: kit}
	if err := g.New(t.Context(), root, "example.com/production-security", opts); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// The runtime password deliberately exercises SQL and URI quoting, not just hex.
	adminPassword := "disposable-admin-password-" + t.Name()
	migrationPassword := "disposable-migration-password-" + t.Name()
	runtimePassword := "disposable-runtime @:$' password-" + t.Name()
	write(".env.postgres", "POSTGRES_PASSWORD="+adminPassword+"\nPOSTGRES_MIGRATOR_PASSWORD="+migrationPassword+"\nPOSTGRES_RUNTIME_PASSWORD="+runtimePassword+"\n")
	dsn := func(user, password, host string) string {
		return (&url.URL{Scheme: "postgres", User: url.UserPassword(user, password), Host: host, Path: "/app", RawQuery: "sslmode=disable"}).String()
	}
	write(".env", "DATABASE_URL="+dsn("app_runtime", runtimePassword, "postgres:5432")+"\n")
	write("test-compose.yml", "services:\n  postgres:\n    ports: !override [\"127.0.0.1::5432\"]\n")
	composeProject := "kit-production-" + time.Now().Format("20060102150405.000000000")
	composeProject = strings.ReplaceAll(composeProject, ".", "")
	baseEnv := []string{}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if key == "DATABASE_URL" || key == "ATLAS_DATABASE_URL" || key == "TEST_DATABASE_URL" || strings.HasPrefix(key, "TEST_RATE_") || key == "RATE_TEST_CHILD" || strings.HasPrefix(key, "COMPOSE_") {
			continue
		}
		baseEnv = append(baseEnv, entry)
	}
	baseEnv = append(baseEnv, "COMPOSE_PROJECT_NAME="+composeProject, "COMPOSE_FILE="+filepath.Join(root, "docker-compose.yml")+":"+filepath.Join(root, "test-compose.yml"))
	command := func(env []string, program string, args ...string) ([]byte, error) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), program, args...)
		cmd.Dir = root
		cmd.Env = append(append([]string{}, baseEnv...), env...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		output := append(append([]byte{}, stdout.Bytes()...), stderr.Bytes()...)
		if program != "docker" || len(args) < 2 || args[0] != "compose" || args[1] != "config" {
			for _, secret := range []string{adminPassword, migrationPassword, runtimePassword} {
				if strings.Contains(string(output), secret) {
					t.Fatal("command exposed a database password")
				}
			}
		}
		if err != nil {
			return output, err
		}
		return stdout.Bytes(), nil
	}
	ok := func(env []string, program string, args ...string) string {
		t.Helper()
		out, err := command(env, program, args...)
		if err != nil {
			t.Fatalf("%s failed: %v\n%s", program, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	reject := func(env []string, program string, args ...string) {
		t.Helper()
		if out, err := command(env, program, args...); err == nil {
			t.Fatalf("%s unexpectedly accepted forbidden operation: %s", program, out)
		}
	}
	var compose struct {
		Services map[string]struct{ Environment map[string]string }
	}
	if err := json.Unmarshal([]byte(ok(nil, "docker", "compose", "config", "--format", "json")), &compose); err != nil {
		t.Fatal(err)
	}
	for key, value := range compose.Services["api"].Environment {
		if strings.HasPrefix(key, "POSTGRES_") || strings.Contains(value, adminPassword) || strings.Contains(value, migrationPassword) {
			t.Fatal("API received privileged database credentials")
		}
	}
	if strings.ReplaceAll(compose.Services["api"].Environment["DATABASE_URL"], "$$", "$") != dsn("app_runtime", runtimePassword, "postgres:5432") {
		t.Fatal("API runtime DSN was replaced")
	}
	reject(nil, "sh", "scripts/atlas.sh", "migrate", "apply", "--env", "ci")
	reject([]string{"ATLAS_DATABASE_URL=" + dsn("app_migrator", migrationPassword, "localhost:not-a-port")}, "sh", "scripts/atlas.sh", "migrate", "apply", "--env", "ci")
	t.Cleanup(func() {
		cmd := exec.Command("docker", "compose", "down", "--volumes", "--remove-orphans")
		cmd.Dir, cmd.Env = root, baseEnv
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("remove isolated test stack: %v: %s", err, output)
		}
	})
	ok(nil, "docker", "compose", "up", "-d", "postgres")
	id := ok(nil, "docker", "compose", "ps", "-q", "postgres")
	deadline := time.Now().Add(45 * time.Second)
	for {
		// TCP readiness avoids the image's temporary socket-only bootstrap server.
		if _, err := command(nil, "docker", "exec", id, "pg_isready", "-h", "127.0.0.1", "-U", "postgres", "-d", "app"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("isolated PostgreSQL did not become ready")
		}
		time.Sleep(100 * time.Millisecond)
	}
	port := strings.TrimPrefix(ok(nil, "docker", "port", id, "5432/tcp"), "127.0.0.1:")
	runtimeURL, migrationURL, adminURL := dsn("app_runtime", runtimePassword, "localhost:"+port), dsn("app_migrator", migrationPassword, "localhost:"+port), dsn("postgres", adminPassword, "localhost:"+port)
	migrationEnv := []string{"ATLAS_DATABASE_URL=" + migrationURL}
	if err := g.Add(t.Context(), root, filepath.Join(kit, "examples/product.yaml")); err != nil {
		t.Fatal(err)
	}
	ok(nil, "sh", "scripts/atlas.sh", "migrate", "diff", "products", "--env", "local")
	ok(migrationEnv, "sh", "scripts/atlas.sh", "migrate", "apply", "--env", "ci")
	if drift := ok(migrationEnv, "sh", "scripts/atlas.sh", "schema", "diff", "--env", "ci", "--from", "env://url", "--to", "env://src", "--exclude", "atlas_schema_revisions", "--format", "{{ sql . }}"); drift != "" {
		t.Fatalf("schema drift: %s", drift)
	}
	write("cmd/dbprobe/main.go", `package main
import("context";"fmt";"os";"time";"example.com/production-security/internal/platform/database";"example.com/production-security/internal/platform/auth")
func check(err error){if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}}
func main(){ ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second); defer cancel()
c,err:=database.Open(ctx,os.Getenv("DATABASE_URL"),database.PoolOptions{MaxOpenConnections:2,MaxIdleConnections:1})
check(err)
if os.Getenv("PROBE_AUTH")=="1"{
passwords,err:=auth.NewPasswordManager(auth.DefaultPasswordParams(),1);check(err)
s:=auth.NewStore(c.ORM);_,_,err=s.Bootstrap(ctx,"runtime-control@example.test","disposable user password",passwords);check(err)
u,err:=s.FindUserByEmail(ctx,"runtime-control@example.test");check(err)
token,err:=s.CreateSession(ctx,u.ID,"","probe","127.0.0.1",time.Hour,time.Now().UTC());check(err)
identity,err:=s.Authenticate(ctx,token,time.Now().UTC(),time.Hour);check(err);if identity.Role!="admin"{os.Exit(1)}
check(s.DeleteSession(ctx,token))
};check(c.Close())}
`)
	probe := filepath.Join(root, "dbprobe")
	ok(nil, "go", "build", "-o", probe, "./cmd/dbprobe")
	ok([]string{"DATABASE_URL=" + runtimeURL}, probe)
	ok([]string{"DATABASE_URL=" + runtimeURL, "PROBE_AUTH=1"}, probe)
	rateEnv := []string{"TEST_RATE_DATABASE_URL=" + runtimeURL, "TEST_RATE_DISPOSABLE=1"}
	// Mutation control: the two-process check must catch a process-local fallback.
	// Only this newly generated disposable project's file is changed, then restored.
	limiterPath := "internal/platform/auth/ratelimit.go"
	limiterSource, err := os.ReadFile(filepath.Join(root, limiterPath))
	if err != nil {
		t.Fatal(err)
	}
	localOnly := strings.Replace(string(limiterSource), "return l.allowShared(ctx, ip, email)", "return l.ip.Take(ip, now) && (email == \"\" || l.email.Take(email, now)), nil", 1)
	if localOnly == string(limiterSource) {
		t.Fatal("shared limiter mutation target missing")
	}
	write(limiterPath, localOnly)
	mutationOutput, mutationErr := command(rateEnv, "go", "test", "-race", "./internal/platform/auth", "-run", "^TestPostgresSharedLimiter$", "-count=1")
	write(limiterPath, string(limiterSource))
	if mutationErr == nil || !strings.Contains(string(mutationOutput), "two processes admitted 4 ip attempts, want 2") {
		t.Fatalf("shared limiter negative control did not detect local fallback: %v\n%s", mutationErr, mutationOutput)
	}
	ok(rateEnv, "go", "test", "-race", "./internal/platform/auth", "-run", "^TestPostgresSharedLimiter$", "-count=1")
	reject([]string{"DATABASE_URL=" + migrationURL}, probe)
	reject([]string{"DATABASE_URL=" + adminURL + "&options=-c%20role%3Dapp_runtime"}, probe)
	sql := func(user, password, query string, allowed bool) string {
		t.Helper()
		args := []string{"exec", "-e", "PGPASSWORD", id, "psql", "-X", "-h", "127.0.0.1", "-U", user, "-d", "app", "-v", "ON_ERROR_STOP=1", "-Atc", query}
		if !allowed {
			reject([]string{"PGPASSWORD=" + password}, "docker", args...)
			return ""
		}
		return ok([]string{"PGPASSWORD=" + password}, "docker", args...)
	}
	sql("app_runtime", runtimePassword, `INSERT INTO products(name,status,owner_id,price,"order",created_at,updated_at) VALUES('runtime-control','enabled',1,2,'first',now(),now()); UPDATE products SET price=3 WHERE name='runtime-control'; SELECT * FROM products; DELETE FROM products WHERE name='runtime-control';`, true)
	for _, query := range []string{
		"CREATE DATABASE forbidden", "CREATE ROLE forbidden", "CREATE SCHEMA forbidden", "CREATE TABLE public.forbidden(id int)", "CREATE TEMP TABLE forbidden(id int)",
		"ALTER TABLE public.products ADD COLUMN forbidden int", "DROP TABLE public.products", "SET ROLE app_migrator",
		"UPDATE atlas_schema_revisions.atlas_schema_revisions SET description='forbidden'", "TRUNCATE atlas_schema_revisions.atlas_schema_revisions",
	} {
		sql("app_runtime", runtimePassword, query, false)
	}
	sql("postgres", adminPassword, "GRANT app_migrator TO app_runtime WITH INHERIT FALSE, SET TRUE", true)
	reject([]string{"DATABASE_URL=" + runtimeURL}, probe)
	sql("postgres", adminPassword, "REVOKE app_migrator FROM app_runtime", true)
	sql("postgres", adminPassword, "GRANT USAGE ON SCHEMA atlas_schema_revisions TO app_runtime; GRANT UPDATE(description) ON atlas_schema_revisions.atlas_schema_revisions TO app_runtime", true)
	reject([]string{"DATABASE_URL=" + runtimeURL}, probe)
	sql("postgres", adminPassword, "REVOKE UPDATE(description) ON atlas_schema_revisions.atlas_schema_revisions FROM app_runtime; REVOKE USAGE ON SCHEMA atlas_schema_revisions FROM app_runtime", true)
	ok([]string{"DATABASE_URL=" + runtimeURL}, probe)
	// Default ACLs must also work for tables introduced by later migrations.
	if err := g.Add(t.Context(), root, filepath.Join(kit, "examples/defaults.yaml")); err != nil {
		t.Fatal(err)
	}
	ok(nil, "sh", "scripts/atlas.sh", "migrate", "diff", "defaults", "--env", "local")
	ok(migrationEnv, "sh", "scripts/atlas.sh", "migrate", "apply", "--env", "ci")
	sql("app_runtime", runtimePassword, "INSERT INTO defaults(created_at,updated_at) VALUES(now(),now()); SELECT * FROM defaults", true)
	// Re-running the empty-volume hook must fail, not rewrite existing roles/data.
	reject(nil, "docker", "exec", id, "sh", "/docker-entrypoint-initdb.d/10-app.sh")
	ok([]string{"DATABASE_URL=" + runtimeURL}, probe)
	for _, object := range []string{"SCHEMA public", "DATABASE app"} {
		privileges := "CREATE"
		if strings.HasPrefix(object, "DATABASE") {
			privileges = "CREATE,TEMP"
		}
		sql("postgres", adminPassword, "ALTER "+object+" OWNER TO app_runtime; REVOKE "+privileges+" ON "+object+" FROM app_runtime", true)
		if _, err := command([]string{"DATABASE_URL=" + runtimeURL}, probe); err == nil {
			t.Errorf("runtime accepted %s ownership with revoked ACL", object)
		}
		sql("postgres", adminPassword, "ALTER "+object+" OWNER TO app_migrator; REVOKE "+privileges+" ON "+object+" FROM app_runtime", true)
	}
	sql("postgres", adminPassword, "GRANT CONNECT ON DATABASE app TO app_runtime; GRANT USAGE ON SCHEMA public TO app_runtime", true)
	// Never print server logs: failed password-bearing SQL may contain test secrets.
	ok(nil, "docker", "logs", id)
	backup := filepath.Join(root, "test-backup.dump")
	ok(nil, "sh", "scripts/postgres-backup.sh", backup)
	reject(nil, "sh", "scripts/postgres-backup.sh", backup)
	if info, err := os.Stat(backup); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("backup not mode 600")
	}
	// Independent cluster: roles are recreated by the hook, not inherited from source.
	restoreEnv := []string{"COMPOSE_PROJECT_NAME=" + composeProject + "-restore"}
	t.Cleanup(func() {
		cmd := exec.Command("docker", "compose", "down", "--volumes", "--remove-orphans")
		cmd.Dir, cmd.Env = root, append(append([]string{}, baseEnv...), restoreEnv...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("remove restore test stack: %v: %s", err, output)
		}
	})
	ok(restoreEnv, "docker", "compose", "up", "-d", "postgres")
	restoreID := ok(restoreEnv, "docker", "compose", "ps", "-q", "postgres")
	deadline = time.Now().Add(45 * time.Second)
	for {
		if _, err := command(nil, "docker", "exec", restoreID, "pg_isready", "-h", "127.0.0.1", "-U", "postgres", "-d", "app"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("restore PostgreSQL did not become ready")
		}
		time.Sleep(100 * time.Millisecond)
	}
	restorePort := strings.TrimPrefix(ok(nil, "docker", "port", restoreID, "5432/tcp"), "127.0.0.1:")
	ok(restoreEnv, "sh", "scripts/postgres-restore.sh", backup, "app_restore_test")
	reject(restoreEnv, "sh", "scripts/postgres-restore.sh", backup, "app_restore_test")
	restoredURL := strings.Replace(dsn("app_runtime", runtimePassword, "localhost:"+restorePort), "/app?", "/app_restore_test?", 1)
	ok([]string{"DATABASE_URL=" + restoredURL}, probe)
	ok([]string{"DATABASE_URL=" + restoredURL, "PROBE_AUTH=1"}, probe)
	out := ok([]string{"PGPASSWORD=" + runtimePassword}, "docker", "exec", "-e", "PGPASSWORD", restoreID, "psql", "-X", "-h", "127.0.0.1", "-U", "app_runtime", "-d", "app_restore_test", "-Atc", "SELECT count(*) FROM defaults")
	if out != "1" {
		t.Fatalf("restored defaults count = %s", out)
	}
	out = ok([]string{"PGPASSWORD=" + runtimePassword}, "docker", "exec", "-e", "PGPASSWORD", restoreID, "psql", "-X", "-h", "127.0.0.1", "-U", "app_runtime", "-d", "app_restore_test", "-Atc", "INSERT INTO defaults(nickname,created_at,updated_at) VALUES('restore-sequence',now(),now()) RETURNING id")
	if !strings.HasPrefix(out, "2\n") {
		t.Fatalf("restored sequence = %s", out)
	}
	reject([]string{"PGPASSWORD=" + runtimePassword}, "docker", "exec", "-e", "PGPASSWORD", restoreID, "psql", "-X", "-h", "127.0.0.1", "-U", "app_runtime", "-d", "app_restore_test", "-Atc", "UPDATE atlas_schema_revisions.atlas_schema_revisions SET description='forbidden'")
	restoredMigrationURL := strings.Replace(dsn("app_migrator", migrationPassword, "localhost:"+restorePort), "/app?", "/app_restore_test?", 1)
	status := ok([]string{"ATLAS_DATABASE_URL=" + restoredMigrationURL}, "sh", "scripts/atlas.sh", "migrate", "status", "--env", "ci")
	if !strings.Contains(status, "Already at latest version") {
		t.Fatalf("restored migration status: %s", status)
	}
	write("corrupt.dump", "not a PostgreSQL archive")
	reject(restoreEnv, "sh", "scripts/postgres-restore.sh", filepath.Join(root, "corrupt.dump"), "app_restore_corrupt")
	if count := ok(nil, "docker", "exec", restoreID, "psql", "-X", "-U", "postgres", "-d", "app_restore_corrupt", "-Atc", "SELECT count(*) FROM pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema')"); count != "0" {
		t.Fatalf("failed restore left user tables: %s", count)
	}
	ok([]string{"DATABASE_URL=" + runtimeURL}, probe)
	ok(nil, "go", "test", "-race", "./...")
	ok(nil, "go", "vet", "./...")
	ok(nil, "go", "tool", "govulncheck", "./...")
	ok([]string{"GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0"}, "go", "build", "-mod=readonly", "-o", filepath.Join(root, "api-linux-amd64"), "./cmd/api")
	t.Log("production roles, migrations, DML/DDL/owner/SET ROLE/ledger checks, secret-safe failure logs, Session lifecycle, two-process limiter with negative control, independent-cluster restore, race/vet/vuln and Linux build passed")
}

func TestProductionPostgresInitRejectsUnsafePasswords(t *testing.T) {
	script, err := filepath.Abs("scaffold/database/postgres/scripts/postgres-init.sh.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	for _, passwords := range [][3]string{
		{"", "long-migration-password", "long-runtime-password"},
		{"long-administrator-password", "", "long-runtime-password"},
		{"long-administrator-password", "long-migration-password", "short"},
		{"same-password-more-than-twenty", "same-password-more-than-twenty", "long-runtime-password"},
		{"long-administrator-password", "same-password-more-than-twenty", "same-password-more-than-twenty"},
	} {
		cmd := exec.Command("sh", script)
		cmd.Env = []string{"PATH=/usr/bin:/bin", "POSTGRES_PASSWORD=" + passwords[0], "POSTGRES_MIGRATOR_PASSWORD=" + passwords[1], "POSTGRES_RUNTIME_PASSWORD=" + passwords[2]}
		out, err := cmd.CombinedOutput()
		if err == nil || (!strings.Contains(string(out), "Set POSTGRES") && !strings.Contains(string(out), "must be")) {
			t.Fatalf("unsafe credentials reached psql: %v: %s", err, out)
		}
	}
}
