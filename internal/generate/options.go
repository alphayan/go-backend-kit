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

	AuthNone AuthChoice = "none"
	AuthJWT  AuthChoice = "jwt"
)

var (
	httpChoices      = []string{string(HTTPEcho), string(HTTPFiber)}
	databaseChoices  = []string{string(DatabaseSQLite), string(DatabasePostgres)}
	cacheChoices     = []string{string(CacheNone), string(CacheRedis)}
	messagingChoices = []string{string(MessagingNone), string(MessagingNATS)}
	loggingChoices   = []string{string(LoggingSlog), string(LoggingZap), string(LoggingZerolog)}
	authChoices      = []string{string(AuthNone), string(AuthJWT)}
)

type ProjectOptions struct {
	HTTP      HTTPChoice      `json:"http"`
	Database  DatabaseChoice  `json:"database"`
	Cache     CacheChoice     `json:"cache"`
	Messaging MessagingChoice `json:"messaging"`
	Logging   LoggingChoice   `json:"logging"`
	Auth      AuthChoice      `json:"auth"`
}

func DefaultProjectOptions() ProjectOptions {
	return ProjectOptions{
		HTTP:      HTTPEcho,
		Database:  DatabaseSQLite,
		Cache:     CacheNone,
		Messaging: MessagingNone,
		Logging:   LoggingSlog,
		Auth:      AuthNone,
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
	}
}

func (o ProjectOptions) Validate() error {
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
	case AuthNone, AuthJWT:
	default:
		return invalidChoiceError("auth", string(o.Auth), authChoices)
	}
	return nil
}

func (o ProjectOptions) Fingerprint() (string, error) {
	if err := o.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(map[string]string{
		"auth":      string(o.Auth),
		"cache":     string(o.Cache),
		"database":  string(o.Database),
		"http":      string(o.HTTP),
		"logging":   string(o.Logging),
		"messaging": string(o.Messaging),
	})
	if err != nil {
		return "", fmt.Errorf("encode selection fingerprint: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func (o ProjectOptions) HasRedis() bool   { return o.Cache == CacheRedis }
func (o ProjectOptions) HasNATS() bool    { return o.Messaging == MessagingNATS }
func (o ProjectOptions) HasJWT() bool     { return o.Auth == AuthJWT }
func (o ProjectOptions) IsSQLite() bool   { return o.Database == DatabaseSQLite }
func (o ProjectOptions) IsPostgres() bool { return o.Database == DatabasePostgres }
func (o ProjectOptions) IsEcho() bool     { return o.HTTP == HTTPEcho }
func (o ProjectOptions) IsFiber() bool    { return o.HTTP == HTTPFiber }
func (o ProjectOptions) IsSlog() bool     { return o.Logging == LoggingSlog }
func (o ProjectOptions) IsZap() bool      { return o.Logging == LoggingZap }
func (o ProjectOptions) IsZerolog() bool  { return o.Logging == LoggingZerolog }

func invalidChoiceError(name, value string, allowed []string) error {
	return fmt.Errorf("invalid %s value %q (allowed: %s)", name, value, strings.Join(allowed, ", "))
}
