package generate

import (
	"strings"
	"testing"
)

func TestDefaultProjectOptions(t *testing.T) {
	opts := DefaultProjectOptions()
	if err := opts.Validate(); err != nil {
		t.Fatal(err)
	}
	if opts.HTTP != HTTPEcho || opts.Database != DatabaseSQLite || opts.Cache != CacheNone ||
		opts.Messaging != MessagingNone || opts.Logging != LoggingSlog || opts.Auth != AuthNone {
		t.Fatalf("defaults = %+v", opts)
	}
}

func TestLegacyProjectOptions(t *testing.T) {
	opts := LegacyProjectOptions()
	if err := opts.Validate(); err != nil {
		t.Fatal(err)
	}
	if opts.HTTP != HTTPEcho || opts.Database != DatabasePostgres || opts.Cache != CacheNone ||
		opts.Messaging != MessagingNone || opts.Logging != LoggingSlog || opts.Auth != AuthNone {
		t.Fatalf("legacy = %+v", opts)
	}
}

func TestProjectOptionsValidate(t *testing.T) {
	valid := DefaultProjectOptions()
	tests := []struct {
		name    string
		mutate  func(*ProjectOptions)
		want    string
		allowed string
	}{
		{"http", func(o *ProjectOptions) { o.HTTP = "chi" }, `invalid http value "chi"`, "echo, fiber"},
		{"database", func(o *ProjectOptions) { o.Database = "mysql" }, `invalid database value "mysql"`, "sqlite, postgres"},
		{"cache", func(o *ProjectOptions) { o.Cache = "memcached" }, `invalid cache value "memcached"`, "none, redis"},
		{"messaging", func(o *ProjectOptions) { o.Messaging = "kafka" }, `invalid messaging value "kafka"`, "none, nats"},
		{"logging", func(o *ProjectOptions) { o.Logging = "logrus" }, `invalid logging value "logrus"`, "slog, zap, zerolog"},
		{"auth", func(o *ProjectOptions) { o.Auth = "oauth" }, `invalid auth value "oauth"`, "none, jwt"},
		{"empty http", func(o *ProjectOptions) { o.HTTP = "" }, `invalid http value ""`, "echo, fiber"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts := valid
			test.mutate(&opts)
			err := opts.Validate()
			if err == nil {
				t.Fatal("Validate() error = nil")
			}
			if !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), test.allowed) {
				t.Fatalf("Validate() error = %q, want %q and allowed %q", err, test.want, test.allowed)
			}
		})
	}
}

func TestProjectOptionsValidateAcceptsEveryChoice(t *testing.T) {
	opts := DefaultProjectOptions()
	for _, httpChoice := range []HTTPChoice{HTTPEcho, HTTPFiber} {
		for _, database := range []DatabaseChoice{DatabaseSQLite, DatabasePostgres} {
			for _, cache := range []CacheChoice{CacheNone, CacheRedis} {
				for _, messaging := range []MessagingChoice{MessagingNone, MessagingNATS} {
					for _, logging := range []LoggingChoice{LoggingSlog, LoggingZap, LoggingZerolog} {
						for _, auth := range []AuthChoice{AuthNone, AuthJWT} {
							opts.HTTP, opts.Database, opts.Cache = httpChoice, database, cache
							opts.Messaging, opts.Logging, opts.Auth = messaging, logging, auth
							if err := opts.Validate(); err != nil {
								t.Fatalf("Validate(%+v) = %v", opts, err)
							}
						}
					}
				}
			}
		}
	}
}

func TestSelectionFingerprintIsCanonical(t *testing.T) {
	left := ProjectOptions{HTTP: HTTPFiber, Database: DatabasePostgres, Cache: CacheRedis, Messaging: MessagingNATS, Logging: LoggingZap, Auth: AuthJWT}
	right := left
	got, err := left.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	want, err := right.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if got != want || len(got) != 64 {
		t.Fatalf("fingerprint = %q, want stable 64-character hex", got)
	}
	left.Cache = CacheNone
	changed, err := left.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if changed == got {
		t.Fatal("fingerprint ignored cache selection")
	}
}
