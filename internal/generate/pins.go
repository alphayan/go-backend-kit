package generate

const (
	pinGoVersion    = "1.26.5"
	pinToolchain    = "go1.26.5"
	pinEcho         = "v5.3.1"
	pinFiber        = "v3.4.0"
	pinGorm         = "v1.31.2"
	pinGormCLI      = "v0.2.4"
	pinGormPostgres = "v1.6.2"
	pinSQLite       = "v1.2.2"
	pinAtlas        = "v1.3.0"
	pinAtlasGorm    = "v0.6.1"
	pinRedis        = "v9.22.0"
	pinNATS         = "v1.53.1"
	pinJose         = "v4.1.4"
	pinZap          = "v1.28.0"
	pinSlogZap      = "v2.7.0"
	pinZerolog      = "v1.35.1"
	pinSlogZerolog  = "v2.9.2"
	pinUUID         = "v1.6.0"
	pinDecimal      = "v1.4.0"
	pinSwgui        = "v1.8.9"
	pinDatatypes    = "v1.2.7"
	pinGRPC         = "v1.82.1"
	pinOTelSDK      = "v1.43.0"
	pinVuln         = "v1.6.0"
	pinPgx          = "v5.9.2"

	pinAtlasImage    = "arigaio/atlas:1.3.0-community"
	pinPostgresImage = "postgres:18.4-alpine3.24"
	pinRedisImage    = "redis:8.10.0-alpine3.23"
	pinNATSImage     = "nats:2.14.5-alpine3.22"
)

type dependencyPins struct {
	GoVersion     string
	Toolchain     string
	Echo          string
	Fiber         string
	Gorm          string
	GormCLI       string
	GormPostgres  string
	SQLite        string
	Atlas         string
	AtlasGorm     string
	Redis         string
	NATS          string
	Jose          string
	Zap           string
	SlogZap       string
	Zerolog       string
	SlogZerolog   string
	UUID          string
	Decimal       string
	Swgui         string
	Datatypes     string
	GRPC          string
	OTelSDK       string
	Vuln          string
	Pgx           string
	AtlasImage    string
	PostgresImage string
	RedisImage    string
	NATSImage     string
}

func currentPins() dependencyPins {
	return dependencyPins{
		GoVersion:     pinGoVersion,
		Toolchain:     pinToolchain,
		Echo:          pinEcho,
		Fiber:         pinFiber,
		Gorm:          pinGorm,
		GormCLI:       pinGormCLI,
		GormPostgres:  pinGormPostgres,
		SQLite:        pinSQLite,
		Atlas:         pinAtlas,
		AtlasGorm:     pinAtlasGorm,
		Redis:         pinRedis,
		NATS:          pinNATS,
		Jose:          pinJose,
		Zap:           pinZap,
		SlogZap:       pinSlogZap,
		Zerolog:       pinZerolog,
		SlogZerolog:   pinSlogZerolog,
		UUID:          pinUUID,
		Decimal:       pinDecimal,
		Swgui:         pinSwgui,
		Datatypes:     pinDatatypes,
		GRPC:          pinGRPC,
		OTelSDK:       pinOTelSDK,
		Vuln:          pinVuln,
		Pgx:           pinPgx,
		AtlasImage:    pinAtlasImage,
		PostgresImage: pinPostgresImage,
		RedisImage:    pinRedisImage,
		NATSImage:     pinNATSImage,
	}
}
