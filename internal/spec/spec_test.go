package spec_test

import (
	"strings"
	"testing"

	"github.com/alphayan/go-backend-kit/internal/spec"
)

const validProduct = `schema_version: 1
name: Product
table: products
route: /products
fields:
  - name: name
    type: string
    required: true
    max_length: 120
    searchable: true
  - name: status
    type: string
    required: true
    enum: [enabled, disabled]
    filterable: true
  - name: owner_id
    type: int64
    filterable: true
    sortable: true
`

func TestParseValidResource(t *testing.T) {
	r, err := spec.Parse([]byte(validProduct))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if r.SchemaVersion != 1 || r.Name != "Product" || r.Package != "product" {
		t.Fatalf("unexpected normalized resource: %#v", r)
	}
	if got, want := r.Fields[2].GoName, "OwnerID"; got != want {
		t.Fatalf("GoName = %q, want %q", got, want)
	}
	if got, want := r.Fields[2].Column, "owner_id"; got != want {
		t.Fatalf("Column = %q, want %q", got, want)
	}
}

func TestParseSupportsEveryFieldType(t *testing.T) {
	types := []string{"string", "text", "bool", "int32", "int64", "float64", "decimal", "time", "uuid", "json"}
	for _, fieldType := range types {
		t.Run(fieldType, func(t *testing.T) {
			yaml := "schema_version: 1\nname: Value\ntable: values\nroute: /values\nfields:\n  - name: payload\n    type: " + fieldType + "\n"
			if _, err := spec.Parse([]byte(yaml)); err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
		})
	}
}

func TestParseRejectsInvalidSpecs(t *testing.T) {
	tests := map[string]string{
		"unknown key":         strings.Replace(validProduct, "schema_version: 1", "schema_version: 1\nsurprise: true", 1),
		"future schema":       strings.Replace(validProduct, "schema_version: 1", "schema_version: 2", 1),
		"invalid model name":  strings.Replace(validProduct, "name: Product", "name: product-item", 1),
		"generated type name": strings.Replace(validProduct, "name: Product", "name: Handler", 1),
		"unimportable main":   strings.Replace(validProduct, "name: Product", "name: Main", 1),
		"ignored testdata":    strings.Replace(validProduct, "name: Product", "name: Testdata", 1),
		"windows con":         strings.Replace(validProduct, "name: Product", "name: Con", 1),
		"windows nul":         strings.Replace(validProduct, "name: Product", "name: Nul", 1),
		"windows com1":        strings.Replace(validProduct, "name: Product", "name: COM1", 1),
		"windows lpt9":        strings.Replace(validProduct, "name: Product", "name: LPT9", 1),
		"invalid table":       strings.Replace(validProduct, "table: products", "table: public.products", 1),
		"path traversal":      strings.Replace(validProduct, "route: /products", "route: /../products", 1),
		"base field":          strings.Replace(validProduct, "name: name", "name: created_at", 1),
		"base go field":       validProduct + "  - name: i_d\n    type: int64\n",
		"method collision":    validProduct + "  - name: table_name\n    type: string\n",
		"hook collision":      validProduct + "  - name: after_find\n    type: string\n",
		"trailing underscore": validProduct + "  - name: broken_\n    type: string\n",
		"empty name segment":  validProduct + "  - name: broken__name\n    type: string\n",
		"duplicate field":     validProduct + "  - name: owner_id\n    type: int64\n",
		"go name collision":   validProduct + "  - name: owner_i_d\n    type: int64\n",
		"unknown type":        strings.Replace(validProduct, "type: string", "type: bytes", 1),
		"nullable required":   strings.Replace(validProduct, "required: true", "required: true\n    nullable: true", 1),
		"default required":    strings.Replace(validProduct, "max_length: 120", "default: unnamed\n    max_length: 120", 1),
		"bad max length":      strings.Replace(validProduct, "max_length: 120", "max_length: 0", 1),
		"search number":       strings.Replace(validProduct, "name: owner_id\n    type: int64", "name: owner_id\n    type: int64\n    searchable: true", 1),
		"bad numeric range":   strings.Replace(validProduct, "sortable: true", "sortable: true\n    min: 10\n    max: 1", 1),
		"integer empty range": strings.Replace(validProduct, "sortable: true", "sortable: true\n    min: 0.1\n    max: 0.9", 1),
		"float64 empty range": strings.Replace(
			strings.Replace(validProduct, "type: int64", "type: float64", 1),
			"sortable: true",
			"sortable: true\n    min: 12.5000000000000001\n    max: 12.5000000000000002",
			1,
		),
		"enum too long":     strings.Replace(validProduct, "enum: [enabled, disabled]", "enum: [enabled, disabled]\n    max_length: 3", 1),
		"default above max": strings.Replace(validProduct, "sortable: true", "sortable: true\n    default: 11\n    max: 10", 1),
		"exact int64 default above max": strings.Replace(
			validProduct,
			"sortable: true",
			"sortable: true\n    default: 9007199254740993\n    max: 9007199254740992",
			1,
		),
		"exact decimal default above max": validProduct + `  - name: amount
    type: decimal
    default: "9007199254740993"
    max: 9007199254740992
`,
		"default too long":    validProduct + "  - name: code\n    type: string\n    default: abc\n    max_length: 2\n",
		"bad decimal default": validProduct + "  - name: amount\n    type: decimal\n    default: not-a-decimal\n",
		"nonfinite maximum":   strings.Replace(validProduct, "sortable: true", "sortable: true\n    max: .inf", 1),
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := spec.Parse([]byte(input)); err == nil {
				t.Fatal("Parse() error = nil, want validation error")
			}
		})
	}
}

func TestParseIsDeterministic(t *testing.T) {
	a, err := spec.Parse([]byte(validProduct))
	if err != nil {
		t.Fatal(err)
	}
	b, err := spec.Parse([]byte(validProduct))
	if err != nil {
		t.Fatal(err)
	}
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatalf("fingerprints differ: %q != %q", a.Fingerprint(), b.Fingerprint())
	}
}

func TestFingerprintUsesCanonicalNumericBounds(t *testing.T) {
	withBound := func(bound string) string {
		return strings.Replace(validProduct, "sortable: true", "sortable: true\n    max: "+bound, 1)
	}
	a, err := spec.Parse([]byte(withBound("1.00")))
	if err != nil {
		t.Fatal(err)
	}
	b, err := spec.Parse([]byte(withBound("1e0")))
	if err != nil {
		t.Fatal(err)
	}
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatalf("equivalent numeric bounds changed fingerprint: %q != %q", a.Fingerprint(), b.Fingerprint())
	}
}

func TestParseNormalizesFloat64DefaultBeforeConstraintComparison(t *testing.T) {
	resource, err := spec.Parse([]byte(`schema_version: 1
name: Reading
table: readings
route: /readings
fields:
  - name: value
    type: float64
    default: 9007199254740993
    max: 9007199254740992
`))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := resource.Fields[0].Default.(float64)
	if !ok || got != 9007199254740992 {
		t.Fatalf("float64 default = %#v, want normalized float64 value", resource.Fields[0].Default)
	}
}
