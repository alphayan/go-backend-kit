package generate

import (
	"strconv"
	"strings"
	"testing"

	"github.com/alphayan/go-backend-kit/internal/spec"
	"github.com/shopspring/decimal"
)

func TestFloatSampleSatisfiesExactBound(t *testing.T) {
	tests := map[string]struct {
		constraint string
		valid      func(decimal.Decimal, decimal.Decimal) bool
	}{
		"minimum": {
			constraint: "min: 12.5000000000000001",
			valid:      func(value, bound decimal.Decimal) bool { return !value.LessThan(bound) },
		},
		"maximum": {
			constraint: "max: 12.4999999999999999",
			valid:      func(value, bound decimal.Decimal) bool { return !value.GreaterThan(bound) },
		},
		"subnormal minimum": {
			constraint: "min: 1e-400",
			valid:      func(value, bound decimal.Decimal) bool { return !value.LessThan(bound) },
		},
		"subnormal maximum": {
			constraint: "max: -1e-400",
			valid:      func(value, bound decimal.Decimal) bool { return !value.GreaterThan(bound) },
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			resource, err := spec.Parse([]byte(`schema_version: 1
name: Reading
table: readings
route: /readings
fields:
  - name: value
    type: float64
    ` + test.constraint + "\n"))
			if err != nil {
				t.Fatal(err)
			}
			field := resource.Fields[0]
			sample, err := strconv.ParseFloat(sampleGoValue(field), 64)
			if err != nil {
				t.Fatal(err)
			}
			bound := field.Min
			if bound == nil {
				bound = field.Max
			}
			if !test.valid(decimal.NewFromFloat(sample), bound.Decimal()) {
				t.Fatalf("sample %s does not satisfy %s", sampleGoValue(field), strings.TrimSpace(test.constraint))
			}
		})
	}
}

func TestModelImportsGroupStandardAndThirdParty(t *testing.T) {
	resource := spec.Resource{Fields: []spec.Field{
		{Type: spec.TypeUUID},
		{Type: spec.TypeDecimal},
		{Type: spec.TypeJSON},
	}}
	want := `"time"

"github.com/google/uuid"
"github.com/shopspring/decimal"
"gorm.io/datatypes"
"gorm.io/gorm"`
	if got := modelImports(resource); got != want {
		t.Fatalf("modelImports() = %q, want %q", got, want)
	}
}

