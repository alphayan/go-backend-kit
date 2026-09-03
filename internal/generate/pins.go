package generate

const (
	pinGoVersion    = "1.27.1"
	pinToolchain    = "go1.27.1"
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
	pinCrypto       = "v0.56.0"
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
	pinPrometheus   = "v1.24.1"
	pinVuln         = "v1.6.0"
	pinPgx          = "v5.9.2"
	pinNode         = "24.20.0"
	pinPNPM         = "11.25.0"
	pinVue          = "3.5.42"
	pinVueRouter    = "5.3.1"
	pinVueQuery     = "5.102.8"
	pinZod          = "4.5.4"
	pinVite         = "8.2.2"
	pinViteVue      = "6.0.8"
	pinTypeScript   = "5.9.3"
	pinVueTSC       = "3.3.11"
	pinVueCompiler  = "3.5.42"
	pinVitest       = "4.1.11"
	pinNodeTypes    = "24.13.3"

	pinAtlasImage      = "arigaio/atlas:1.3.0-community"
	pinPostgresImage   = "postgres:18.6-alpine3.24"
	pinRedisImage      = "redis:8.10.0-alpine3.23"
	pinNATSImage       = "nats:2.14.5-alpine3.22"
	pinPrometheusImage = "prom/prometheus:v3.5.0"
	pinGrafanaImage    = "grafana/grafana:12.1.0"
)

type dependencyPins struct {
	GoVersion       string
	Toolchain       string
	Echo            string
	Fiber           string
	Gorm            string
	GormCLI         string
	GormPostgres    string
	SQLite          string
	Atlas           string
	AtlasGorm       string
	Redis           string
	NATS            string
	Jose            string
	Crypto          string
	Zap             string
	SlogZap         string
	Zerolog         string
	SlogZerolog     string
	UUID            string
	Decimal         string
	Swgui           string
	Datatypes       string
	GRPC            string
	OTelSDK         string
	Prometheus      string
	Vuln            string
	Pgx             string
	Node            string
	PNPM            string
	Vue             string
	VueRouter       string
	VueQuery        string
	Zod             string
	Vite            string
	ViteVue         string
	TypeScript      string
	VueTSC          string
	VueCompiler     string
	Vitest          string
	NodeTypes       string
	AtlasImage      string
	PostgresImage   string
	RedisImage      string
	NATSImage       string
	PrometheusImage string
	GrafanaImage    string
}

func currentPins() dependencyPins {
	return dependencyPins{
		GoVersion:       pinGoVersion,
		Toolchain:       pinToolchain,
		Echo:            pinEcho,
		Fiber:           pinFiber,
		Gorm:            pinGorm,
		GormCLI:         pinGormCLI,
		GormPostgres:    pinGormPostgres,
		SQLite:          pinSQLite,
		Atlas:           pinAtlas,
		AtlasGorm:       pinAtlasGorm,
		Redis:           pinRedis,
		NATS:            pinNATS,
		Jose:            pinJose,
		Crypto:          pinCrypto,
		Zap:             pinZap,
		SlogZap:         pinSlogZap,
		Zerolog:         pinZerolog,
		SlogZerolog:     pinSlogZerolog,
		UUID:            pinUUID,
		Decimal:         pinDecimal,
		Swgui:           pinSwgui,
		Datatypes:       pinDatatypes,
		GRPC:            pinGRPC,
		OTelSDK:         pinOTelSDK,
		Prometheus:      pinPrometheus,
		Vuln:            pinVuln,
		Pgx:             pinPgx,
		Node:            pinNode,
		PNPM:            pinPNPM,
		Vue:             pinVue,
		VueRouter:       pinVueRouter,
		VueQuery:        pinVueQuery,
		Zod:             pinZod,
		Vite:            pinVite,
		ViteVue:         pinViteVue,
		TypeScript:      pinTypeScript,
		VueTSC:          pinVueTSC,
		VueCompiler:     pinVueCompiler,
		Vitest:          pinVitest,
		NodeTypes:       pinNodeTypes,
		AtlasImage:      pinAtlasImage,
		PostgresImage:   pinPostgresImage,
		RedisImage:      pinRedisImage,
		NATSImage:       pinNATSImage,
		PrometheusImage: pinPrometheusImage,
		GrafanaImage:    pinGrafanaImage,
	}
}
