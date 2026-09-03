package generate

import (
	"strings"
	"testing"

	"github.com/alphayan/go-backend-kit/internal/spec"
)

func TestSessionFrontendGeneratedContract(t *testing.T) {
	resource := mustParseFrontendResource(t, `schema_version: 1
name: InventoryItem
table: inventory_items
route: /stock-items
fields:
  - {name: sku, type: string, required: true, max_length: 40, searchable: true, unique: true}
  - {name: status, type: string, enum: [active, archived], filterable: true}
  - {name: quantity, type: int32, min: 0, max: 1000, sortable: true}
  - {name: price, type: decimal, min: 0}
  - {name: metadata, type: json, nullable: true}
`)
	files, err := renderGenerated("example.com/app", []spec.Resource{resource}, ProjectOptions{
		HTTP: HTTPEcho, Database: DatabaseSQLite, Cache: CacheNone, Messaging: MessagingNone,
		Logging: LoggingSlog, Auth: AuthSession, Profile: ProfilePersonal,
	})
	if err != nil {
		t.Fatal(err)
	}
	resourcePath := "web/src/generated/inventoryitem_gen.ts"
	body := string(files[resourcePath])
	for _, want := range []string{
		generatedMarker,
		`apiPath: "/api/v1/stock-items"`,
		`sku: z.string().max(40)`,
		`status: z.enum(["active","archived"]).optional()`,
		`quantity: numberSchema(true, "0", "1000").optional()`,
		`price: decimalSchema("0", undefined).optional()`,
		`metadata: jsonSchema.nullable().optional()`,
		`"searchable":true`,
		`"filterable":true`,
		`"sortable":true`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("%s missing %q\n%s", resourcePath, want, body)
		}
	}
	registry := string(files["web/src/generated/registry_gen.ts"])
	if !strings.Contains(registry, `import { inventoryitemResource } from "./inventoryitem_gen"`) ||
		!strings.Contains(registry, "inventoryitemResource,") {
		t.Fatalf("frontend registry does not include resource:\n%s", registry)
	}
	if err := addGeneratedManifest(files); err != nil {
		t.Fatalf("frontend generated paths rejected by manifest: %v", err)
	}
}

func TestNonSessionFrontendOutputIsAbsent(t *testing.T) {
	resource := mustParseFrontendResource(t, `schema_version: 1
name: Product
table: products
route: /products
fields:
  - {name: name, type: string, required: true}
`)
	for _, authChoice := range []AuthChoice{AuthNone, AuthJWT} {
		files, err := renderGenerated("example.com/app", []spec.Resource{resource}, ProjectOptions{
			HTTP: HTTPEcho, Database: DatabaseSQLite, Cache: CacheNone, Messaging: MessagingNone,
			Logging: LoggingSlog, Auth: authChoice, Profile: ProfilePersonal,
		})
		if err != nil {
			t.Fatal(err)
		}
		for name := range files {
			if strings.HasPrefix(name, "web/src/generated/") {
				t.Errorf("auth %s unexpectedly generated %s", authChoice, name)
			}
		}
	}
}

func TestSessionFrontendScaffoldIsSelected(t *testing.T) {
	files, err := selectedScaffoldFiles(ProjectOptions{
		HTTP: HTTPEcho, Database: DatabaseSQLite, Cache: CacheNone, Messaging: MessagingNone,
		Logging: LoggingSlog, Auth: AuthSession, Profile: ProfilePersonal,
	})
	if err != nil {
		t.Fatal(err)
	}
	outputs := make(map[string]bool, len(files))
	for _, file := range files {
		outputs[file.Output] = true
	}
	for _, want := range []string{
		"package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", ".node-version", "web/index.html", "web/embed.go",
		"web/src/App.vue", "web/src/components/DataTable.vue", "web/src/components/FormShell.vue",
		"web/src/views/LoginView.vue", "web/src/views/ResourceView.vue",
		"web/src/views/UsersView.vue", "web/src/views/AuditView.vue",
		"internal/platform/auth/admin.go", "internal/app/admin_handlers.go", "internal/app/admin_handlers_test.go",
	} {
		if !outputs[want] {
			t.Errorf("session scaffold missing %s", want)
		}
	}
}

func mustParseFrontendResource(t *testing.T, source string) spec.Resource {
	t.Helper()
	resource, err := spec.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	return resource
}