func TestModelUsesNewExprForNullableTimeFields(t *testing.T) {
	resource, err := spec.Parse([]byte(`schema_version: 1
name: Event
table: events
route: /events
fields:
  - name: observed_at
    type: time
    nullable: true
`))
	if err != nil {
		t.Fatal(err)
	}

	rendered, err := executeGoTemplate("model_gen.go", modelTemplate, resourceData{
		Module:   "example.com/project",
		Resource: resource,
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "value.ObservedAt = new(value.ObservedAt.UTC())"; !strings.Contains(string(rendered), want) {
		t.Fatalf("generated model does not contain %q:\n%s", want, rendered)
	}
}

func TestModelAddsCheckConstraintForEnum(t *testing.T) {
	resource, err := spec.Parse([]byte(`schema_version: 1
name: Task
table: tasks
route: /tasks
fields:
  - name: state
    type: string
    enum: [new, "owner's review"]
`))
	if err != nil {
		t.Fatal(err)
	}

	rendered, err := executeGoTemplate("model_gen.go", modelTemplate, resourceData{
		Module:   "example.com/project",
		Resource: resource,
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "check:state IN ('new', 'owner''s review')"; !strings.Contains(string(rendered), want) {
		t.Fatalf("generated model does not contain %q:\n%s", want, rendered)
	}
}

func TestStoreUsesConsistentReadSnapshotAndAtomicUpdate(t *testing.T) {
	resource, err := spec.Parse([]byte(`schema_version: 1
name: Task
table: tasks
route: /tasks
fields:
  - name: title
    type: string
    searchable: true
`))
	if err != nil {
		t.Fatal(err)
	}

	postgres, err := executeGoTemplate("store_gen.go", storeTemplate, resourceData{
		Module:   "example.com/project",
		Resource: resource,
		Options:  LegacyProjectOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	source := string(postgres)
	for _, want := range []string{
		`"database/sql"`,
		`&sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}`,
		`item, err := (store{db: tx}).get(ctx, id)`,
		`updated = item`,
		`pattern := searchPattern(filters.query)`,
		`LIKE ? ESCAPE '\\'`,
		`strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_")`,
	} {
		if !strings.Contains(source, want) {
			t.Errorf("generated postgres store does not contain %q:\n%s", want, source)
		}
	}
	if strings.Contains(source, "return s.get(ctx, id)") {
		t.Errorf("generated update reads outside its transaction:\n%s", source)
	}

	sqlite, err := executeGoTemplate("store_gen.go", storeTemplate, resourceData{
		Module:   "example.com/project",
		Resource: resource,
		Options:  DefaultProjectOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqliteSource := string(sqlite)
	if strings.Contains(sqliteSource, `"database/sql"`) || strings.Contains(sqliteSource, "sql.TxOptions") {
		t.Fatalf("sqlite store retained postgres transaction options:\n%s", sqliteSource)
	}
}

func TestRenderGeneratedSelectsHTTPDatabaseAndAuth(t *testing.T) {
	resource, err := spec.Parse([]byte(`schema_version: 1
name: Task
table: tasks
route: /tasks
fields:
  - name: title
    type: string
  - name: external_id
    type: uuid
  - name: metadata
    type: json
`))
	if err != nil {
		t.Fatal(err)
	}

	fiberOpts := DefaultProjectOptions()
	fiberOpts.HTTP = HTTPFiber
	fiberFiles, err := renderGenerated("example.com/project", []spec.Resource{resource}, fiberOpts)
	if err != nil {
		t.Fatal(err)
	}
	fiberHTTP := string(fiberFiles["internal/resources/task/http_gen.go"])
	if !strings.Contains(fiberHTTP, "fiber.Ctx") || strings.Contains(fiberHTTP, "echo.Context") {
		t.Fatalf("fiber HTTP template was not selected:\n%s", fiberHTTP)
	}
	if strings.Contains(string(fiberFiles["internal/generated/register_gen.go"]), "echo.Group") {
		t.Fatal("fiber registrar still imports Echo")
	}

	sqliteFiles, err := renderGenerated("example.com/project", []spec.Resource{resource}, DefaultProjectOptions())
	if err != nil {
		t.Fatal(err)
	}
	model := string(sqliteFiles["internal/resources/task/model_gen.go"])
	if !strings.Contains(model, "column:external_id;type:text;not null") {
		t.Fatalf("sqlite UUID tag missing:\n%s", model)
	}
	if !strings.Contains(model, "column:metadata;type:json;not null") {
		t.Fatalf("sqlite JSON tag missing:\n%s", model)
	}

	jwtOpts := DefaultProjectOptions()
	jwtOpts.Auth = AuthJWT
	jwtFiles, err := renderGenerated("example.com/project", []spec.Resource{resource}, jwtOpts)
	if err != nil {
		t.Fatal(err)
	}
	specJSON := string(jwtFiles["openapi/openapi_gen.json"])
	if !strings.Contains(specJSON, `"bearerAuth"`) || !strings.Contains(specJSON, `"401"`) {
		t.Fatalf("JWT OpenAPI security missing:\n%s", specJSON)
	}
	noneSpec := string(sqliteFiles["openapi/openapi_gen.json"])
	if strings.Contains(noneSpec, "bearerAuth") || strings.Contains(noneSpec, `"401"`) {
		t.Fatalf("auth=none OpenAPI still declares bearer security:\n%s", noneSpec)
	}
}

func TestRenderSessionPermissionsSchemaAndOpenAPI(t *testing.T) {
	resource, err := spec.Parse([]byte(`schema_version: 1
name: Inventory
table: inventory_items
route: /odd-items
fields:
  - name: label
    type: string
`))
	if err != nil {
		t.Fatal(err)
	}
	opts := DefaultProjectOptions()
	opts.Auth = AuthSession
	files, err := renderGenerated("example.com/project", []spec.Resource{resource}, opts)
	if err != nil {
		t.Fatal(err)
	}
	permissions := string(files["internal/generated/permissions_gen.go"])
	for _, want := range []string{
		`"inventory.read": {}`,
		`"inventory.create": {}`,
		`"GET /api/v1/odd-items"`,
		`"PATCH /api/v1/odd-items/:id"`,
	} {
		if !strings.Contains(permissions, want) {
			t.Errorf("permissions output missing %q:\n%s", want, permissions)
		}
	}
	schema := string(files["tools/gormschema/main_gen.go"])
	for _, want := range []string{
		`platformauth "example.com/project/internal/platform/auth"`,
		`&platformauth.User{}`,
		`&platformauth.Session{}`,
		`&platformauth.AuditLog{}`,
	} {
		if !strings.Contains(schema, want) {
			t.Errorf("gorm schema output missing %q:\n%s", want, schema)
		}
	}
	document := string(files["openapi/openapi_gen.json"])
	for _, want := range []string{`"/auth/login"`, `"/auth/password"`, `"/auth/users"`, `"/auth/users/{id}/sessions/{session_id}"`, `"/auth/audit-logs"`, `"AuthUpdateUser"`, `"sessionCookie"`, `"in": "cookie"`, `"403"`, `"429"`} {
		if !strings.Contains(document, want) {
			t.Errorf("session OpenAPI missing %q:\n%s", want, document)
		}
	}

	noneFiles, err := renderGenerated("example.com/project", []spec.Resource{resource}, DefaultProjectOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := noneFiles["internal/generated/permissions_gen.go"]; exists {
		t.Fatal("auth=none rendered session permissions")
	}
	if strings.Contains(string(noneFiles["tools/gormschema/main_gen.go"]), "platformauth") || strings.Contains(string(noneFiles["openapi/openapi_gen.json"]), "/auth/login") {
		t.Fatal("auth=none output contains session artifacts")
	}
}

func TestSessionOpenAPIReservesBuiltInComponentNames(t *testing.T) {
	resource, err := spec.Parse([]byte(`schema_version: 1
name: Login
table: logins
route: /logins
fields: []
`))
	if err != nil {
		t.Fatal(err)
	}
	opts := DefaultProjectOptions()
	opts.Auth = AuthSession
	_, err = buildOpenAPI([]spec.Resource{resource}, opts)
	if err == nil || !strings.Contains(err.Error(), `schema "Login"`) || !strings.Contains(err.Error(), "session login input") {
		t.Fatalf("buildOpenAPI() error = %v", err)
	}
}

func TestSessionGormSchemaAliasesPlatformAuthBesideAuthResource(t *testing.T) {
	resource, err := spec.Parse([]byte(`schema_version: 1
name: Auth
table: authentications
route: /auth-records
fields: []
`))
	if err != nil {
		t.Fatal(err)
	}
	opts := DefaultProjectOptions()
	opts.Auth = AuthSession
	files, err := renderGenerated("example.com/project", []spec.Resource{resource}, opts)
	if err != nil {
		t.Fatal(err)
	}
	schema := string(files["tools/gormschema/main_gen.go"])
	if !strings.Contains(schema, `platformauth "example.com/project/internal/platform/auth"`) || !strings.Contains(schema, `"example.com/project/internal/resources/auth"`) {
		t.Fatalf("schema imports =\n%s", schema)
	}
}
