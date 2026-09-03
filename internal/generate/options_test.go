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
		opts.Messaging != MessagingNone || opts.Logging != LoggingSlog || opts.Auth != AuthNone ||
		opts.Profile != ProfilePersonal {
		t.Fatalf("defaults = %+v", opts)
	}
}

func TestLegacyProjectOptions(t *testing.T) {
	opts := LegacyProjectOptions()
	if err := opts.Validate(); err != nil {
		t.Fatal(err)
	}
	if opts.HTTP != HTTPEcho || opts.Database != DatabasePostgres || opts.Cache != CacheNone ||
		opts.Messaging != MessagingNone || opts.Logging != LoggingSlog || opts.Auth != AuthNone ||
		opts.Profile != ProfilePersonal {
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
		{"auth", func(o *ProjectOptions) { o.Auth = "oauth" }, `invalid auth value "oauth"`, "none, jwt, session"},
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

func TestProjectOptionsValidateRejectsUnknownProfile(t *testing.T) {
	opts := DefaultProjectOptions()
	opts.Profile = "enterprise"
	err := opts.Validate()
	if err == nil || !strings.Contains(err.Error(), "invalid profile value") ||
		!strings.Contains(err.Error(), "personal, production") {
		t.Fatalf("Validate() error = %v, want invalid profile with allowed values", err)
	}
}

func TestProjectOptionsValidateAcceptsEveryChoice(t *testing.T) {
	opts := DefaultProjectOptions()
	for _, httpChoice := range []HTTPChoice{HTTPEcho, HTTPFiber} {
		for _, database := range []DatabaseChoice{DatabaseSQLite, DatabasePostgres} {
			for _, cache := range []CacheChoice{CacheNone, CacheRedis} {
				for _, messaging := range []MessagingChoice{MessagingNone, MessagingNATS} {
					for _, logging := range []LoggingChoice{LoggingSlog, LoggingZap, LoggingZerolog} {
						for _, auth := range []AuthChoice{AuthNone, AuthJWT, AuthSession} {
							opts.HTTP, opts.Database, opts.Cache = httpChoice, database, cache
							opts.Messaging, opts.Logging, opts.Auth = messaging, logging, auth
							if auth == AuthSession && httpChoice == HTTPFiber {
								continue
							}
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

func TestProjectOptionsValidateRejectsFiberSession(t *testing.T) {
	opts := DefaultProjectOptions()
	opts.HTTP = HTTPFiber
	opts.Auth = AuthSession
	want := `auth "session" requires --http echo; Fiber session authentication is not supported in v1`
	if err := opts.Validate(); err == nil || err.Error() != want {
		t.Fatalf("Validate() error = %v, want %q", err, want)
	}
}

func TestAuthChoicesReturnsCopy(t *testing.T) {
	choices := AuthChoices()
	if got := strings.Join(choices, ","); got != "none,jwt,session" {
		t.Fatalf("AuthChoices() = %q", got)
	}
	choices[0] = "changed"
	if got := strings.Join(AuthChoices(), ","); got != "none,jwt,session" {
		t.Fatalf("AuthChoices() was mutated: %q", got)
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
	left = right
	left.Profile = ProfileProduction
	production, err := left.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if production == got {
		t.Fatal("fingerprint ignored profile selection")
	}
}

func TestExistingSelectionFingerprintsRemainStable(t *testing.T) {
	tests := []struct {
		name string
		opts ProjectOptions
		want string
	}{
		{"default", DefaultProjectOptions(), "ed5a7536eca6df4c3bca962930100ff302b36c90f2d6619102b6080bb62b0054"},
		{"echo postgres jwt", ProjectOptions{HTTP: HTTPEcho, Database: DatabasePostgres, Cache: CacheRedis, Messaging: MessagingNATS, Logging: LoggingZap, Auth: AuthJWT, Profile: ProfilePersonal}, "724c10146e4c3d2f87dcff9c797bd67728407a707c763b101a2ed9f83d37df28"},
		{"fiber postgres none", ProjectOptions{HTTP: HTTPFiber, Database: DatabasePostgres, Cache: CacheNone, Messaging: MessagingNone, Logging: LoggingSlog, Auth: AuthNone, Profile: ProfilePersonal}, "d9f26bd54fafc9c0d19f7f693b688250878384f369a32eef5f960b6be60b3c64"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.opts.Fingerprint()
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("Fingerprint() = %s, want %s", got, test.want)
			}
		})
	}
}
