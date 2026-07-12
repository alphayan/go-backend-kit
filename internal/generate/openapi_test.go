package generate

import (
	"testing"

	"github.com/alphayan/go-backend-kit/internal/spec"
	"github.com/pb33f/libopenapi"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestOpenAPIPreservesNullableConstraintsAndDecimal(t *testing.T) {
	resource, err := spec.Parse([]byte(`schema_version: 1
name: Invoice
table: invoices
route: /invoices
fields:
  - name: state
    type: string
    nullable: true
    enum: [draft, paid]
  - name: amount
    type: decimal
    min: 0
    default: "2.50"
`))
	if err != nil {
		t.Fatal(err)
	}
	document := buildOpenAPI([]spec.Resource{resource})
	components := document["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	create := schemas["CreateInvoiceInput"].(map[string]any)
	properties := create["properties"].(map[string]any)
	if _, exists := properties["id"]; exists {
		t.Fatal("create schema exposes base field id")
	}
	state := properties["state"].(map[string]any)
	anyOf := state["anyOf"].([]any)
	stringState := anyOf[0].(map[string]any)
	if _, ok := stringState["enum"]; !ok {
		t.Fatalf("nullable enum constraint is outside its value schema: %#v", state)
	}
	amount := properties["amount"].(map[string]any)
	if amount["type"] != "string" || amount["format"] != "decimal" || amount["x-minimum"] != float64(0) {
		t.Fatalf("decimal schema = %#v", amount)
	}
	if amount["default"] != "2.5" {
		t.Fatalf("decimal default = %#v", amount["default"])
	}
	paths := document["paths"].(map[string]any)
	if len(paths) != 2 {
		t.Fatalf("paths = %d, want collection and member", len(paths))
	}
	encoded, err := marshalJSON(document)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := libopenapi.NewDocument(encoded)
	if err != nil {
		t.Fatalf("parse OpenAPI document: %v", err)
	}
	if _, err := parsed.BuildV3Model(); err != nil {
		t.Fatalf("build OpenAPI 3.1 model: %v", err)
	}
}

func TestOpenAPINonNullableJSONRejectsNull(t *testing.T) {
	resource, err := spec.Parse([]byte(`schema_version: 1
name: Document
table: documents
route: /documents
fields:
  - name: payload
    type: json
`))
	if err != nil {
		t.Fatal(err)
	}
	document := buildOpenAPI([]spec.Resource{resource})
	components := document["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	create := schemas["CreateDocumentInput"].(map[string]any)
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource("create-document.json", create); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile("create-document.json")
	if err != nil {
		t.Fatalf("compile generated JSON Schema: %v", err)
	}
	if err := compiled.Validate(map[string]any{"payload": map[string]any{"ok": true}}); err != nil {
		t.Fatalf("valid JSON payload rejected: %v", err)
	}
	if err := compiled.Validate(map[string]any{"payload": nil}); err == nil {
		t.Fatal("non-nullable JSON schema accepted null")
	}
}
