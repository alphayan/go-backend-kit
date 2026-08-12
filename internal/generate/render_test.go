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

func TestStoreUsesConsistentReadSnapshotAndAtomicUpdate(t *testing.T) {
	resource, err := spec.Parse([]byte(`schema_version: 1
name: Task
table: tasks
route: /tasks
fields:
  - name: title
    type: string
`))
	if err != nil {
		t.Fatal(err)
	}

	rendered, err := executeGoTemplate("store_gen.go", storeTemplate, resourceData{
		Module:   "example.com/project",
		Resource: resource,
	})
	if err != nil {
		t.Fatal(err)
	}
	source := string(rendered)
	for _, want := range []string{
		`"database/sql"`,
		`&sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}`,
		`item, err := (store{db: tx}).get(ctx, id)`,
		`updated = item`,
	} {
		if !strings.Contains(source, want) {
			t.Errorf("generated store does not contain %q:\n%s", want, source)
		}
	}
	if strings.Contains(source, "return s.get(ctx, id)") {
		t.Errorf("generated update reads outside its transaction:\n%s", source)
	}
}
