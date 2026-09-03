package generate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

type HTTPChoice string
type DatabaseChoice string
type CacheChoice string
type MessagingChoice string
type LoggingChoice string
type AuthChoice string
type ProfileChoice string

const (
	HTTPEcho  HTTPChoice = "echo"
	HTTPFiber HTTPChoice = "fiber"

	DatabaseSQLite   DatabaseChoice = "sqlite"
	DatabasePostgres DatabaseChoice = "postgres"

	CacheNone  CacheChoice = "none"
	CacheRedis CacheChoice = "redis"

	MessagingNone MessagingChoice = "none"
	MessagingNATS MessagingChoice = "nats"

	LoggingSlog    LoggingChoice = "slog"
	LoggingZap     LoggingChoice = "zap"
	LoggingZerolog LoggingChoice = "zerolog"

	AuthNone    AuthChoice = "none"
	AuthJWT     AuthChoice = "jwt"
	AuthSession AuthChoice = "session"

	ProfilePersonal   ProfileChoice = "personal"
	ProfileProduction ProfileChoice = "production"
)

var (
	httpChoices      = []string{string(HTTPEcho), string(HTTPFiber)}
	databaseChoices  = []string{string(DatabaseSQLite), string(DatabasePostgres)}
	cacheChoices     = []string{string(CacheNone), string(CacheRedis)}
	messagingChoices = []string{string(MessagingNone), string(MessagingNATS)}
	loggingChoices   = []string{string(LoggingSlog), string(LoggingZap), string(LoggingZerolog)}
	authChoices      = []string{string(AuthNone), string(AuthJWT), string(AuthSession)}
	profileChoices   = []string{string(ProfilePersonal), string(ProfileProduction)}
)

type ProjectOptions struct {
	HTTP      HTTPChoice      `json:"http"`
	Database  DatabaseChoice  `json:"database"`
	Cache     CacheChoice     `json:"cache"`
	Messaging MessagingChoice `json:"messaging"`
	Logging   LoggingChoice   `json:"logging"`
	Auth      AuthChoice      `json:"auth"`
	Profile   ProfileChoice   `json:"profile"`
}

func DefaultProjectOptions() ProjectOptions {
	return ProjectOptions{
		HTTP:      HTTPEcho,
		Database:  DatabaseSQLite,
		Cache:     CacheNone,
		Messaging: MessagingNone,
		Logging:   LoggingSlog,
		Auth:      AuthNone,
		Profile:   ProfilePersonal,
	}
}

func LegacyProjectOptions() ProjectOptions {
	return ProjectOptions{
		HTTP:      HTTPEcho,
		Database:  DatabasePostgres,
		Cache:     CacheNone,
		Messaging: MessagingNone,
		Logging:   LoggingSlog,
		Auth:      AuthNone,
		Profile:   ProfilePersonal,
	}
}

func (o ProjectOptions) Validate() error {
	o = o.normalized()
	switch o.HTTP {
	case HTTPEcho, HTTPFiber:
	default:
		return invalidChoiceError("http", string(o.HTTP), httpChoices)
	}
	switch o.Database {
	case DatabaseSQLite, DatabasePostgres:
	default:
		return invalidChoiceError("database", string(o.Database), databaseChoices)
	}
	switch o.Cache {
	case CacheNone, CacheRedis:
	default:
		return invalidChoiceError("cache", string(o.Cache), cacheChoices)
	}
	switch o.Messaging {
	case MessagingNone, MessagingNATS:
	default:
		return invalidChoiceError("messaging", string(o.Messaging), messagingChoices)
	}
	switch o.Logging {
	case LoggingSlog, LoggingZap, LoggingZerolog:
	default:
		return invalidChoiceError("logging", string(o.Logging), loggingChoices)
	}
	switch o.Auth {
	case AuthNone, AuthJWT, AuthSession:
	default:
		return invalidChoiceError("auth", string(o.Auth), authChoices)
	}
	switch o.Profile {
	case ProfilePersonal, ProfileProduction:
	default:
		return invalidChoiceError("profile", string(o.Profile), profileChoices)
	}
	if o.Auth == AuthSession && o.HTTP != HTTPEcho {
		return fmt.Errorf("auth %q requires --http echo; Fiber session authentication is not supported in v1", o.Auth)
	}
	return nil
}

func (o ProjectOptions) Fingerprint() (string, error) {
	return o.fingerprint(false)
}

func (o ProjectOptions) legacyFingerprint() (string, error) {
	return o.fingerprint(true)
}

func (o ProjectOptions) fingerprint(legacy bool) (string, error) {
	o = o.normalized()
	if err := o.Validate(); err != nil {
		return "", err
	}
	selection := map[string]string{
		"auth":      string(o.Auth),
		"cache":     string(o.Cache),
		"database":  string(o.Database),
		"http":      string(o.HTTP),
		"logging":   string(o.Logging),
		"messaging": string(o.Messaging),
	}
	if !legacy {
		selection["profile"] = string(o.Profile)
	}
	payload, err := json.Marshal(selection)
	if err != nil {
		return "", fmt.Errorf("encode selection fingerprint: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func (o ProjectOptions) HasRedis() bool   { return o.Cache == CacheRedis }
func (o ProjectOptions) HasNATS() bool    { return o.Messaging == MessagingNATS }
func (o ProjectOptions) HasJWT() bool     { return o.Auth == AuthJWT }
func (o ProjectOptions) HasSession() bool { return o.Auth == AuthSession }
func (o ProjectOptions) IsProduction() bool {
	return o.normalized().Profile == ProfileProduction
}
func (o ProjectOptions) IsSQLite() bool   { return o.Database == DatabaseSQLite }
func (o ProjectOptions) IsPostgres() bool { return o.Database == DatabasePostgres }
func (o ProjectOptions) IsEcho() bool     { return o.HTTP == HTTPEcho }
func (o ProjectOptions) IsFiber() bool    { return o.HTTP == HTTPFiber }
func (o ProjectOptions) IsSlog() bool     { return o.Logging == LoggingSlog }
func (o ProjectOptions) IsZap() bool      { return o.Logging == LoggingZap }
func (o ProjectOptions) IsZerolog() bool  { return o.Logging == LoggingZerolog }

func (o ProjectOptions) normalized() ProjectOptions {
	if o.Profile == "" {
		o.Profile = ProfilePersonal
	}
	return o
}

func invalidChoiceError(name, value string, allowed []string) error {
	return fmt.Errorf("invalid %s value %q (allowed: %s)", name, value, strings.Join(allowed, ", "))
}

// AuthChoices returns a copy of the supported command-line auth values.
func AuthChoices() []string { return append([]string(nil), authChoices...) }
